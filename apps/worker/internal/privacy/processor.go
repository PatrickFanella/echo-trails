package privacy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ObjectWriter interface {
	PutObject(context.Context, string, string, string, []byte) error
	DeleteObject(context.Context, string, string) error
}

type Processor struct {
	pool    *pgxpool.Pool
	objects ObjectWriter
	bucket  string
}

var exportSectionQueries = map[string]string{
	"account": `SELECT jsonb_build_object('email', email, 'role', role, 'status', status, 'created_at', created_at, 'updated_at', updated_at) FROM users WHERE id = $1`,
	"profile": `SELECT jsonb_build_object('handle', p.handle, 'display_name', p.display_name, 'bio', p.bio, 'created_at', p.created_at) FROM user_profiles p WHERE p.user_id = $1`,
	"echoes":  `SELECT coalesce(jsonb_agg(jsonb_build_object('id', id, 'place_id', place_id, 'title', title, 'content_mode', content_mode, 'body_text', body_text, 'caption_text', caption_text, 'status', status, 'created_at', created_at) ORDER BY created_at), '[]') FROM echoes WHERE author_user_id = $1`,
	"replies": `SELECT coalesce(jsonb_agg(jsonb_build_object('id', id, 'echo_id', echo_id, 'body_text', body_text, 'status', status, 'created_at', created_at) ORDER BY created_at), '[]') FROM replies WHERE author_user_id = $1`,
	"reply_revisions": `SELECT coalesce(jsonb_agg(jsonb_build_object('reply_id', revision.reply_id, 'revision_number', revision.revision_number, 'body_text', revision.body_text, 'created_at', revision.created_at) ORDER BY revision.created_at), '[]')
		FROM reply_revisions revision JOIN replies reply ON reply.id = revision.reply_id WHERE reply.author_user_id = $1`,
	"media": `SELECT coalesce(jsonb_agg(jsonb_build_object('id', asset.id, 'echo_id', asset.echo_id, 'reply_id', asset.reply_id,
		'media_type', asset.media_type, 'content_type', asset.content_type, 'byte_size', asset.byte_size, 'duration_ms', asset.duration_ms,
		'width', asset.width, 'height', asset.height, 'status', asset.status, 'created_at', asset.created_at) ORDER BY asset.created_at), '[]')
		FROM media_assets asset WHERE asset.owner_user_id = $1`,
	"transcripts": `SELECT coalesce(jsonb_agg(jsonb_build_object('media_asset_id', transcript.media_asset_id, 'status', transcript.status,
		'locale', transcript.locale, 'text', transcript.transcript_text, 'created_at', transcript.created_at) ORDER BY transcript.created_at), '[]')
		FROM media_transcripts transcript JOIN media_assets asset ON asset.id = transcript.media_asset_id WHERE asset.owner_user_id = $1`,
	"reactions":     `SELECT coalesce(jsonb_agg(jsonb_build_object('echo_id', echo_id, 'reaction_type', reaction_type, 'created_at', created_at) ORDER BY created_at), '[]') FROM reactions WHERE user_id = $1`,
	"reports":       `SELECT coalesce(jsonb_agg(jsonb_build_object('id', id, 'echo_id', echo_id, 'reply_id', reply_id, 'reported_user_id', reported_user_id, 'category', category, 'details', details, 'status', status, 'created_at', created_at) ORDER BY created_at), '[]') FROM reports WHERE reporter_user_id = $1`,
	"subscriptions": `SELECT coalesce(jsonb_agg(jsonb_build_object('place_id', place_id, 'notifications_enabled', notifications_enabled, 'created_at', created_at)), '[]') FROM place_subscriptions WHERE user_id = $1`,
	"consents":      `SELECT coalesce(jsonb_agg(jsonb_build_object('type', consent_type, 'version', version, 'granted', granted, 'recorded_at', recorded_at)), '[]') FROM consents WHERE user_id = $1`,
	"notification_preferences": `SELECT coalesce(jsonb_build_object('direct_activity_enabled', direct_activity_enabled,
		'place_alerts_enabled', place_alerts_enabled, 'moderation_enabled', moderation_enabled, 'upload_status_enabled', upload_status_enabled,
		'updated_at', updated_at), '{}'::jsonb) FROM notification_preferences WHERE user_id = $1`,
	"notification_mutes": `SELECT coalesce(jsonb_agg(jsonb_build_object('place_id', place_id, 'echo_id', echo_id, 'muted_until', muted_until, 'created_at', created_at)), '[]') FROM notification_mutes WHERE user_id = $1`,
	"notifications": `SELECT coalesce(jsonb_agg(jsonb_build_object('id', id, 'type', type, 'echo_id', echo_id, 'reply_id', reply_id,
		'place_id', place_id, 'preview', payload->>'preview', 'created_at', created_at, 'read_at', read_at) ORDER BY created_at), '[]') FROM notifications WHERE user_id = $1`,
	"blocks":         `SELECT coalesce(jsonb_agg(jsonb_build_object('blocked_user_id', blocked_user_id, 'created_at', created_at)), '[]') FROM blocks WHERE blocker_user_id = $1`,
	"hidden_content": `SELECT coalesce(jsonb_agg(jsonb_build_object('target_type', target_type, 'target_id', target_id, 'created_at', created_at)), '[]') FROM hidden_targets WHERE user_id = $1`,
	"appeals":        `SELECT coalesce(jsonb_agg(jsonb_build_object('id', id, 'moderation_action_id', moderation_action_id, 'body_text', body_text, 'status', status, 'created_at', created_at, 'resolved_at', resolved_at) ORDER BY created_at), '[]') FROM appeals WHERE user_id = $1`,
	"moderation_actions": `SELECT coalesce(jsonb_agg(jsonb_build_object('id', action.id, 'action_type', action.action_type, 'reason_code', action.reason_code, 'created_at', action.created_at) ORDER BY action.created_at), '[]')
		FROM moderation_actions action WHERE action.target_user_id = $1 OR action.echo_id IN (SELECT id FROM echoes WHERE author_user_id = $1) OR action.reply_id IN (SELECT id FROM replies WHERE author_user_id = $1)`,
	"analytics": `SELECT coalesce(jsonb_agg(jsonb_build_object('event_name', event_name, 'occurred_at', occurred_at, 'properties', properties) ORDER BY occurred_at), '[]') FROM analytics_events WHERE user_id = $1`,
	"devices":   `SELECT coalesce(jsonb_agg(jsonb_build_object('id', id, 'platform', platform, 'environment', push_environment, 'app_version', app_version, 'locale', locale, 'timezone', timezone, 'created_at', created_at, 'last_seen_at', last_seen_at, 'disabled_at', disabled_at) ORDER BY created_at), '[]') FROM devices WHERE user_id = $1`,
	"sessions":  `SELECT coalesce(jsonb_agg(jsonb_build_object('id', id, 'device_name', device_name, 'created_at', created_at, 'last_seen_at', last_seen_at, 'expires_at', expires_at, 'revoked_at', revoked_at) ORDER BY created_at), '[]') FROM sessions WHERE user_id = $1`,
}

func NewProcessor(pool *pgxpool.Pool, objects ObjectWriter, bucket string) (*Processor, error) {
	if pool == nil || objects == nil || bucket == "" {
		return nil, errors.New("privacy processor requires Postgres and object storage")
	}
	return &Processor{pool: pool, objects: objects, bucket: bucket}, nil
}

func (p *Processor) Process(ctx context.Context, jobID, kind string) error {
	if kind == "privacy_export" {
		return p.export(ctx, jobID)
	}
	if kind == "privacy_delete" {
		return p.delete(ctx, jobID)
	}
	return errors.New("unsupported privacy job")
}

func (p *Processor) CleanupExpiredExports(ctx context.Context, now time.Time, limit int) (int, error) {
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	rows, err := p.pool.Query(ctx, `SELECT id::text, result_object_key FROM privacy_jobs
		WHERE job_type = 'export' AND status = 'ready' AND expires_at <= $1 AND result_object_key IS NOT NULL
		ORDER BY expires_at LIMIT $2`, now.UTC(), limit)
	if err != nil {
		return 0, err
	}
	type expiredExport struct{ id, key string }
	values := []expiredExport{}
	for rows.Next() {
		var value expiredExport
		if err = rows.Scan(&value.id, &value.key); err != nil {
			rows.Close()
			return 0, err
		}
		values = append(values, value)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return 0, err
	}
	cleaned := 0
	for _, value := range values {
		if err = p.objects.DeleteObject(ctx, p.bucket, value.key); err != nil {
			return cleaned, err
		}
		command, updateErr := p.pool.Exec(ctx, `UPDATE privacy_jobs SET status = 'completed', result_object_key = NULL
			WHERE id = $1 AND status = 'ready'`, value.id)
		if updateErr != nil {
			return cleaned, updateErr
		}
		if command.RowsAffected() == 1 {
			cleaned++
		}
	}
	return cleaned, nil
}

func (p *Processor) export(ctx context.Context, jobID string) error {
	var userID string
	if err := p.pool.QueryRow(ctx, `UPDATE privacy_jobs SET status = 'processing' WHERE id = $1 AND job_type = 'export' RETURNING user_id::text`, jobID).Scan(&userID); err != nil {
		return err
	}
	sections := make(map[string]any, len(exportSectionQueries))
	for name, query := range exportSectionQueries {
		var raw []byte
		if err := p.pool.QueryRow(ctx, query, userID).Scan(&raw); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var value any
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &value)
		}
		sections[name] = value
	}
	data, err := marshalExportDocument(userID, time.Now().UTC(), sections)
	if err != nil {
		return err
	}
	objectKey := fmt.Sprintf("privacy/exports/%s/export.json", jobID)
	if err = p.objects.PutObject(ctx, p.bucket, objectKey, "application/json", data); err != nil {
		return err
	}
	_, err = p.pool.Exec(ctx, `UPDATE privacy_jobs SET status = 'ready', result_object_key = $2, completed_at = now(), expires_at = now() + interval '7 days' WHERE id = $1`, jobID, objectKey)
	return err
}

func marshalExportDocument(userID string, generatedAt time.Time, sections map[string]any) ([]byte, error) {
	document := map[string]any{"schema_version": 1, "generated_at": generatedAt.UTC(), "user_id": userID}
	for name, value := range sections {
		document[name] = value
	}
	return json.Marshal(document)
}

func (p *Processor) delete(ctx context.Context, jobID string) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var userID string
	if err = tx.QueryRow(ctx, `UPDATE privacy_jobs SET status = 'processing' WHERE id = $1 AND job_type = 'delete' RETURNING user_id::text`, jobID).Scan(&userID); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `WITH deletion_assets AS (
		SELECT id, storage_bucket, object_key FROM media_assets WHERE owner_user_id = $1
		UNION
		SELECT asset.id, asset.storage_bucket, asset.object_key FROM media_assets asset
		JOIN echo_links link ON link.quote_snapshot_media_asset_id = asset.id
		JOIN replies reply ON reply.id = link.quoted_reply_id WHERE reply.author_user_id = $1
	), deletion_objects AS (
		SELECT storage_bucket, object_key FROM deletion_assets
		UNION
		SELECT derivative.storage_bucket, derivative.object_key FROM media_derivatives derivative
		JOIN deletion_assets asset ON asset.id = derivative.media_asset_id
		UNION
		SELECT $2::text, result_object_key FROM privacy_jobs WHERE user_id = $1 AND result_object_key IS NOT NULL
	)
	SELECT storage_bucket, object_key FROM deletion_objects ORDER BY storage_bucket, object_key`, userID, p.bucket)
	if err != nil {
		return err
	}
	objects := []storedObject{}
	for rows.Next() {
		var object storedObject
		if err = rows.Scan(&object.bucket, &object.key); err != nil {
			rows.Close()
			return err
		}
		objects = append(objects, object)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	if err = deleteStoredObjects(ctx, p.objects, objects); err != nil {
		return err
	}
	// Privacy deletion overrides ordinary snapshot immutability.
	if _, err = tx.Exec(ctx, `UPDATE media_assets SET status = 'deleted', deleted_at = now()
		WHERE id IN (
			SELECT quote_snapshot_media_asset_id FROM echo_links
			WHERE quoted_reply_id IN (SELECT id FROM replies WHERE author_user_id = $1)
			AND quote_snapshot_media_asset_id IS NOT NULL
		)`, userID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE echo_links link SET snapshot_status = 'suppressed', snapshot_suppressed_at = now(),
		snapshot_suppression_reason = 'privacy_deletion', quote_snapshot_media_asset_id = NULL,
		metadata = metadata - 'quoted_reply_body' - 'quoted_reply_author_handle'
		WHERE link.quoted_reply_id IN (SELECT id FROM replies WHERE author_user_id = $1)`, userID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE media_assets SET status = 'deleted', deleted_at = now() WHERE owner_user_id = $1`, userID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE replies SET status = 'deleted', body_text = 'Reply deleted', deleted_at = now(), updated_at = now() WHERE author_user_id = $1`, userID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE echoes SET status = 'deleted', title = 'Deleted Echo',
		body_text = CASE WHEN content_mode = 'text' THEN 'Echo deleted' ELSE NULL END,
		caption_text = NULL, deleted_at = now(), updated_at = now() WHERE author_user_id = $1`, userID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE devices SET disabled_at = now(), push_token_ciphertext = NULL WHERE user_id = $1`, userID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM auth_identities WHERE user_id = $1`, userID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE user_profiles SET handle = 'deleted_' || left(replace(user_id::text, '-', ''), 8), display_name = 'Deleted user', bio = NULL, avatar_media_asset_id = NULL, updated_at = now() WHERE user_id = $1`, userID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE users SET email = NULL, status = 'deleted', deleted_at = now(), updated_at = now() WHERE id = $1`, userID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE privacy_jobs SET result_object_key = NULL, expires_at = NULL WHERE user_id = $1`, userID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE privacy_jobs SET status = 'completed', completed_at = now() WHERE id = $1`, jobID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type storedObject struct{ bucket, key string }

func deleteStoredObjects(ctx context.Context, objects ObjectWriter, values []storedObject) error {
	for _, value := range values {
		if value.bucket == "" || value.key == "" {
			continue
		}
		if err := objects.DeleteObject(ctx, value.bucket, value.key); err != nil {
			return err
		}
	}
	return nil
}
