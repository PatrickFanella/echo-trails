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

	MediaActivityStatusCreated       = "created"
	MediaActivityStatusUploaded      = "uploaded"
	MediaActivityStatusProcessing    = "processing"
	MediaActivityStatusPendingReview = "pending_review"
	MediaActivityStatusFailed        = "failed"
	MediaActivityStatusExpired       = "expired"

	MediaTranscriptStatusPending = "pending"
	MediaModerationStatusPending = "pending"
	MediaModerationRiskUnknown   = "unknown"

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
	SortOrder   int    `json:"sort_order,omitempty"`
}

type CreateAvatarUploadRequest struct {
	ContentType string `json:"content_type"`
	ByteSize    int64  `json:"byte_size"`
}

type MediaUploadResponse struct {
	UploadID         string    `json:"upload_id"`
	MediaAssetID     string    `json:"media_asset_id"`
	UploadURL        string    `json:"upload_url,omitempty"`
	Method           string    `json:"method"`
	ObjectKey        string    `json:"object_key"`
	Status           string    `json:"status"`
	ActivityStatus   string    `json:"activity_status"`
	TranscriptStatus string    `json:"transcript_status,omitempty"`
	ModerationStatus string    `json:"moderation_status,omitempty"`
	ModerationRisk   string    `json:"moderation_risk,omitempty"`
	StatusReason     string    `json:"status_reason,omitempty"`
	ExpiresAt        time.Time `json:"expires_at"`
}

type MediaUploadStore interface {
	CreateUpload(context.Context, string, CreateMediaUploadRequest, string, time.Time) (MediaUploadResponse, error)
	GetUpload(context.Context, string, string) (MediaUploadResponse, error)
	CompleteUpload(context.Context, string, string) (MediaUploadResponse, error)
	RenewUpload(context.Context, string, string, time.Time) (MediaUploadResponse, error)
}

// AvatarMediaUploadStore keeps profile images on the same private upload,
// processing, metadata-stripping, and moderation path as Echo photos. Avatar
// assets have no echo_id and may only be selected after approval.
type AvatarMediaUploadStore interface {
	CreateAvatarUpload(context.Context, string, CreateAvatarUploadRequest, string, time.Time) (MediaUploadResponse, error)
	ValidateAvatarSelection(context.Context, string, string) error
}

func (s *MemoryMediaUploadStore) RenewUpload(_ context.Context, _ string, uploadID string, now time.Time) (MediaUploadResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	response, ok := s.uploads[uploadID]
	if !ok {
		return MediaUploadResponse{}, ErrMediaUploadNotFound
	}
	if response.Status == MediaUploadStatusUploaded {
		return MediaUploadResponse{}, ErrInvalidMediaUpload
	}
	response.Status = MediaUploadStatusCreated
	response.ActivityStatus = MediaActivityStatusCreated
	response.ExpiresAt = now.Add(15 * time.Minute).UTC()
	response.UploadURL = strings.TrimRight(s.baseURL, "/") + "/" + response.ObjectKey
	response.StatusReason = "upload authorization renewed"
	s.uploads[uploadID] = response
	return response, nil
}

type PrivateObjectURLStore interface {
	GetPrivateObjectURL(context.Context, string) (string, error)
}

type MemoryMediaUploadStore struct {
	mu          sync.Mutex
	baseURL     string
	now         func() time.Time
	uploads     map[string]MediaUploadResponse
	idempotency map[string]string
	avatarOwner map[string]string
	avatarState map[string]string
}

func NewMemoryMediaUploadStore() *MemoryMediaUploadStore {
	return &MemoryMediaUploadStore{
		baseURL:     "http://127.0.0.1:19000/" + defaultMediaBucket,
		now:         time.Now,
		uploads:     map[string]MediaUploadResponse{},
		idempotency: map[string]string{},
		avatarOwner: map[string]string{},
		avatarState: map[string]string{},
	}
}

func (s *MemoryMediaUploadStore) CreateAvatarUpload(_ context.Context, userID string, req CreateAvatarUploadRequest, idempotencyKey string, now time.Time) (MediaUploadResponse, error) {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(idempotencyKey) == "" || !validAvatarUpload(req) {
		return MediaUploadResponse{}, ErrInvalidMediaUpload
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	scopedKey := userID + ":avatar:" + idempotencyKey
	if uploadID, ok := s.idempotency[scopedKey]; ok {
		return s.safeResponse(s.uploads[uploadID], now), nil
	}
	uploadID := newOpaqueID("upl")
	assetID := newOpaqueID("ast")
	objectKey := fmt.Sprintf("media/original/%s/%s", assetID, safeExtension(req.ContentType))
	response := MediaUploadResponse{UploadID: uploadID, MediaAssetID: assetID, UploadURL: strings.TrimRight(s.baseURL, "/") + "/" + objectKey,
		Method: "PUT", ObjectKey: objectKey, Status: MediaUploadStatusCreated, ActivityStatus: MediaActivityStatusCreated, ExpiresAt: now.Add(15 * time.Minute).UTC()}
	s.uploads[uploadID] = response
	s.idempotency[scopedKey] = uploadID
	s.avatarOwner[assetID] = userID
	s.avatarState[assetID] = MediaUploadStatusCreated
	return response, nil
}

func (s *MemoryMediaUploadStore) ValidateAvatarSelection(_ context.Context, userID, mediaAssetID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.avatarOwner[mediaAssetID] != userID || s.avatarState[mediaAssetID] != "approved" {
		return ErrAvatarNotApproved
	}
	return nil
}

// ApproveAvatarAsset is intentionally test/local-memory only. Production
// approval is written by the conservative moderation worker.
func (s *MemoryMediaUploadStore) ApproveAvatarAsset(mediaAssetID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.avatarOwner[mediaAssetID]; ok {
		s.avatarState[mediaAssetID] = "approved"
	}
}

func validAvatarUpload(req CreateAvatarUploadRequest) bool {
	if req.ByteSize <= 0 || req.ByteSize > 10*1024*1024 {
		return false
	}
	for _, contentType := range []string{"image/jpeg", "image/png", "image/heic"} {
		if req.ContentType == contentType {
			return true
		}
	}
	return false
}

func (s *MemoryMediaUploadStore) CreateUpload(_ context.Context, _ string, req CreateMediaUploadRequest, idempotencyKey string, now time.Time) (MediaUploadResponse, error) {
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
				response.ActivityStatus = MediaActivityStatusExpired
				response.StatusReason = "upload expired before completion"
				s.uploads[uploadID] = response
			}
			return s.safeResponse(response, now), nil
		}
	}
	uploadID := newOpaqueID("upl")
	assetID := newOpaqueID("ast")
	objectKey := fmt.Sprintf("media/original/%s/%s", assetID, safeExtension(req.ContentType))
	response := MediaUploadResponse{
		UploadID:       uploadID,
		MediaAssetID:   assetID,
		UploadURL:      strings.TrimRight(s.baseURL, "/") + "/" + objectKey,
		Method:         "PUT",
		ObjectKey:      objectKey,
		Status:         MediaUploadStatusCreated,
		ActivityStatus: MediaActivityStatusCreated,
		ExpiresAt:      now.Add(15 * time.Minute).UTC(),
	}
	s.uploads[uploadID] = response
	if idempotencyKey != "" {
		s.idempotency[idempotencyKey] = uploadID
	}
	return response, nil
}

func (s *MemoryMediaUploadStore) GetUpload(_ context.Context, _ string, uploadID string) (MediaUploadResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	response, ok := s.uploads[uploadID]
	if !ok {
		return MediaUploadResponse{}, ErrMediaUploadNotFound
	}
	now := s.now().UTC()
	if response.Status == MediaUploadStatusCreated && !response.ExpiresAt.After(now) {
		response.Status = MediaUploadStatusExpired
		response.ActivityStatus = MediaActivityStatusExpired
		response.StatusReason = "upload expired before completion"
		s.uploads[uploadID] = response
	}
	return s.safeResponse(response, now), nil
}

func (s *MemoryMediaUploadStore) CompleteUpload(_ context.Context, _ string, uploadID string) (MediaUploadResponse, error) {
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
		response.ActivityStatus = MediaActivityStatusExpired
		response.StatusReason = "upload expired before completion"
		s.uploads[uploadID] = response
		return MediaUploadResponse{}, ErrMediaUploadExpired
	}
	response.Status = MediaUploadStatusUploaded
	response.ActivityStatus = MediaActivityStatusProcessing
	response.TranscriptStatus = MediaTranscriptStatusPending
	response.ModerationStatus = MediaModerationStatusPending
	response.ModerationRisk = MediaModerationRiskUnknown
	response.StatusReason = "upload complete; processing queued"
	s.uploads[uploadID] = response
	if _, ok := s.avatarState[response.MediaAssetID]; ok {
		s.avatarState[response.MediaAssetID] = MediaUploadStatusUploaded
	}
	return s.safeResponse(response, now), nil
}

func (s *MemoryMediaUploadStore) safeResponse(response MediaUploadResponse, now time.Time) MediaUploadResponse {
	if response.Status != MediaUploadStatusCreated || !response.ExpiresAt.After(now) {
		response.UploadURL = ""
	}
	if response.ActivityStatus == "" {
		response.ActivityStatus = activityStatusForUpload(response.Status)
	}
	return response
}

func activityStatusForUpload(status string) string {
	switch status {
	case MediaUploadStatusCreated:
		return MediaActivityStatusCreated
	case MediaUploadStatusUploaded:
		return MediaActivityStatusUploaded
	case MediaUploadStatusExpired:
		return MediaActivityStatusExpired
	case MediaUploadStatusFailed:
		return MediaActivityStatusFailed
	default:
		return MediaActivityStatusFailed
	}
}

func validateMediaUpload(req CreateMediaUploadRequest) error {
	if strings.TrimSpace(req.EchoID) == "" || strings.TrimSpace(req.MediaType) == "" || strings.TrimSpace(req.ContentType) == "" {
		return ErrInvalidMediaUpload
	}
	if req.ByteSize <= 0 || req.ByteSize > maxMediaBytes {
		return ErrInvalidMediaUpload
	}
	if req.SortOrder < 0 || req.SortOrder > 4 {
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
	"image": {"image/jpeg", "image/heic"},
	"audio": {"audio/mp4"},
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
