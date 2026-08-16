package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/primitives/pagination"
	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/primitives/uuidv7"
)

type EchoContentMode string

const (
	EchoContentText     EchoContentMode = "text"
	EchoContentAudio    EchoContentMode = "audio"
	EchoContentPhotoSet EchoContentMode = "photo_set"
	EchoContentVideo    EchoContentMode = "video"

	EchoStatusDraft     = "draft"
	EchoStatusUploading = "uploading"
	EchoStatusPublished = "published"
	EchoStatusLimited   = "limited"
	EchoStatusRemoved   = "removed"
	EchoStatusDeleted   = "deleted"
	EchoStatusFailed    = "failed"
)

var (
	ErrInvalidEcho          = errors.New("invalid echo")
	ErrEchoNotFound         = errors.New("echo not found")
	ErrEchoForbidden        = errors.New("echo forbidden")
	ErrEchoConflict         = errors.New("echo idempotency conflict")
	ErrCreationProofMissing = errors.New("creation proof missing")
	ErrCreationProofUsed    = errors.New("creation proof already used")
)

type Coordinate struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

type CreateEchoRequest struct {
	PlaceID       string          `json:"place_id"`
	ContentMode   EchoContentMode `json:"content_mode"`
	Title         string          `json:"title"`
	BodyText      string          `json:"body_text,omitempty"`
	CaptionText   string          `json:"caption_text,omitempty"`
	CreationToken string          `json:"creation_token"`
}

// EchoPatchRequest deliberately excludes Place and content mode. Moving an
// Echo or changing its media mode would invalidate its creation proof and
// processing pipeline; callers create a new Echo for those changes.
type EchoPatchRequest struct {
	Title       *string `json:"title,omitempty"`
	BodyText    *string `json:"body_text,omitempty"`
	CaptionText *string `json:"caption_text,omitempty"`
}

type EchoRecord struct {
	ID              string
	AuthorUserID    string
	AuthorHandle    string
	PlaceID         string
	ContentMode     EchoContentMode
	Title           string
	BodyText        string
	CaptionText     string
	Status          string
	PublicLocation  Coordinate
	PrivateLocation Coordinate
	CreatedAt       time.Time
	UpdatedAt       time.Time
	PublishedAt     *time.Time
	StitchSource    *StitchSourceResponse
	Media           []MediaAssetResponse
}

// MediaAssetResponse contains only public, processed derivatives. Originals,
// storage keys, and moderation evidence are intentionally omitted.
type MediaAssetResponse struct {
	ID              string   `json:"id"`
	MediaType       string   `json:"media_type"`
	Status          string   `json:"status"`
	PublicURL       string   `json:"public_url"`
	ThumbnailURL    *string  `json:"thumbnail_url,omitempty"`
	DurationSeconds *float64 `json:"duration_seconds,omitempty"`
	Transcript      *string  `json:"transcript,omitempty"`
}

type StitchSourceResponse struct {
	EchoID           string `json:"echo_id"`
	Title            string `json:"title"`
	AuthorHandle     string `json:"author_handle"`
	QuotedReplyID    string `json:"quoted_reply_id,omitempty"`
	QuoteSnapshotURL string `json:"quote_snapshot_url,omitempty"`
	QuoteUnavailable bool   `json:"quote_unavailable,omitempty"`
}

type EchoResponse struct {
	ID             string                `json:"id"`
	AuthorHandle   string                `json:"author_handle"`
	PlaceID        string                `json:"place_id"`
	ContentMode    EchoContentMode       `json:"content_mode"`
	Title          string                `json:"title"`
	BodyText       string                `json:"body_text,omitempty"`
	CaptionText    string                `json:"caption_text,omitempty"`
	Status         string                `json:"status"`
	PublicLocation Coordinate            `json:"public_location"`
	CreatedAt      time.Time             `json:"created_at"`
	UpdatedAt      time.Time             `json:"updated_at"`
	PublishedAt    *time.Time            `json:"published_at,omitempty"`
	StitchSource   *StitchSourceResponse `json:"stitch_source,omitempty"`
	Media          []MediaAssetResponse  `json:"media,omitempty"`
}

type EchoCreateInput struct {
	Request                 CreateEchoRequest
	AuthorUserID            string
	AuthorHandle            string
	PrivateLocation         Coordinate
	PublicLocation          Coordinate
	CreationAuthorizationID string
	IdempotencyKey          string
	Now                     time.Time
}

type EchoStore interface {
	CreateDraft(context.Context, EchoCreateInput) (EchoRecord, error)
	Get(context.Context, string, string) (EchoRecord, error)
	UpdateDraft(context.Context, string, string, EchoPatchRequest, time.Time) (EchoRecord, error)
	Delete(context.Context, string, string, time.Time) error
	Submit(context.Context, string, string, time.Time) (EchoRecord, error)
	ListPublishedByPlace(context.Context, string, int) ([]EchoRecord, error)
	ListPublishedByPlacePage(context.Context, string, string, int) (EchoRecordPage, error)
	ListPublishedByPlaceSortedPage(context.Context, string, string, int, string) (EchoRecordPage, error)
}

type EchoRecordPage struct {
	Records    []EchoRecord
	NextCursor string
}

func validateEchoPatch(record EchoRecord, patch EchoPatchRequest) (EchoRecord, error) {
	if patch.Title == nil && patch.BodyText == nil && patch.CaptionText == nil {
		return EchoRecord{}, ErrInvalidEcho
	}
	if patch.Title != nil {
		record.Title = strings.TrimSpace(*patch.Title)
	}
	if patch.BodyText != nil {
		record.BodyText = strings.TrimSpace(*patch.BodyText)
	}
	if patch.CaptionText != nil {
		record.CaptionText = strings.TrimSpace(*patch.CaptionText)
	}
	request := CreateEchoRequest{
		PlaceID: record.PlaceID, ContentMode: record.ContentMode, Title: record.Title,
		BodyText: record.BodyText, CaptionText: record.CaptionText, CreationToken: "retained-server-side",
	}
	if err := ValidateCreateEchoRequest(request); err != nil {
		return EchoRecord{}, err
	}
	return record, nil
}

type echoIdempotencyRecord struct {
	Fingerprint string
	EchoID      string
	Response    EchoRecord
}

type MemoryEchoStore struct {
	mu          sync.RWMutex
	records     map[string]EchoRecord
	byPlace     map[string][]string
	idempotency map[string]echoIdempotencyRecord
}

func NewMemoryEchoStore(seed []EchoRecord) *MemoryEchoStore {
	store := &MemoryEchoStore{records: map[string]EchoRecord{}, byPlace: map[string][]string{}, idempotency: map[string]echoIdempotencyRecord{}}
	for _, echo := range seed {
		store.records[echo.ID] = echo
		store.byPlace[echo.PlaceID] = append(store.byPlace[echo.PlaceID], echo.ID)
	}
	return store
}

func DefaultEchoes() []EchoRecord {
	publishedAt := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	return []EchoRecord{{
		ID: "echo-active-1", AuthorUserID: "user-seed-1", AuthorHandle: "seed_storyteller", PlaceID: PlacePrincePondID,
		ContentMode: EchoContentText, Title: "Storm meeting place", BodyText: "People gathered here after every big storm.", Status: EchoStatusPublished,
		PublicLocation: Coordinate{Lat: 41.7975912, Lon: -88.0153297}, CreatedAt: publishedAt, UpdatedAt: publishedAt, PublishedAt: &publishedAt,
	}}
}

func (s *MemoryEchoStore) CreateDraft(_ context.Context, input EchoCreateInput) (EchoRecord, error) {
	fingerprint := echoFingerprint(input)
	key := input.AuthorUserID + ":" + input.IdempotencyKey
	s.mu.Lock()
	defer s.mu.Unlock()
	if previous, ok := s.idempotency[key]; ok {
		if previous.Fingerprint != fingerprint {
			return EchoRecord{}, ErrEchoConflict
		}
		return previous.Response, nil
	}
	id, err := uuidv7.NewAt(input.Now, rand.Reader)
	if err != nil {
		return EchoRecord{}, err
	}
	record := EchoRecord{
		ID: id.String(), AuthorUserID: input.AuthorUserID, AuthorHandle: input.AuthorHandle, PlaceID: input.Request.PlaceID,
		ContentMode: input.Request.ContentMode, Title: input.Request.Title, BodyText: input.Request.BodyText, CaptionText: input.Request.CaptionText,
		Status: EchoStatusDraft, PublicLocation: input.PublicLocation, PrivateLocation: input.PrivateLocation, CreatedAt: input.Now.UTC(), UpdatedAt: input.Now.UTC(),
	}
	s.records[record.ID] = record
	s.byPlace[record.PlaceID] = append(s.byPlace[record.PlaceID], record.ID)
	s.idempotency[key] = echoIdempotencyRecord{Fingerprint: fingerprint, EchoID: record.ID, Response: record}
	return record, nil
}

func (s *MemoryEchoStore) Get(_ context.Context, echoID, requesterID string) (EchoRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.records[echoID]
	if !ok || record.Status == EchoStatusDeleted || record.Status == EchoStatusRemoved {
		return EchoRecord{}, ErrEchoNotFound
	}
	if record.Status != EchoStatusPublished && record.AuthorUserID != requesterID {
		return EchoRecord{}, ErrEchoForbidden
	}
	return record, nil
}

func (s *MemoryEchoStore) UpdateDraft(_ context.Context, echoID, authorID string, patch EchoPatchRequest, now time.Time) (EchoRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[echoID]
	if !ok || record.Status == EchoStatusDeleted || record.Status == EchoStatusRemoved {
		return EchoRecord{}, ErrEchoNotFound
	}
	if record.AuthorUserID != authorID {
		return EchoRecord{}, ErrEchoForbidden
	}
	if record.Status != EchoStatusDraft {
		return EchoRecord{}, ErrInvalidEcho
	}
	updated, err := validateEchoPatch(record, patch)
	if err != nil {
		return EchoRecord{}, err
	}
	updated.UpdatedAt = now.UTC()
	s.records[echoID] = updated
	return updated, nil
}

func (s *MemoryEchoStore) Delete(_ context.Context, echoID, authorID string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[echoID]
	if !ok {
		return ErrEchoNotFound
	}
	if record.AuthorUserID != authorID {
		return ErrEchoForbidden
	}
	if record.Status == EchoStatusDeleted {
		return nil
	}
	record.Status = EchoStatusDeleted
	record.UpdatedAt = now.UTC()
	s.records[echoID] = record
	return nil
}

func (s *MemoryEchoStore) Submit(_ context.Context, echoID, authorID string, now time.Time) (EchoRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[echoID]
	if !ok {
		return EchoRecord{}, ErrEchoNotFound
	}
	if record.AuthorUserID != authorID {
		return EchoRecord{}, ErrEchoForbidden
	}
	if record.Status == EchoStatusPublished || record.Status == EchoStatusUploading {
		return record, nil
	}
	if record.Status != EchoStatusDraft {
		return EchoRecord{}, ErrInvalidEcho
	}
	if record.ContentMode == EchoContentText {
		record.Status = EchoStatusPublished
		publishedAt := now.UTC()
		record.PublishedAt = &publishedAt
	} else {
		record.Status = EchoStatusUploading
	}
	record.UpdatedAt = now.UTC()
	s.records[echoID] = record
	return record, nil
}

func (s *MemoryEchoStore) ListPublishedByPlace(_ context.Context, placeID string, limit int) ([]EchoRecord, error) {
	page, err := s.ListPublishedByPlacePage(context.Background(), placeID, "", limit)
	return page.Records, err
}

func (s *MemoryEchoStore) ListPublishedByPlacePage(_ context.Context, placeID, cursorValue string, limit int) (EchoRecordPage, error) {
	return s.ListPublishedByPlaceSortedPage(context.Background(), placeID, cursorValue, limit, "notable")
}

func (s *MemoryEchoStore) ListPublishedByPlaceSortedPage(_ context.Context, placeID, cursorValue string, limit int, order string) (EchoRecordPage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	var cursor *pagination.Cursor
	if cursorValue != "" {
		decoded, err := pagination.Decode(cursorValue)
		if err != nil {
			return EchoRecordPage{}, err
		}
		cursor = &decoded
	}
	eligible := make([]EchoRecord, 0, len(s.byPlace[placeID]))
	for _, echoID := range s.byPlace[placeID] {
		record := s.records[echoID]
		if record.Status == EchoStatusPublished {
			eligible = append(eligible, record)
		}
	}
	oldest := order == "oldest"
	sort.Slice(eligible, func(i, j int) bool {
		left, right := echoPageTime(eligible[i]), echoPageTime(eligible[j])
		if left.Equal(right) {
			if oldest {
				return eligible[i].ID < eligible[j].ID
			}
			return eligible[i].ID > eligible[j].ID
		}
		if oldest {
			return left.Before(right)
		}
		return left.After(right)
	})
	result := make([]EchoRecord, 0, limit+1)
	for _, record := range eligible {
		if cursor != nil {
			timestamp := echoPageTime(record)
			if oldest {
				if timestamp.Before(cursor.After) || (timestamp.Equal(cursor.After) && record.ID <= cursor.ID.String()) {
					continue
				}
			} else if timestamp.After(cursor.After) || (timestamp.Equal(cursor.After) && record.ID >= cursor.ID.String()) {
				continue
			}
		}
		result = append(result, record)
		if len(result) == limit+1 {
			break
		}
	}
	page := EchoRecordPage{Records: result}
	if len(result) > limit {
		page.Records = result[:limit]
		last := page.Records[len(page.Records)-1]
		id, err := uuidv7.Parse(last.ID)
		if err != nil {
			return EchoRecordPage{}, err
		}
		page.NextCursor, err = pagination.Encode(pagination.Cursor{After: echoPageTime(last), ID: id})
		if err != nil {
			return EchoRecordPage{}, err
		}
	}
	return page, nil
}

func echoPageTime(record EchoRecord) time.Time {
	if record.PublishedAt != nil {
		return record.PublishedAt.UTC()
	}
	return record.CreatedAt.UTC()
}

func ValidateCreateEchoRequest(request CreateEchoRequest) error {
	request.PlaceID = strings.TrimSpace(request.PlaceID)
	request.Title = strings.TrimSpace(request.Title)
	request.BodyText = strings.TrimSpace(request.BodyText)
	request.CaptionText = strings.TrimSpace(request.CaptionText)
	if request.PlaceID == "" || request.Title == "" || len([]rune(request.Title)) > 80 || request.CreationToken == "" {
		return ErrInvalidEcho
	}
	switch request.ContentMode {
	case EchoContentText:
		if request.BodyText == "" || len([]rune(request.BodyText)) > 500 || request.CaptionText != "" {
			return ErrInvalidEcho
		}
	case EchoContentAudio, EchoContentPhotoSet, EchoContentVideo:
		if request.BodyText != "" || len([]rune(request.CaptionText)) > 500 {
			return ErrInvalidEcho
		}
	default:
		return ErrInvalidEcho
	}
	return nil
}

func toEchoResponse(record EchoRecord, _ string) EchoResponse {
	response := EchoResponse{
		ID: record.ID, AuthorHandle: record.AuthorHandle, PlaceID: record.PlaceID, ContentMode: record.ContentMode,
		Title: record.Title, BodyText: record.BodyText, CaptionText: record.CaptionText, Status: record.Status,
		PublicLocation: record.PublicLocation, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt, PublishedAt: record.PublishedAt,
		StitchSource: record.StitchSource,
		Media:        append([]MediaAssetResponse(nil), record.Media...),
	}
	return response
}

func jitteredPublicLocation(place Place, echoID string, key []byte) Coordinate {
	if place.Latitude == nil || place.Longitude == nil {
		return Coordinate{}
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(echoID))
	digest := mac.Sum(nil)
	angle := (float64(binary.BigEndian.Uint64(digest[:8])) / float64(math.MaxUint64)) * 2 * math.Pi
	radius := math.Min(math.Max(place.RadiusMeters*0.35, 20), 75)
	latRadians := *place.Latitude * math.Pi / 180
	latOffset := math.Cos(angle) * radius / 111320
	lonOffset := math.Sin(angle) * radius / (111320 * math.Max(math.Cos(latRadians), 0.01))
	return Coordinate{Lat: *place.Latitude + latOffset, Lon: *place.Longitude + lonOffset}
}

func echoFingerprint(input EchoCreateInput) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(strings.Join([]string{
		input.AuthorUserID, input.Request.PlaceID, string(input.Request.ContentMode), input.Request.Title,
		input.Request.BodyText, input.Request.CaptionText, input.Request.CreationToken,
	}, "\x00")))
	return hex.EncodeToString(hash.Sum(nil))
}

type CreationAuthorization struct {
	ID                     string
	TokenHash              string
	UserID                 string
	PlaceID                string
	Coordinate             Coordinate
	CapturedAt             time.Time
	AccuracyMeters         float64
	ExpiresAt              time.Time
	ConsumedIdempotencyKey string
}

type CreationAuthorizationStore interface {
	Save(context.Context, CreationAuthorization) (CreationAuthorization, error)
	Consume(context.Context, string, string, string, string, time.Time) (CreationAuthorization, error)
}

type MemoryCreationAuthorizationStore struct {
	mu      sync.Mutex
	records map[string]CreationAuthorization
}

func NewMemoryCreationAuthorizationStore() *MemoryCreationAuthorizationStore {
	return &MemoryCreationAuthorizationStore{records: map[string]CreationAuthorization{}}
}

func (s *MemoryCreationAuthorizationStore) Save(_ context.Context, authorization CreationAuthorization) (CreationAuthorization, error) {
	if authorization.ID == "" {
		id, err := uuidv7.New()
		if err != nil {
			return CreationAuthorization{}, err
		}
		authorization.ID = id.String()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[authorization.TokenHash] = authorization
	return authorization, nil
}

func (s *MemoryCreationAuthorizationStore) Consume(_ context.Context, token, userID, placeID, idempotencyKey string, now time.Time) (CreationAuthorization, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	hash := hashCreationToken(token)
	authorization, ok := s.records[hash]
	if !ok || authorization.UserID != userID || authorization.PlaceID != placeID || !authorization.ExpiresAt.After(now.UTC()) {
		return CreationAuthorization{}, ErrCreationProofMissing
	}
	if authorization.ConsumedIdempotencyKey != "" && authorization.ConsumedIdempotencyKey != idempotencyKey {
		return CreationAuthorization{}, ErrCreationProofUsed
	}
	authorization.ConsumedIdempotencyKey = idempotencyKey
	s.records[hash] = authorization
	return authorization, nil
}

func hashCreationToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}
