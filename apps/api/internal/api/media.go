package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	MediaUploadStatusCreated  = "created"
	MediaUploadStatusUploaded = "uploaded"
	MediaUploadStatusExpired  = "expired"
	MediaUploadStatusFailed   = "failed"

	defaultMediaBucket = "echotrails-dev"
	maxMediaBytes      = 100 * 1024 * 1024
)

var (
	ErrInvalidMediaUpload  = errors.New("invalid media upload")
	ErrMediaUploadNotFound = errors.New("media upload not found")
	ErrMediaUploadExpired  = errors.New("media upload expired")
)

type CreateMediaUploadRequest struct {
	EchoID      string `json:"echo_id"`
	MediaType   string `json:"media_type"`
	ContentType string `json:"content_type"`
	ByteSize    int64  `json:"byte_size"`
}

type MediaUploadResponse struct {
	UploadID     string    `json:"upload_id"`
	MediaAssetID string    `json:"media_asset_id"`
	UploadURL    string    `json:"upload_url,omitempty"`
	Method       string    `json:"method"`
	ObjectKey    string    `json:"object_key"`
	Status       string    `json:"status"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type MediaUploadStore interface {
	CreateUpload(context.Context, CreateMediaUploadRequest, string, time.Time) (MediaUploadResponse, error)
	GetUpload(context.Context, string) (MediaUploadResponse, error)
	CompleteUpload(context.Context, string) (MediaUploadResponse, error)
}

type MemoryMediaUploadStore struct {
	mu          sync.Mutex
	baseURL     string
	now         func() time.Time
	uploads     map[string]MediaUploadResponse
	idempotency map[string]string
}

func NewMemoryMediaUploadStore() *MemoryMediaUploadStore {
	return &MemoryMediaUploadStore{
		baseURL:     "http://127.0.0.1:19000/" + defaultMediaBucket,
		now:         time.Now,
		uploads:     map[string]MediaUploadResponse{},
		idempotency: map[string]string{},
	}
}

func (s *MemoryMediaUploadStore) CreateUpload(_ context.Context, req CreateMediaUploadRequest, idempotencyKey string, now time.Time) (MediaUploadResponse, error) {
	if err := validateMediaUpload(req); err != nil {
		return MediaUploadResponse{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if idempotencyKey != "" {
		if uploadID, ok := s.idempotency[idempotencyKey]; ok {
			response := s.uploads[uploadID]
			if response.Status == MediaUploadStatusCreated && !response.ExpiresAt.After(now) {
				response.Status = MediaUploadStatusExpired
				s.uploads[uploadID] = response
			}
			return s.safeResponse(response, now), nil
		}
	}
	uploadID := newOpaqueID("upl")
	assetID := newOpaqueID("ast")
	objectKey := fmt.Sprintf("media/original/%s/%s", assetID, safeExtension(req.ContentType))
	response := MediaUploadResponse{
		UploadID:     uploadID,
		MediaAssetID: assetID,
		UploadURL:    strings.TrimRight(s.baseURL, "/") + "/" + objectKey,
		Method:       "PUT",
		ObjectKey:    objectKey,
		Status:       MediaUploadStatusCreated,
		ExpiresAt:    now.Add(15 * time.Minute).UTC(),
	}
	s.uploads[uploadID] = response
	if idempotencyKey != "" {
		s.idempotency[idempotencyKey] = uploadID
	}
	return response, nil
}

func (s *MemoryMediaUploadStore) GetUpload(_ context.Context, uploadID string) (MediaUploadResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	response, ok := s.uploads[uploadID]
	if !ok {
		return MediaUploadResponse{}, ErrMediaUploadNotFound
	}
	now := s.now().UTC()
	if response.Status == MediaUploadStatusCreated && !response.ExpiresAt.After(now) {
		response.Status = MediaUploadStatusExpired
		s.uploads[uploadID] = response
	}
	return s.safeResponse(response, now), nil
}

func (s *MemoryMediaUploadStore) CompleteUpload(_ context.Context, uploadID string) (MediaUploadResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	response, ok := s.uploads[uploadID]
	if !ok {
		return MediaUploadResponse{}, ErrMediaUploadNotFound
	}
	now := s.now().UTC()
	if response.Status == MediaUploadStatusUploaded {
		return s.safeResponse(response, now), nil
	}
	if !response.ExpiresAt.After(now) {
		response.Status = MediaUploadStatusExpired
		s.uploads[uploadID] = response
		return MediaUploadResponse{}, ErrMediaUploadExpired
	}
	response.Status = MediaUploadStatusUploaded
	s.uploads[uploadID] = response
	return s.safeResponse(response, now), nil
}

func (s *MemoryMediaUploadStore) safeResponse(response MediaUploadResponse, now time.Time) MediaUploadResponse {
	if response.Status != MediaUploadStatusCreated || !response.ExpiresAt.After(now) {
		response.UploadURL = ""
	}
	return response
}

func validateMediaUpload(req CreateMediaUploadRequest) error {
	if strings.TrimSpace(req.EchoID) == "" || strings.TrimSpace(req.MediaType) == "" || strings.TrimSpace(req.ContentType) == "" {
		return ErrInvalidMediaUpload
	}
	if req.ByteSize <= 0 || req.ByteSize > maxMediaBytes {
		return ErrInvalidMediaUpload
	}
	allowed, ok := allowedMediaContentTypes[req.MediaType]
	if !ok {
		return ErrInvalidMediaUpload
	}
	for _, contentType := range allowed {
		if req.ContentType == contentType {
			return nil
		}
	}
	return ErrInvalidMediaUpload
}

var allowedMediaContentTypes = map[string][]string{
	"image": {"image/jpeg", "image/png", "image/heic"},
	"audio": {"audio/mpeg", "audio/mp4", "audio/wav"},
	"video": {"video/mp4", "video/quicktime"},
}

func safeExtension(contentType string) string {
	switch contentType {
	case "image/jpeg":
		return "original.jpg"
	case "image/png":
		return "original.png"
	case "image/heic":
		return "original.heic"
	case "audio/mpeg":
		return "original.mp3"
	case "audio/mp4":
		return "original.m4a"
	case "audio/wav":
		return "original.wav"
	case "video/quicktime":
		return "original.mov"
	default:
		return "original.mp4"
	}
}

func newOpaqueID(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}
