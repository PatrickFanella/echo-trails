package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/primitives/uuidv7"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ObjectInfo struct {
	ByteSize    int64
	ContentType string
}

type ObjectStorage interface {
	PresignPut(context.Context, string, string, string, int64, time.Duration) (string, error)
	PresignGet(context.Context, string, string, time.Duration) (string, error)
	Head(context.Context, string, string) (ObjectInfo, error)
}

func (s *S3ObjectStorage) PresignGet(ctx context.Context, bucket, key string, expires time.Duration) (string, error) {
	result, err := s.presigner.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)}, s3.WithPresignExpires(expires))
	if err != nil {
		return "", err
	}
	return result.URL, nil
}

type S3ObjectStorage struct {
	client    *s3.Client
	presigner *s3.PresignClient
}

func NewS3ObjectStorage(client *s3.Client) (*S3ObjectStorage, error) {
	if client == nil {
		return nil, errors.New("S3 client is required")
	}
	return &S3ObjectStorage{client: client, presigner: s3.NewPresignClient(client)}, nil
}

func (s *S3ObjectStorage) PresignPut(ctx context.Context, bucket, key, contentType string, byteSize int64, expires time.Duration) (string, error) {
	result, err := s.presigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket), Key: aws.String(key), ContentType: aws.String(contentType), ContentLength: aws.Int64(byteSize),
	}, s3.WithPresignExpires(expires))
	if err != nil {
		return "", err
	}
	return result.URL, nil
}

func (s *S3ObjectStorage) Head(ctx context.Context, bucket, key string) (ObjectInfo, error) {
	result, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return ObjectInfo{}, err
	}
	return ObjectInfo{ByteSize: aws.ToInt64(result.ContentLength), ContentType: aws.ToString(result.ContentType)}, nil
}

type PostgresMediaUploadStore struct {
	pool    *pgxpool.Pool
	storage ObjectStorage
	bucket  string
}

func NewPostgresMediaUploadStore(pool *pgxpool.Pool, storage ObjectStorage, bucket string) (*PostgresMediaUploadStore, error) {
	if pool == nil || storage == nil || strings.TrimSpace(bucket) == "" {
		return nil, errors.New("Postgres, object storage, and bucket are required")
	}
	return &PostgresMediaUploadStore{pool: pool, storage: storage, bucket: strings.TrimSpace(bucket)}, nil
}

func (s *PostgresMediaUploadStore) CreateUpload(ctx context.Context, userID string, request CreateMediaUploadRequest, idempotencyKey string, now time.Time) (MediaUploadResponse, error) {
	if err := validateMediaUpload(request); err != nil || strings.TrimSpace(userID) == "" || strings.TrimSpace(idempotencyKey) == "" {
		return MediaUploadResponse{}, ErrInvalidMediaUpload
	}
	scopedKey := userID + ":" + idempotencyKey
	if existing, found, err := s.findByIdempotencyKey(ctx, scopedKey, now); err != nil {
		return MediaUploadResponse{}, err
	} else if found {
		return existing, nil
	}
	assetID, err := uuidv7.New()
	if err != nil {
		return MediaUploadResponse{}, err
	}
	uploadID, err := uuidv7.New()
	if err != nil {
		return MediaUploadResponse{}, err
	}
	objectKey := fmt.Sprintf("media/original/%s/%s", assetID.String(), safeExtension(request.ContentType))
	expiresAt := now.Add(15 * time.Minute).UTC()
	uploadURL, err := s.storage.PresignPut(ctx, s.bucket, objectKey, request.ContentType, request.ByteSize, 15*time.Minute)
	if err != nil {
		return MediaUploadResponse{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return MediaUploadResponse{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var contentMode, status string
	if err = tx.QueryRow(ctx, `SELECT content_mode, status FROM echoes
		WHERE id = $1 AND author_user_id = $2 FOR UPDATE`, request.EchoID, userID).Scan(&contentMode, &status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MediaUploadResponse{}, ErrMediaUploadNotFound
		}
		return MediaUploadResponse{}, err
	}
	if status != EchoStatusDraft || !mediaMatchesEchoMode(request.MediaType, EchoContentMode(contentMode)) {
		return MediaUploadResponse{}, ErrInvalidMediaUpload
	}
	if err = s.validateAttachmentLimits(ctx, tx, request); err != nil {
		return MediaUploadResponse{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO media_assets
		(id, owner_user_id, echo_id, media_type, storage_bucket, object_key, content_type, byte_size, status, sort_order, original_expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'pending_upload', $9, $10)`, assetID.String(), userID, request.EchoID,
		request.MediaType, s.bucket, objectKey, request.ContentType, request.ByteSize, request.SortOrder, now.Add(14*24*time.Hour).UTC())
	if err != nil {
		return MediaUploadResponse{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO media_uploads
		(id, media_asset_id, upload_url, method, status, expires_at, idempotency_key)
		VALUES ($1, $2, $3, 'PUT', 'created', $4, $5)`, uploadID.String(), assetID.String(), uploadURL, expiresAt, scopedKey)
	if err != nil {
		return MediaUploadResponse{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return MediaUploadResponse{}, err
	}
	return MediaUploadResponse{UploadID: uploadID.String(), MediaAssetID: assetID.String(), UploadURL: uploadURL, Method: "PUT", ObjectKey: objectKey, Status: MediaUploadStatusCreated, ActivityStatus: MediaActivityStatusCreated, ExpiresAt: expiresAt}, nil
}

func (s *PostgresMediaUploadStore) CreateAvatarUpload(ctx context.Context, userID string, request CreateAvatarUploadRequest, idempotencyKey string, now time.Time) (MediaUploadResponse, error) {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(idempotencyKey) == "" || !validAvatarUpload(request) {
		return MediaUploadResponse{}, ErrInvalidMediaUpload
	}
	scopedKey := userID + ":avatar:" + idempotencyKey
	if existing, found, err := s.findByIdempotencyKey(ctx, scopedKey, now); err != nil {
		return MediaUploadResponse{}, err
	} else if found {
		return existing, nil
	}
	assetID, err := uuidv7.New()
	if err != nil {
		return MediaUploadResponse{}, err
	}
	uploadID, err := uuidv7.New()
	if err != nil {
		return MediaUploadResponse{}, err
	}
	objectKey := fmt.Sprintf("media/original/%s/%s", assetID.String(), safeExtension(request.ContentType))
	expiresAt := now.Add(15 * time.Minute).UTC()
	uploadURL, err := s.storage.PresignPut(ctx, s.bucket, objectKey, request.ContentType, request.ByteSize, 15*time.Minute)
	if err != nil {
		return MediaUploadResponse{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return MediaUploadResponse{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `INSERT INTO media_assets
		(id, owner_user_id, media_type, storage_bucket, object_key, content_type, byte_size, status, sort_order, original_expires_at)
		VALUES ($1, $2, 'image', $3, $4, $5, $6, 'pending_upload', 0, $7)`, assetID.String(), userID, s.bucket,
		objectKey, request.ContentType, request.ByteSize, now.Add(14*24*time.Hour).UTC()); err != nil {
		return MediaUploadResponse{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO media_uploads
		(id, media_asset_id, upload_url, method, status, expires_at, idempotency_key)
		VALUES ($1, $2, $3, 'PUT', 'created', $4, $5)`, uploadID.String(), assetID.String(), uploadURL, expiresAt, scopedKey); err != nil {
		return MediaUploadResponse{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return MediaUploadResponse{}, err
	}
	return MediaUploadResponse{UploadID: uploadID.String(), MediaAssetID: assetID.String(), UploadURL: uploadURL, Method: "PUT", ObjectKey: objectKey,
		Status: MediaUploadStatusCreated, ActivityStatus: MediaActivityStatusCreated, ExpiresAt: expiresAt}, nil
}

func (s *PostgresMediaUploadStore) ValidateAvatarSelection(ctx context.Context, userID, mediaAssetID string) error {
	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM media_assets asset WHERE asset.id = $1 AND asset.owner_user_id = $2
		AND asset.media_type = 'image' AND asset.status = 'approved' AND asset.deleted_at IS NULL
		AND EXISTS (SELECT 1 FROM media_derivatives derivative WHERE derivative.media_asset_id = asset.id
			AND derivative.derivative_type IN ('preview', 'thumbnail')))`, mediaAssetID, userID).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return ErrAvatarNotApproved
	}
	return nil
}

func (s *PostgresMediaUploadStore) GetUpload(ctx context.Context, userID, uploadID string) (MediaUploadResponse, error) {
	response, _, _, err := s.load(ctx, userID, uploadID)
	if err != nil {
		return MediaUploadResponse{}, err
	}
	if response.Status == MediaUploadStatusCreated && !response.ExpiresAt.After(time.Now().UTC()) {
		_, _ = s.pool.Exec(ctx, `UPDATE media_uploads SET status = 'expired', updated_at = now() WHERE id = $1 AND status = 'created'`, uploadID)
		response.Status = MediaUploadStatusExpired
		response.ActivityStatus = MediaActivityStatusExpired
		response.StatusReason = "upload expired before completion"
	}
	return safePersistentMediaResponse(response), nil
}

func (s *PostgresMediaUploadStore) CompleteUpload(ctx context.Context, userID, uploadID string) (MediaUploadResponse, error) {
	response, contentType, expectedBytes, err := s.load(ctx, userID, uploadID)
	if err != nil {
		return MediaUploadResponse{}, err
	}
	if response.Status == MediaUploadStatusUploaded {
		return safePersistentMediaResponse(response), nil
	}
	if !response.ExpiresAt.After(time.Now().UTC()) {
		_, _ = s.pool.Exec(ctx, `UPDATE media_uploads SET status = 'expired', updated_at = now() WHERE id = $1`, uploadID)
		return MediaUploadResponse{}, ErrMediaUploadExpired
	}
	object, err := s.storage.Head(ctx, s.bucket, response.ObjectKey)
	if err != nil || object.ByteSize != expectedBytes || !strings.EqualFold(strings.TrimSpace(object.ContentType), contentType) {
		return MediaUploadResponse{}, ErrInvalidMediaUpload
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return MediaUploadResponse{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `UPDATE media_uploads SET status = 'uploaded', upload_url = '', updated_at = now() WHERE id = $1`, uploadID); err != nil {
		return MediaUploadResponse{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE media_assets SET status = 'uploaded', updated_at = now() WHERE id = $1`, response.MediaAssetID); err != nil {
		return MediaUploadResponse{}, err
	}
	for _, jobType := range []string{"process_media", "generate_transcript", "scan_media_safety"} {
		jobID, idErr := uuidv7.New()
		if idErr != nil {
			return MediaUploadResponse{}, idErr
		}
		if _, err = tx.Exec(ctx, `INSERT INTO jobs (id, queue, job_type, payload, max_attempts)
			VALUES ($1, 'media', $2, jsonb_build_object('asset_id', $3::text), 3)`, jobID.String(), jobType, response.MediaAssetID); err != nil {
			return MediaUploadResponse{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return MediaUploadResponse{}, err
	}
	response.Status = MediaUploadStatusUploaded
	response.ActivityStatus = MediaActivityStatusProcessing
	response.TranscriptStatus = MediaTranscriptStatusPending
	response.ModerationStatus = MediaModerationStatusPending
	response.ModerationRisk = MediaModerationRiskUnknown
	response.StatusReason = "upload verified; processing queued"
	return safePersistentMediaResponse(response), nil
}

func (s *PostgresMediaUploadStore) RenewUpload(ctx context.Context, userID, uploadID string, now time.Time) (MediaUploadResponse, error) {
	response, contentType, expectedBytes, err := s.load(ctx, userID, uploadID)
	if err != nil {
		return MediaUploadResponse{}, err
	}
	if response.Status == MediaUploadStatusUploaded {
		return MediaUploadResponse{}, ErrInvalidMediaUpload
	}
	expiresAt := now.Add(15 * time.Minute).UTC()
	uploadURL, err := s.storage.PresignPut(ctx, s.bucket, response.ObjectKey, contentType, expectedBytes, 15*time.Minute)
	if err != nil {
		return MediaUploadResponse{}, err
	}
	command, err := s.pool.Exec(ctx, `UPDATE media_uploads SET upload_url = $3, status = 'created', expires_at = $4, updated_at = $5
		WHERE id = $1 AND media_asset_id = $2 AND status IN ('created', 'expired', 'failed')`, uploadID, response.MediaAssetID, uploadURL, expiresAt, now.UTC())
	if err != nil {
		return MediaUploadResponse{}, err
	}
	if command.RowsAffected() != 1 {
		return MediaUploadResponse{}, ErrInvalidMediaUpload
	}
	response.UploadURL = uploadURL
	response.Status = MediaUploadStatusCreated
	response.ActivityStatus = MediaActivityStatusCreated
	response.ExpiresAt = expiresAt
	response.StatusReason = "upload authorization renewed"
	return response, nil
}

func (s *PostgresMediaUploadStore) load(ctx context.Context, userID, uploadID string) (MediaUploadResponse, string, int64, error) {
	var response MediaUploadResponse
	var contentType string
	var byteSize int64
	err := s.pool.QueryRow(ctx, `SELECT u.id::text, a.id::text, u.upload_url, u.method, a.object_key, u.status, u.expires_at,
		a.content_type, a.byte_size FROM media_uploads u JOIN media_assets a ON a.id = u.media_asset_id
		WHERE u.id = $1 AND a.owner_user_id = $2 AND a.deleted_at IS NULL`, uploadID, userID).Scan(&response.UploadID,
		&response.MediaAssetID, &response.UploadURL, &response.Method, &response.ObjectKey, &response.Status, &response.ExpiresAt, &contentType, &byteSize)
	if errors.Is(err, pgx.ErrNoRows) {
		return MediaUploadResponse{}, "", 0, ErrMediaUploadNotFound
	}
	response.ActivityStatus = activityStatusForUpload(response.Status)
	return response, contentType, byteSize, err
}

func (s *PostgresMediaUploadStore) findByIdempotencyKey(ctx context.Context, key string, now time.Time) (MediaUploadResponse, bool, error) {
	var uploadID, ownerID string
	err := s.pool.QueryRow(ctx, `SELECT u.id::text, a.owner_user_id::text FROM media_uploads u JOIN media_assets a ON a.id = u.media_asset_id WHERE u.idempotency_key = $1`, key).Scan(&uploadID, &ownerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return MediaUploadResponse{}, false, nil
	}
	if err != nil {
		return MediaUploadResponse{}, false, err
	}
	response, err := s.GetUpload(ctx, ownerID, uploadID)
	return response, err == nil, err
}

func (s *PostgresMediaUploadStore) validateAttachmentLimits(ctx context.Context, query queryRower, request CreateMediaUploadRequest) error {
	var count int
	var totalBytes int64
	if err := query.QueryRow(ctx, `SELECT count(*), coalesce(sum(byte_size), 0) FROM media_assets
		WHERE echo_id = $1 AND media_type <> 'quote_snapshot'
		AND deleted_at IS NULL AND status <> 'deleted'`, request.EchoID).Scan(&count, &totalBytes); err != nil {
		return err
	}
	switch request.MediaType {
	case "image":
		if count >= 5 || request.ByteSize > 20*1024*1024 || totalBytes+request.ByteSize > 75*1024*1024 {
			return ErrInvalidMediaUpload
		}
	case "audio":
		if count != 0 || request.ByteSize > 25*1024*1024 {
			return ErrInvalidMediaUpload
		}
	case "video":
		if count != 0 || request.ByteSize > 100*1024*1024 {
			return ErrInvalidMediaUpload
		}
	default:
		return ErrInvalidMediaUpload
	}
	return nil
}

func mediaMatchesEchoMode(mediaType string, mode EchoContentMode) bool {
	return (mediaType == "image" && mode == EchoContentPhotoSet) ||
		(mediaType == "audio" && mode == EchoContentAudio) ||
		(mediaType == "video" && mode == EchoContentVideo)
}

func safePersistentMediaResponse(response MediaUploadResponse) MediaUploadResponse {
	if response.Status != MediaUploadStatusCreated || !response.ExpiresAt.After(time.Now().UTC()) {
		response.UploadURL = ""
	}
	return response
}

func (s *PostgresMediaUploadStore) GetPublicAssetURL(ctx context.Context, assetID string) (string, error) {
	var bucket, objectKey string
	err := s.pool.QueryRow(ctx, `SELECT d.storage_bucket, d.object_key FROM media_assets a
		LEFT JOIN echoes e ON e.id = a.echo_id
		JOIN LATERAL (
			SELECT storage_bucket, object_key FROM media_derivatives
			WHERE media_asset_id = a.id AND derivative_type IN ('preview', 'transcode', 'thumbnail')
			UNION ALL SELECT a.storage_bucket, a.object_key WHERE a.media_type = 'quote_snapshot'
			LIMIT 1
		) d ON true
		WHERE a.id = $1 AND a.status = 'approved' AND a.deleted_at IS NULL
		AND (a.echo_id IS NULL OR e.status = 'published')`, assetID).Scan(&bucket, &objectKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrMediaUploadNotFound
	}
	if err != nil {
		return "", err
	}
	return s.storage.PresignGet(ctx, bucket, objectKey, 15*time.Minute)
}

func (s *PostgresMediaUploadStore) GetPrivateObjectURL(ctx context.Context, objectKey string) (string, error) {
	if strings.TrimSpace(objectKey) == "" {
		return "", ErrMediaUploadNotFound
	}
	return s.storage.PresignGet(ctx, s.bucket, objectKey, 5*time.Minute)
}
