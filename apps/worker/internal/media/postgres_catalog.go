package media

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"git.subcult.tv/subculture-collective/echo-trails/apps/worker/internal/identifier"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ObjectWriter interface {
	PutObject(context.Context, string, string, string, []byte) error
	DeleteObject(context.Context, string, string) error
}

func (c *PostgresCatalog) CleanupExpiredOriginals(ctx context.Context, now time.Time, limit int) (int, error) {
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	rows, err := c.pool.Query(ctx, `SELECT id::text, storage_bucket, object_key FROM media_assets
		WHERE original_deleted_at IS NULL AND media_type <> 'quote_snapshot' AND moderation_hold = false
		AND ((original_expires_at IS NOT NULL AND original_expires_at <= $1)
			OR (status IN ('pending_upload', 'uploaded', 'failed') AND created_at <= $1 - interval '14 days'))
		ORDER BY coalesce(original_expires_at, created_at) LIMIT $2`, now.UTC(), limit)
	if err != nil {
		return 0, err
	}
	type original struct{ id, bucket, key string }
	originals := []original{}
	for rows.Next() {
		var value original
		if err = rows.Scan(&value.id, &value.bucket, &value.key); err != nil {
			rows.Close()
			return 0, err
		}
		originals = append(originals, value)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return 0, err
	}
	deleted := 0
	for _, value := range originals {
		if err = c.objects.DeleteObject(ctx, value.bucket, value.key); err != nil {
			return deleted, err
		}
		command, updateErr := c.pool.Exec(ctx, `UPDATE media_assets SET original_deleted_at = $2, updated_at = $2 WHERE id = $1 AND original_deleted_at IS NULL`, value.id, now.UTC())
		if updateErr != nil {
			return deleted, updateErr
		}
		if command.RowsAffected() == 1 {
			deleted++
		}
	}
	return deleted, nil
}

type PostgresCatalog struct {
	pool    *pgxpool.Pool
	objects ObjectWriter
	bucket  string
}

func (c *PostgresCatalog) LoadEchoText(ctx context.Context, echoID string) (EchoText, error) {
	var content EchoText
	err := c.pool.QueryRow(ctx, `SELECT id::text, title, coalesce(body_text, ''), coalesce(caption_text, '')
		FROM echoes WHERE id = $1 AND status IN ('uploading', 'processing', 'pending_review')`, echoID).
		Scan(&content.EchoID, &content.Title, &content.Body, &content.Caption)
	if errors.Is(err, pgx.ErrNoRows) {
		return EchoText{}, ErrUnsupportedMedia
	}
	return content, err
}

func (c *PostgresCatalog) SaveEchoModerationReview(ctx context.Context, review EchoModerationReview) error {
	id, err := randomUUID()
	if err != nil {
		return err
	}
	categories, err := json.Marshal(review.Categories)
	if err != nil {
		return err
	}
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `INSERT INTO echo_moderation_reviews
		(id, echo_id, status, risk_level, categories, scanner_name, failure_reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (echo_id, scanner_name) DO UPDATE SET status = EXCLUDED.status,
		risk_level = EXCLUDED.risk_level, categories = EXCLUDED.categories,
		failure_reason = EXCLUDED.failure_reason, updated_at = now()`, id, review.EchoID,
		review.Status, review.RiskLevel, categories, review.ScannerName, review.FailureReason)
	if err != nil {
		return err
	}
	status := map[string]string{ModerationStatusApproved: "published", ModerationStatusRejected: "limited", ModerationStatusNeedsReview: "pending_review"}[review.Status]
	if status == "" {
		status = "pending_review"
	}
	if status == "published" {
		ready, readyErr := quoteSnapshotPublicationReady(ctx, tx, review.EchoID)
		if readyErr != nil {
			return readyErr
		}
		if !ready {
			status = "processing"
		}
	}
	command, err := tx.Exec(ctx, `UPDATE echoes SET status = CASE WHEN content_mode = 'text' OR $2 <> 'published' THEN $2 ELSE status END,
		published_at = CASE WHEN content_mode = 'text' AND $2 = 'published' THEN coalesce(published_at, now()) ELSE published_at END,
		updated_at = now() WHERE id = $1 AND status IN ('uploading', 'processing', 'pending_review')`, review.EchoID, status)
	if err != nil {
		return err
	}
	var contentMode string
	if err = tx.QueryRow(ctx, `SELECT content_mode FROM echoes WHERE id = $1`, review.EchoID).Scan(&contentMode); err != nil {
		return err
	}
	if status == "published" && contentMode == "text" && command.RowsAffected() == 1 {
		jobID, idErr := randomUUID()
		if idErr != nil {
			return idErr
		}
		if _, err = tx.Exec(ctx, `INSERT INTO jobs (id, queue, job_type, payload, max_attempts)
			VALUES ($1, 'ranking', 'rank_echo', jsonb_build_object('echo_id', $2::text), 3)`, jobID, review.EchoID); err != nil {
			return err
		}
	}
	if status == "limited" {
		if err = ensureAutomatedModerationAction(ctx, tx, review.EchoID); err != nil {
			return err
		}
	}
	if contentMode != "text" {
		var assetID string
		if err = tx.QueryRow(ctx, `SELECT id::text FROM media_assets WHERE echo_id = $1 AND deleted_at IS NULL ORDER BY id LIMIT 1`, review.EchoID).Scan(&assetID); err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		} else if err = c.reconcileMediaTx(ctx, tx, assetID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func NewPostgresCatalog(pool *pgxpool.Pool, objects ObjectWriter, bucket string) (*PostgresCatalog, error) {
	if pool == nil || objects == nil || bucket == "" {
		return nil, errors.New("postgres, object writer, and bucket are required")
	}
	return &PostgresCatalog{pool: pool, objects: objects, bucket: bucket}, nil
}

func (c *PostgresCatalog) LoadAsset(ctx context.Context, assetID string) (Asset, error) {
	var asset Asset
	err := c.pool.QueryRow(ctx, `SELECT id::text, media_type, content_type, storage_bucket, object_key, status, coalesce(byte_size, 0)
		FROM media_assets WHERE id = $1 AND deleted_at IS NULL`, assetID).
		Scan(&asset.ID, &asset.MediaType, &asset.ContentType, &asset.StorageBucket, &asset.ObjectKey, &asset.Status, &asset.ByteSize)
	if errors.Is(err, pgx.ErrNoRows) {
		return Asset{}, ErrUnsupportedMedia
	}
	return asset, err
}

func (c *PostgresCatalog) SaveResult(ctx context.Context, result Result) error {
	for _, derivative := range result.Derivatives {
		if len(derivative.Data) == 0 {
			return errors.New("processed derivative body is empty")
		}
		if err := c.objects.PutObject(ctx, c.bucket, derivative.ObjectKey, derivative.ContentType, derivative.Data); err != nil {
			return err
		}
	}
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `DELETE FROM media_moderation_reviews WHERE media_asset_id = $1 AND scanner_name = 'processing-failure'`, result.AssetID); err != nil {
		return err
	}
	var bucket string
	if err = tx.QueryRow(ctx, `UPDATE media_assets SET status = $2, sha256_hash = $3,
		duration_ms = nullif($4, 0), width = nullif($5, 0), height = nullif($6, 0),
		verified_content_type = $7, malware_scan_status = $8,
		original_expires_at = now() + interval '7 days', updated_at = now()
		WHERE id = $1 RETURNING storage_bucket`, result.AssetID, result.Status, result.SHA256, result.DurationMS, result.Width, result.Height,
		result.VerifiedContentType, result.MalwareScanStatus).Scan(&bucket); err != nil {
		return err
	}
	for _, derivative := range result.Derivatives {
		id, idErr := randomUUID()
		if idErr != nil {
			return idErr
		}
		_, err = tx.Exec(ctx, `INSERT INTO media_derivatives
			(id, media_asset_id, derivative_type, storage_bucket, object_key, content_type, byte_size)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (storage_bucket, object_key) DO UPDATE SET content_type = EXCLUDED.content_type, byte_size = EXCLUDED.byte_size`,
			id, result.AssetID, derivative.DerivativeType, bucket, derivative.ObjectKey, derivative.ContentType, len(derivative.Data))
		if err != nil {
			return err
		}
	}
	if err = c.reconcileMediaTx(ctx, tx, result.AssetID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (c *PostgresCatalog) SaveProcessingFailure(ctx context.Context, assetID string, processingErr error) error {
	reason := "media processing failed"
	if processingErr != nil {
		reason = processingErr.Error()
		if len(reason) > 500 {
			reason = reason[:500]
		}
	}
	return c.SaveModerationReview(ctx, ModerationReview{
		AssetID: assetID, Status: ModerationStatusFailed, RiskLevel: RiskUnknown,
		ScannerName: "processing-failure", FailureReason: reason,
	})
}

func (c *PostgresCatalog) SaveTranscript(ctx context.Context, transcript Transcript) error {
	id, err := randomUUID()
	if err != nil {
		return err
	}
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `INSERT INTO media_transcripts
		(id, media_asset_id, status, locale, transcript_text, confidence, model_name, failure_reason, updated_at)
		VALUES ($1, $2, $3, $4, $5, nullif($6, 0), $7, $8, now())
		ON CONFLICT (media_asset_id, locale) DO UPDATE SET status = EXCLUDED.status,
		transcript_text = EXCLUDED.transcript_text, confidence = EXCLUDED.confidence,
		model_name = EXCLUDED.model_name, failure_reason = EXCLUDED.failure_reason, updated_at = now()`,
		id, transcript.AssetID, transcript.Status, transcript.Locale, transcript.Text, transcript.Confidence, transcript.ModelName, transcript.FailureReason)
	if err != nil {
		return err
	}
	if err = c.reconcileMediaTx(ctx, tx, transcript.AssetID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (c *PostgresCatalog) SaveModerationReview(ctx context.Context, review ModerationReview) error {
	id, err := randomUUID()
	if err != nil {
		return err
	}
	categories, err := json.Marshal(review.Categories)
	if err != nil {
		return err
	}
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `INSERT INTO media_moderation_reviews
		(id, media_asset_id, status, risk_level, categories, scanner_name, failure_reason, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		ON CONFLICT (media_asset_id, scanner_name) DO UPDATE SET status = EXCLUDED.status,
		risk_level = EXCLUDED.risk_level, categories = EXCLUDED.categories,
		failure_reason = EXCLUDED.failure_reason, updated_at = now()`,
		id, review.AssetID, review.Status, review.RiskLevel, categories, review.ScannerName, review.FailureReason)
	if err != nil {
		return err
	}
	if err = c.reconcileMediaTx(ctx, tx, review.AssetID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (c *PostgresCatalog) reconcileMediaTx(ctx context.Context, tx pgx.Tx, assetID string) error {
	var echoID *string
	if err := tx.QueryRow(ctx, `SELECT echo_id::text FROM media_assets WHERE id = $1`, assetID).Scan(&echoID); err != nil {
		return err
	}
	// A low-risk verdict cannot approve the original. Approval requires at least
	// one metadata-safe derivative created by process_media.
	_, err := tx.Exec(ctx, `WITH review AS (
		SELECT status FROM media_moderation_reviews WHERE media_asset_id = $1
		ORDER BY updated_at DESC LIMIT 1
	) UPDATE media_assets a SET
		status = CASE
			WHEN review.status = 'approved' AND EXISTS (SELECT 1 FROM media_derivatives d WHERE d.media_asset_id = a.id) THEN 'approved'
			WHEN review.status = 'rejected' THEN 'rejected'
			WHEN review.status IN ('needs_review', 'failed') THEN 'pending_review'
			ELSE a.status END,
		moderation_hold = review.status IN ('needs_review', 'failed', 'rejected'),
		processed_at = CASE WHEN EXISTS (SELECT 1 FROM media_derivatives d WHERE d.media_asset_id = a.id) THEN coalesce(a.processed_at, now()) ELSE a.processed_at END,
		updated_at = now()
	FROM review WHERE a.id = $1`, assetID)
	if err != nil || echoID == nil {
		return err
	}

	var total, rejected, held, incomplete, transcriptIncomplete, transcriptFailed int
	err = tx.QueryRow(ctx, `SELECT count(*),
		count(*) FILTER (WHERE a.status = 'rejected'),
		count(*) FILTER (WHERE a.status = 'pending_review' OR a.moderation_hold),
		count(*) FILTER (WHERE NOT EXISTS (SELECT 1 FROM media_derivatives d WHERE d.media_asset_id = a.id)),
		count(*) FILTER (WHERE a.media_type IN ('audio', 'video') AND NOT EXISTS (
			SELECT 1 FROM media_transcripts t WHERE t.media_asset_id = a.id AND t.status = 'succeeded')),
		count(*) FILTER (WHERE a.media_type IN ('audio', 'video') AND EXISTS (
			SELECT 1 FROM media_transcripts t WHERE t.media_asset_id = a.id AND t.status = 'failed'))
		FROM media_assets a WHERE a.echo_id = $1 AND a.media_type <> 'quote_snapshot'
		AND a.deleted_at IS NULL AND a.status <> 'deleted'`, *echoID).
		Scan(&total, &rejected, &held, &incomplete, &transcriptIncomplete, &transcriptFailed)
	if err != nil || total == 0 {
		return err
	}
	var textStatus string
	if err = tx.QueryRow(ctx, `SELECT coalesce((SELECT status FROM echo_moderation_reviews WHERE echo_id = $1 ORDER BY updated_at DESC LIMIT 1), 'pending')`, *echoID).Scan(&textStatus); err != nil {
		return err
	}
	status := "processing"
	if rejected > 0 || textStatus == ModerationStatusRejected {
		status = "limited"
	} else if held > 0 || transcriptFailed > 0 || textStatus == ModerationStatusNeedsReview || textStatus == ModerationStatusFailed {
		status = "pending_review"
	} else if incomplete == 0 && transcriptIncomplete == 0 && textStatus == ModerationStatusApproved {
		ready, readyErr := quoteSnapshotPublicationReady(ctx, tx, *echoID)
		if readyErr != nil {
			return readyErr
		}
		if ready {
			status = "published"
		}
	}
	if status == "published" {
		command, updateErr := tx.Exec(ctx, `UPDATE echoes SET status = 'published', published_at = coalesce(published_at, now()), updated_at = now()
			WHERE id = $1 AND status IN ('uploading', 'processing', 'pending_review')`, *echoID)
		if updateErr != nil {
			return updateErr
		}
		if command.RowsAffected() == 1 {
			jobID, idErr := randomUUID()
			if idErr != nil {
				return idErr
			}
			_, err = tx.Exec(ctx, `INSERT INTO jobs (id, queue, job_type, payload, max_attempts)
				VALUES ($1, 'ranking', 'rank_echo', jsonb_build_object('echo_id', $2::text), 3)`, jobID, *echoID)
		}
		if err == nil {
			err = ensureUploadStatusNotification(ctx, tx, *echoID, status)
		}
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE echoes SET status = $2, updated_at = now()
		WHERE id = $1 AND status IN ('uploading', 'processing', 'pending_review', 'limited')`, *echoID, status)
	if err == nil && status == "limited" {
		err = ensureAutomatedModerationAction(ctx, tx, *echoID)
	}
	if err == nil && status == "pending_review" {
		err = ensureUploadStatusNotification(ctx, tx, *echoID, status)
	}
	return err
}

func ensureUploadStatusNotification(ctx context.Context, tx pgx.Tx, echoID, status string) error {
	preview := map[string]string{"published": "Your Echo is published.", "pending_review": "Your Echo is awaiting safety review."}[status]
	if preview == "" {
		return nil
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM notifications
		WHERE type = 'upload_status' AND echo_id = $1 AND payload->>'status' = $2)`, echoID, status).Scan(&exists); err != nil || exists {
		return err
	}
	notificationID, err := randomUUID()
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO notifications
		(id, user_id, type, echo_id, payload, expires_at, collapse_key)
		SELECT $1, author_user_id, 'upload_status', id, jsonb_build_object('preview', $2::text, 'status', $3::text),
			now() + interval '90 days', 'upload:' || id::text FROM echoes WHERE id = $4`, notificationID, preview, status, echoID); err != nil {
		return err
	}
	deliveries, err := tx.Exec(ctx, `INSERT INTO notification_deliveries (notification_id, device_id)
		SELECT $1, device.id FROM notifications notification JOIN devices device ON device.user_id = notification.user_id
		LEFT JOIN notification_preferences preference ON preference.user_id = device.user_id
		WHERE notification.id = $1 AND device.disabled_at IS NULL AND device.push_token_ciphertext IS NOT NULL
		AND coalesce(preference.upload_status_enabled, true) ON CONFLICT DO NOTHING`, notificationID)
	if err != nil || deliveries.RowsAffected() == 0 {
		return err
	}
	jobID, err := randomUUID()
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO jobs (id, queue, job_type, payload, max_attempts)
		VALUES ($1, 'notifications', 'send_push', jsonb_build_object('notification_id', $2::text), 5)`, jobID, notificationID)
	return err
}

func ensureAutomatedModerationAction(ctx context.Context, tx pgx.Tx, echoID string) error {
	var actionID string
	err := tx.QueryRow(ctx, `SELECT id::text FROM moderation_actions
		WHERE echo_id = $1 AND action_type = 'limit' AND reason_code = 'automated_safety_rejection'
		ORDER BY created_at DESC LIMIT 1`, echoID).Scan(&actionID)
	if errors.Is(err, pgx.ErrNoRows) {
		actionID, err = randomUUID()
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO moderation_actions
			(id, echo_id, action_type, reason_code, notes)
			VALUES ($1, $2, 'limit', 'automated_safety_rejection', 'Conservative automated review; user may appeal.')`, actionID, echoID); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM notifications
		WHERE type = 'moderation' AND payload->>'moderation_action_id' = $1)`, actionID).Scan(&exists); err != nil || exists {
		return err
	}
	notificationID, err := randomUUID()
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO notifications
		(id, user_id, type, echo_id, payload, expires_at, collapse_key)
		SELECT $1, author_user_id, 'moderation', id,
			jsonb_build_object('preview', 'A moderation decision is ready to review.', 'moderation_action_id', $2::text),
			now() + interval '90 days', 'moderation:' || id::text FROM echoes WHERE id = $3`, notificationID, actionID, echoID); err != nil {
		return err
	}
	deliveries, err := tx.Exec(ctx, `INSERT INTO notification_deliveries (notification_id, device_id)
		SELECT $1, device.id FROM notifications notification JOIN devices device ON device.user_id = notification.user_id
		LEFT JOIN notification_preferences preference ON preference.user_id = device.user_id
		WHERE notification.id = $1 AND device.disabled_at IS NULL AND device.push_token_ciphertext IS NOT NULL
		AND coalesce(preference.moderation_enabled, true) ON CONFLICT DO NOTHING`, notificationID)
	if err != nil || deliveries.RowsAffected() == 0 {
		return err
	}
	jobID, err := randomUUID()
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO jobs (id, queue, job_type, payload, max_attempts)
		VALUES ($1, 'notifications', 'send_push', jsonb_build_object('notification_id', $2::text), 5)`, jobID, notificationID)
	return err
}

func (c *PostgresCatalog) LoadQuoteSnapshot(ctx context.Context, linkID string) (QuoteSnapshot, error) {
	var snapshot QuoteSnapshot
	err := c.pool.QueryRow(ctx, `SELECT l.id::text, l.target_echo_id::text, e.author_user_id::text,
		coalesce(l.metadata->>'quoted_reply_body', ''), coalesce(l.metadata->>'quoted_reply_author_handle', ''),
		coalesce(l.metadata->>'source_title', ''), coalesce(l.metadata->>'source_author_handle', '')
		FROM echo_links l JOIN echoes e ON e.id = l.target_echo_id
		WHERE l.id = $1 AND l.link_type = 'stitch' AND l.quoted_reply_id IS NOT NULL AND l.snapshot_status IN ('queued', 'failed')`, linkID).
		Scan(&snapshot.LinkID, &snapshot.TargetEchoID, &snapshot.OwnerUserID, &snapshot.Body, &snapshot.ReplyAuthorHandle, &snapshot.SourceEchoTitle, &snapshot.SourceAuthorHandle)
	if errors.Is(err, pgx.ErrNoRows) {
		return QuoteSnapshot{}, ErrUnsupportedMedia
	}
	if err == nil {
		_, _ = c.pool.Exec(ctx, `UPDATE echo_links SET snapshot_status = 'rendering' WHERE id = $1`, linkID)
	}
	return snapshot, err
}

func (c *PostgresCatalog) SaveQuoteSnapshot(ctx context.Context, rendered RenderedQuoteSnapshot) error {
	assetID, err := randomUUID()
	if err != nil {
		return err
	}
	objectKey := fmt.Sprintf("media/derivatives/%s/quote.png", rendered.LinkID)
	if err = c.objects.PutObject(ctx, c.bucket, objectKey, "image/png", rendered.PNG); err != nil {
		return err
	}
	digest := sha256.Sum256(rendered.PNG)
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `INSERT INTO media_assets
		(id, owner_user_id, echo_id, media_type, storage_bucket, object_key, content_type, byte_size, width, height, status, sha256_hash, capture_source)
		VALUES ($1, $2, $3, 'quote_snapshot', $4, $5, 'image/png', $6, $7, $8, 'approved', $9, 'quote_renderer')
		ON CONFLICT (storage_bucket, object_key) DO NOTHING`, assetID, rendered.OwnerUserID, rendered.TargetEchoID,
		c.bucket, objectKey, len(rendered.PNG), rendered.Width, rendered.Height, digest[:])
	if err != nil {
		return err
	}
	var targetEchoID string
	err = tx.QueryRow(ctx, `UPDATE echo_links SET quote_snapshot_media_asset_id = (
		SELECT id FROM media_assets WHERE storage_bucket = $2 AND object_key = $3), snapshot_status = 'ready'
		WHERE id = $1 RETURNING target_echo_id::text`, rendered.LinkID, c.bucket, objectKey).Scan(&targetEchoID)
	if err != nil {
		return err
	}
	if err = c.reconcileAfterQuoteSnapshotTx(ctx, tx, targetEchoID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (c *PostgresCatalog) FailQuoteSnapshot(ctx context.Context, linkID string, snapshotErr error) error {
	reason := "snapshot rendering failed"
	if snapshotErr != nil {
		reason = snapshotErr.Error()
		if len(reason) > 500 {
			reason = reason[:500]
		}
	}
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var targetEchoID string
	if err = tx.QueryRow(ctx, `UPDATE echo_links SET snapshot_status = 'failed',
		metadata = jsonb_set(metadata, '{snapshot_failure}', to_jsonb($2::text), true)
		WHERE id = $1 RETURNING target_echo_id::text`, linkID, reason).Scan(&targetEchoID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE echoes SET status = 'pending_review', published_at = NULL, updated_at = now()
		WHERE id = $1 AND status IN ('uploading', 'processing', 'pending_review', 'published')`, targetEchoID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func quoteSnapshotPublicationReady(ctx context.Context, tx pgx.Tx, echoID string) (bool, error) {
	var ready bool
	err := tx.QueryRow(ctx, `SELECT NOT EXISTS (
		SELECT 1 FROM echo_links WHERE target_echo_id = $1 AND quoted_reply_id IS NOT NULL
		AND snapshot_status NOT IN ('ready', 'suppressed'))`, echoID).Scan(&ready)
	return ready, err
}

func (c *PostgresCatalog) reconcileAfterQuoteSnapshotTx(ctx context.Context, tx pgx.Tx, echoID string) error {
	var contentMode, textStatus string
	if err := tx.QueryRow(ctx, `SELECT e.content_mode,
		coalesce((SELECT status FROM echo_moderation_reviews review WHERE review.echo_id = e.id ORDER BY updated_at DESC LIMIT 1), 'pending')
		FROM echoes e WHERE e.id = $1`, echoID).Scan(&contentMode, &textStatus); err != nil {
		return err
	}
	if contentMode != "text" {
		var assetID string
		err := tx.QueryRow(ctx, `SELECT id::text FROM media_assets WHERE echo_id = $1
			AND media_type <> 'quote_snapshot' AND deleted_at IS NULL ORDER BY sort_order, id LIMIT 1`, echoID).Scan(&assetID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		return c.reconcileMediaTx(ctx, tx, assetID)
	}
	if textStatus != ModerationStatusApproved {
		return nil
	}
	command, err := tx.Exec(ctx, `UPDATE echoes SET status = 'published', published_at = coalesce(published_at, now()), updated_at = now()
		WHERE id = $1 AND status IN ('uploading', 'processing', 'pending_review')`, echoID)
	if err != nil || command.RowsAffected() == 0 {
		return err
	}
	jobID, err := randomUUID()
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO jobs (id, queue, job_type, payload, max_attempts)
		VALUES ($1, 'ranking', 'rank_echo', jsonb_build_object('echo_id', $2::text), 3)`, jobID, echoID)
	return err
}

func randomUUID() (string, error) {
	return identifier.NewUUIDv7()
}
