package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/primitives/pagination"
	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/primitives/uuidv7"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func (s *PostgresStore) CreateAdminSession(ctx context.Context, userID, tokenHash, csrfHash string, now time.Time) (AdminSessionRecord, error) {
	var session AdminSessionRecord
	err := s.pool.QueryRow(ctx, `SELECT id::text, role FROM users WHERE id = $1 AND role IN ('moderator', 'admin') AND status = 'active'`, userID).Scan(&session.UserID, &session.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return AdminSessionRecord{}, ErrAdminUnauthorized
	}
	if err != nil {
		return AdminSessionRecord{}, err
	}
	id, err := uuidv7.New()
	if err != nil {
		return AdminSessionRecord{}, err
	}
	session.UserID = userID
	session.ExpiresAt = now.Add(8 * time.Hour).UTC()
	_, err = s.pool.Exec(ctx, `INSERT INTO admin_sessions (id, user_id, session_token_hash, csrf_token_hash, expires_at)
		VALUES ($1, $2, $3, $4, $5)`, id.String(), userID, decodeHex(tokenHash), decodeHex(csrfHash), session.ExpiresAt)
	return session, err
}

func (s *PostgresStore) AuthenticateAdminSession(ctx context.Context, tokenHash, csrfHash string, now time.Time, requireCSRF bool) (AdminSessionRecord, error) {
	var session AdminSessionRecord
	query := `SELECT u.id::text, u.role, a.expires_at FROM admin_sessions a JOIN users u ON u.id = a.user_id
		WHERE a.session_token_hash = $1 AND a.revoked_at IS NULL AND a.expires_at > $2
		AND u.role IN ('moderator', 'admin') AND u.status = 'active'`
	args := []any{decodeHex(tokenHash), now.UTC()}
	if requireCSRF {
		query += ` AND a.csrf_token_hash = $3`
		args = append(args, decodeHex(csrfHash))
	}
	err := s.pool.QueryRow(ctx, query, args...).Scan(&session.UserID, &session.Role, &session.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return AdminSessionRecord{}, ErrAdminUnauthorized
	}
	if err == nil {
		_, _ = s.pool.Exec(ctx, `UPDATE admin_sessions SET last_seen_at = $2 WHERE session_token_hash = $1`, decodeHex(tokenHash), now.UTC())
	}
	return session, err
}

func (s *PostgresStore) RevokeAdminSession(ctx context.Context, tokenHash string, now time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE admin_sessions SET revoked_at = $2 WHERE session_token_hash = $1 AND revoked_at IS NULL`, decodeHex(tokenHash), now.UTC())
	return err
}

func (s *PostgresStore) ListAdminQueue(ctx context.Context, queue, cursorValue string, limit int) (AdminQueuePage, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	createdBefore := time.Now().UTC().Add(time.Hour)
	idBefore := "ffffffff-ffff-ffff-ffff-ffffffffffff"
	if cursorValue != "" {
		cursor, err := pagination.Decode(cursorValue)
		if err != nil {
			return AdminQueuePage{}, ErrAdminInvalid
		}
		createdBefore, idBefore = cursor.After, cursor.ID.String()
	}
	query, err := adminQueueQuery(queue)
	if err != nil {
		return AdminQueuePage{}, err
	}
	rows, err := s.pool.Query(ctx, query, createdBefore, idBefore, limit+1)
	if err != nil {
		return AdminQueuePage{}, err
	}
	defer rows.Close()
	items := make([]AdminQueueItem, 0, limit)
	for rows.Next() {
		var item AdminQueueItem
		var raw []byte
		if err = rows.Scan(&item.ID, &item.Kind, &item.Status, &item.Reason, &item.TargetType, &item.TargetID, &item.CreatedAt, &raw); err != nil {
			return AdminQueuePage{}, err
		}
		_ = json.Unmarshal(raw, &item.Metadata)
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return AdminQueuePage{}, err
	}
	page := AdminQueuePage{Items: items}
	if len(items) > limit {
		last := items[limit-1]
		id, parseErr := uuidv7.Parse(last.ID)
		if parseErr != nil {
			return AdminQueuePage{}, parseErr
		}
		page.Items = items[:limit]
		page.NextCursor, err = pagination.Encode(pagination.Cursor{After: last.CreatedAt, ID: id})
	}
	return page, err
}

func adminQueueQuery(queue string) (string, error) {
	suffix := ` WHERE (created_at, id) < ($1, $2::uuid) ORDER BY created_at DESC, id DESC LIMIT $3`
	switch queue {
	case "reports":
		return `SELECT id::text, 'report', status, category,
			CASE WHEN echo_id IS NOT NULL THEN 'echo' WHEN reply_id IS NOT NULL THEN 'reply' WHEN place_id IS NOT NULL THEN 'place' ELSE 'user' END,
			coalesce(echo_id, reply_id, place_id, reported_user_id)::text, created_at,
			jsonb_build_object('details', details, 'reporter_user_id', reporter_user_id) FROM reports` + suffix, nil
	case "media":
		return `SELECT id, kind, status, reason, target_type, target_id, created_at, metadata FROM (
			SELECT review.id, 'media_review'::text AS kind, review.status, review.risk_level AS reason,
				'media'::text AS target_type, review.media_asset_id AS target_id, review.created_at,
				jsonb_build_object('categories', review.categories, 'scanner_name', review.scanner_name,
					'failure_reason', review.failure_reason, 'echo_id', asset.echo_id) AS metadata
			FROM media_moderation_reviews review JOIN media_assets asset ON asset.id = review.media_asset_id
			WHERE review.status IN ('needs_review', 'failed')
			UNION ALL
			SELECT review.id, 'text_review'::text, review.status, review.risk_level, 'echo'::text,
				review.echo_id, review.created_at,
				jsonb_build_object('categories', review.categories, 'scanner_name', review.scanner_name,
					'failure_reason', review.failure_reason) AS metadata
			FROM echo_moderation_reviews review WHERE review.status IN ('needs_review', 'failed')
		) review_queue WHERE (created_at, id) < ($1, $2::uuid)
		ORDER BY created_at DESC, id DESC LIMIT $3`, nil
	case "places":
		return `SELECT id::text, 'place', status, CASE WHEN sensitive THEN 'sensitive' ELSE '' END, 'place', id::text, created_at,
			jsonb_build_object('name', name, 'category', category, 'radius_meters', default_radius_meters)
			FROM places` + suffix, nil
	case "jobs":
		return `SELECT id::text, 'job', status, coalesce(last_error, ''), 'job', id::text, created_at,
			jsonb_build_object('queue', queue, 'job_type', job_type, 'attempts', attempts, 'max_attempts', max_attempts)
			FROM jobs` + suffix, nil
	case "appeals":
		return `SELECT id::text, 'appeal', status, left(body_text, 300), 'moderation_action', moderation_action_id::text, created_at,
			jsonb_build_object('user_id', user_id)
			FROM appeals` + suffix, nil
	case "audit":
		return `SELECT id::text, 'audit', action, coalesce(reason, ''), coalesce(target_table, ''), coalesce(target_id::text, ''), created_at, metadata
			FROM audit_logs` + suffix, nil
	default:
		return "", ErrAdminInvalid
	}
}

func (s *PostgresStore) ApplyAdminMutation(ctx context.Context, actor AdminSessionRecord, resource string, request AdminMutationRequest, idempotencyKey string, now time.Time) error {
	if !request.Confirmed || strings.TrimSpace(request.ReasonCode) == "" || strings.TrimSpace(idempotencyKey) == "" {
		return ErrAdminInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	fresh, err := reserveAdminIdempotency(ctx, tx, actor.UserID, idempotencyKey, resource, request, now)
	if err != nil || !fresh {
		return err
	}
	actionID, err := uuidv7.New()
	if err != nil {
		return err
	}
	targetID := strings.TrimSpace(request.TargetID)
	switch resource {
	case "moderation":
		if err = applyModerationAction(ctx, tx, actionID.String(), actor.UserID, request, now); err != nil {
			return err
		}
	case "place":
		if actor.Role != "admin" || (request.Action != "approve" && request.Action != "restrict" && request.Action != "merge" && request.Action != "set_boundary" && request.Action != "set_radius") {
			return ErrAdminUnauthorized
		}
		var command pgconn.CommandTag
		var execErr error
		switch request.Action {
		case "set_boundary":
			if len(request.BoundaryGeoJSON) == 0 || request.RadiusMeters == nil || *request.RadiusMeters < 25 || *request.RadiusMeters > 500 {
				return ErrAdminInvalid
			}
			command, execErr = tx.Exec(ctx, `WITH candidate AS (
				SELECT ST_SetSRID(ST_GeomFromGeoJSON($2), 4326) AS geometry
			), valid AS (
				SELECT ST_Multi(geometry)::geography AS boundary FROM candidate
				WHERE ST_GeometryType(geometry) IN ('ST_Polygon', 'ST_MultiPolygon')
				AND ST_IsValid(geometry) AND NOT ST_IsEmpty(geometry)
				AND ST_Area(geometry::geography) BETWEEN 1 AND 25000000
			) UPDATE places SET boundary = valid.boundary, default_radius_meters = $3, updated_at = $4
			FROM valid WHERE places.id = $1`, targetID, string(request.BoundaryGeoJSON), *request.RadiusMeters, now.UTC())
		case "set_radius":
			if request.RadiusMeters == nil || *request.RadiusMeters < 25 || *request.RadiusMeters > 500 {
				return ErrAdminInvalid
			}
			command, execErr = tx.Exec(ctx, `UPDATE places SET boundary = NULL, default_radius_meters = $2, updated_at = $3 WHERE id = $1`, targetID, *request.RadiusMeters, now.UTC())
		default:
			status := map[string]string{"approve": "active", "restrict": "restricted", "merge": "merged"}[request.Action]
			if request.Action == "merge" && strings.TrimSpace(request.MergeIntoID) == "" {
				return ErrAdminInvalid
			}
			command, execErr = tx.Exec(ctx, `UPDATE places SET status = $2, merged_into_place_id = CASE WHEN $2 = 'merged' THEN nullif($3, '')::uuid ELSE NULL END, updated_at = $4 WHERE id = $1`, targetID, status, request.MergeIntoID, now.UTC())
		}
		if execErr != nil || command.RowsAffected() != 1 {
			if execErr != nil {
				return execErr
			}
			return ErrAdminNotFound
		}
	case "appeal":
		if request.Action != "accept" && request.Action != "reject" {
			return ErrAdminInvalid
		}
		command, execErr := tx.Exec(ctx, `UPDATE appeals SET status = $2, resolved_at = $3, resolved_by_user_id = $4 WHERE id = $1 AND status = 'open'`, targetID, request.Action+"ed", now.UTC(), actor.UserID)
		if execErr != nil || command.RowsAffected() != 1 {
			if execErr != nil {
				return execErr
			}
			return ErrAdminNotFound
		}
		if request.Action == "accept" {
			// Reversal is deliberately tied to the appealed moderation action and
			// committed with the appeal resolution and audit entry.
			execReversal := func(query string, args ...any) error {
				_, reversalErr := tx.Exec(ctx, query, args...)
				return reversalErr
			}
			if err = execReversal(`UPDATE echoes echo SET status = 'published', updated_at = $2
				FROM appeals appeal JOIN moderation_actions action ON action.id = appeal.moderation_action_id
				WHERE appeal.id = $1 AND action.echo_id = echo.id AND action.action_type IN ('limit', 'remove')`, targetID, now.UTC()); err != nil {
				return err
			}
			if err = execReversal(`UPDATE replies reply SET status = 'published', updated_at = $2
				FROM appeals appeal JOIN moderation_actions action ON action.id = appeal.moderation_action_id
				WHERE appeal.id = $1 AND action.reply_id = reply.id AND action.action_type IN ('limit', 'remove')`, targetID, now.UTC()); err != nil {
				return err
			}
			if err = execReversal(`UPDATE echo_links link SET
				snapshot_status = CASE WHEN link.quote_snapshot_media_asset_id IS NULL THEN 'queued' ELSE 'ready' END,
				snapshot_suppressed_at = NULL, snapshot_suppression_reason = NULL
				FROM appeals appeal JOIN moderation_actions action ON action.id = appeal.moderation_action_id
				WHERE appeal.id = $1 AND action.reply_id = link.quoted_reply_id
				AND link.snapshot_status = 'suppressed' AND link.snapshot_suppression_reason = 'moderator_safety_removal'`, targetID); err != nil {
				return err
			}
			if err = execReversal(`UPDATE users target SET status = 'active', updated_at = $2
				FROM appeals appeal JOIN moderation_actions action ON action.id = appeal.moderation_action_id
				WHERE appeal.id = $1 AND action.target_user_id = target.id
				AND action.action_type IN ('restrict_posting', 'suspend', 'ban')`, targetID, now.UTC()); err != nil {
				return err
			}
			if err = execReversal(`UPDATE user_trust_scores trust SET
				trust_tier = CASE WHEN target.created_at <= $2::timestamptz - interval '30 days' THEN 'established' ELSE 'new' END,
				weight = CASE WHEN target.created_at <= $2::timestamptz - interval '30 days' THEN 1.0 ELSE 0.5 END,
				reason = 'appeal_accepted', updated_at = $2
				FROM users target, appeals appeal, moderation_actions action
				WHERE appeal.id = $1 AND action.id = appeal.moderation_action_id
				AND action.target_user_id = target.id AND trust.user_id = target.id`, targetID, now.UTC()); err != nil {
				return err
			}
			if err = execReversal(`UPDATE places place SET status = 'active', updated_at = $2
				FROM appeals appeal JOIN moderation_actions action ON action.id = appeal.moderation_action_id
				WHERE appeal.id = $1 AND action.place_id = place.id AND action.action_type = 'place_restrict'`, targetID, now.UTC()); err != nil {
				return err
			}
		}
	case "review":
		if request.Action != "approve" && request.Action != "reject" {
			return ErrAdminInvalid
		}
		if err = applyAdminReview(ctx, tx, actionID.String(), actor.UserID, targetID, request, now); err != nil {
			return err
		}
	case "job":
		if actor.Role != "admin" || (request.Action != "retry" && request.Action != "cancel") {
			return ErrAdminUnauthorized
		}
		status := "cancelled"
		if request.Action == "retry" {
			status = "queued"
		}
		command, execErr := tx.Exec(ctx, `UPDATE jobs SET status = $2, run_after = CASE WHEN $2 = 'queued' THEN $3 ELSE run_after END,
			locked_by = NULL, locked_at = NULL, updated_at = $3 WHERE id = $1`, targetID, status, now.UTC())
		if execErr != nil || command.RowsAffected() != 1 {
			if execErr != nil {
				return execErr
			}
			return ErrAdminNotFound
		}
	default:
		return ErrAdminInvalid
	}
	auditID, err := uuidv7.New()
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_logs (id, actor_user_id, action, target_table, target_id, reason, metadata)
		VALUES ($1, $2, $3, $4, nullif($5, '')::uuid, $6, jsonb_build_object('notes', $7::text, 'idempotency_key', $8::text))`,
		auditID.String(), actor.UserID, request.Action, resource, targetID, request.ReasonCode, request.Notes, idempotencyKey)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func applyModerationAction(ctx context.Context, tx pgx.Tx, actionID, actorID string, request AdminMutationRequest, now time.Time) error {
	allowed := map[string]bool{"limit": true, "remove": true, "restore": true, "warn": true, "restrict_posting": true, "suspend": true, "ban": true, "sensitive_label": true, "place_restrict": true, "place_merge": true}
	if !allowed[request.Action] {
		return ErrAdminInvalid
	}
	columns := map[string]string{"user": "target_user_id", "echo": "echo_id", "reply": "reply_id", "place": "place_id"}
	column, ok := columns[request.TargetType]
	if !ok || request.TargetID == "" {
		return ErrAdminInvalid
	}
	query := `INSERT INTO moderation_actions (id, moderator_user_id, report_id, ` + column + `, action_type, reason_code, notes, created_at)
		VALUES ($1, $2, nullif($3, '')::uuid, $4, $5, $6, $7, $8)`
	if _, err := tx.Exec(ctx, query, actionID, actorID, request.ReportID, request.TargetID, request.Action, request.ReasonCode, request.Notes, now.UTC()); err != nil {
		return err
	}
	switch request.TargetType {
	case "echo":
		status := map[string]string{"limit": "limited", "remove": "removed", "restore": "published"}[request.Action]
		if status != "" {
			_, _ = tx.Exec(ctx, `UPDATE echoes SET status = $2, updated_at = $3 WHERE id = $1`, request.TargetID, status, now.UTC())
		}
		if request.Action == "sensitive_label" {
			_, _ = tx.Exec(ctx, `UPDATE echoes SET sensitive_label = 'moderator_labeled', updated_at = $2 WHERE id = $1`, request.TargetID, now.UTC())
		}
	case "reply":
		status := map[string]string{"limit": "limited", "remove": "removed", "restore": "published"}[request.Action]
		if status != "" {
			_, _ = tx.Exec(ctx, `UPDATE replies SET status = $2, updated_at = $3 WHERE id = $1`, request.TargetID, status, now.UTC())
		}
		if request.Action == "remove" {
			_, _ = tx.Exec(ctx, `UPDATE echo_links SET snapshot_status = 'suppressed', snapshot_suppressed_at = $2,
				snapshot_suppression_reason = 'moderator_safety_removal'
				WHERE quoted_reply_id = $1 AND snapshot_status IN ('queued', 'rendering', 'ready', 'failed')`, request.TargetID, now.UTC())
		}
	case "user":
		status := map[string]string{"restrict_posting": "restricted", "suspend": "suspended", "ban": "banned", "restore": "active"}[request.Action]
		if status != "" {
			_, _ = tx.Exec(ctx, `UPDATE users SET status = $2, updated_at = $3 WHERE id = $1`, request.TargetID, status, now.UTC())
			_, _ = tx.Exec(ctx, `INSERT INTO user_trust_scores (user_id, trust_tier, weight, reason, updated_at)
				SELECT id,
					CASE WHEN $2 = 'banned' THEN 'banned' WHEN $2 IN ('restricted', 'suspended') THEN 'restricted'
						WHEN created_at <= $3::timestamptz - interval '30 days' THEN 'established' ELSE 'new' END,
					CASE WHEN $2 = 'banned' THEN 0 WHEN $2 IN ('restricted', 'suspended') THEN 0.25
						WHEN created_at <= $3::timestamptz - interval '30 days' THEN 1.0 ELSE 0.5 END,
					'moderator_action', $3 FROM users WHERE id = $1
				ON CONFLICT (user_id) DO UPDATE SET trust_tier = EXCLUDED.trust_tier, weight = EXCLUDED.weight,
					reason = EXCLUDED.reason, updated_at = EXCLUDED.updated_at`, request.TargetID, status, now.UTC())
		}
	case "place":
		if request.Action == "place_restrict" {
			_, _ = tx.Exec(ctx, `UPDATE places SET status = 'restricted', updated_at = $2 WHERE id = $1`, request.TargetID, now.UTC())
		}
	}
	if request.ReportID != "" {
		_, _ = tx.Exec(ctx, `UPDATE reports SET status = 'actioned', resolved_at = $2 WHERE id = $1`, request.ReportID, now.UTC())
	}
	if request.TargetType != "place" {
		var affectedUserID, affectedEchoID string
		switch request.TargetType {
		case "echo":
			err := tx.QueryRow(ctx, `SELECT author_user_id::text, id::text FROM echoes WHERE id = $1`, request.TargetID).Scan(&affectedUserID, &affectedEchoID)
			if err != nil {
				return err
			}
		case "reply":
			err := tx.QueryRow(ctx, `SELECT author_user_id::text, echo_id::text FROM replies WHERE id = $1`, request.TargetID).Scan(&affectedUserID, &affectedEchoID)
			if err != nil {
				return err
			}
		case "user":
			affectedUserID = request.TargetID
		}
		if affectedUserID != "" {
			if err := insertModerationNotificationForUser(ctx, tx, actionID, affectedUserID, affectedEchoID, now); err != nil {
				return err
			}
		}
	}
	return nil
}

func applyAdminReview(ctx context.Context, tx pgx.Tx, actionID, actorID, reviewID string, request AdminMutationRequest, now time.Time) error {
	decision := map[string]string{"approve": "approved", "reject": "rejected"}[request.Action]
	risk := map[string]string{"approve": "low", "reject": "high"}[request.Action]
	var assetID string
	var echoID *string
	err := tx.QueryRow(ctx, `SELECT review.media_asset_id::text, asset.echo_id::text
		FROM media_moderation_reviews review JOIN media_assets asset ON asset.id = review.media_asset_id
		WHERE review.id = $1 AND review.status IN ('needs_review', 'failed') FOR UPDATE`, reviewID).Scan(&assetID, &echoID)
	if err == nil {
		if _, err = tx.Exec(ctx, `UPDATE media_moderation_reviews SET status = $2, risk_level = $3,
			failure_reason = '', updated_at = $4 WHERE id = $1`, reviewID, decision, risk, now.UTC()); err != nil {
			return err
		}
		if request.Action == "approve" {
			command, updateErr := tx.Exec(ctx, `UPDATE media_assets asset SET status = 'approved', moderation_hold = false, updated_at = $2
				WHERE asset.id = $1 AND asset.malware_scan_status = 'clean'
				AND EXISTS (SELECT 1 FROM media_derivatives derivative WHERE derivative.media_asset_id = asset.id)`, assetID, now.UTC())
			if updateErr != nil {
				return updateErr
			}
			if command.RowsAffected() != 1 {
				return ErrAdminInvalid
			}
		} else if _, err = tx.Exec(ctx, `UPDATE media_assets SET status = 'rejected', moderation_hold = true, updated_at = $2 WHERE id = $1`, assetID, now.UTC()); err != nil {
			return err
		}
		if echoID == nil {
			return nil // Avatar review: selection remains gated by the asset state.
		}
		return finalizeAdminEchoReview(ctx, tx, actionID, actorID, *echoID, request, now)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var textEchoID string
	err = tx.QueryRow(ctx, `SELECT echo_id::text FROM echo_moderation_reviews
		WHERE id = $1 AND status IN ('needs_review', 'failed') FOR UPDATE`, reviewID).Scan(&textEchoID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrAdminNotFound
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE echo_moderation_reviews SET status = $2, risk_level = $3,
		failure_reason = '', updated_at = $4 WHERE id = $1`, reviewID, decision, risk, now.UTC()); err != nil {
		return err
	}
	return finalizeAdminEchoReview(ctx, tx, actionID, actorID, textEchoID, request, now)
}

func finalizeAdminEchoReview(ctx context.Context, tx pgx.Tx, actionID, actorID, echoID string, request AdminMutationRequest, now time.Time) error {
	if request.Action == "reject" {
		if _, err := tx.Exec(ctx, `UPDATE echoes SET status = 'limited', updated_at = $2 WHERE id = $1`, echoID, now.UTC()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO moderation_actions
			(id, moderator_user_id, echo_id, action_type, reason_code, notes, created_at)
			VALUES ($1, $2, $3, 'limit', $4, $5, $6)`, actionID, actorID, echoID, request.ReasonCode, request.Notes, now.UTC()); err != nil {
			return err
		}
		return insertModerationNotification(ctx, tx, actionID, echoID, now)
	}
	var status string
	err := tx.QueryRow(ctx, `UPDATE echoes echo SET status = CASE WHEN
		coalesce((SELECT review.status FROM echo_moderation_reviews review WHERE review.echo_id = echo.id ORDER BY review.updated_at DESC LIMIT 1), 'pending') = 'approved'
		AND (echo.content_mode = 'text' OR (
			NOT EXISTS (SELECT 1 FROM media_assets asset WHERE asset.echo_id = echo.id AND asset.deleted_at IS NULL AND asset.status <> 'approved')
			AND NOT EXISTS (SELECT 1 FROM media_assets asset WHERE asset.echo_id = echo.id AND asset.media_type IN ('audio', 'video')
				AND NOT EXISTS (SELECT 1 FROM media_transcripts transcript WHERE transcript.media_asset_id = asset.id AND transcript.status = 'succeeded'))
		)) THEN 'published' ELSE 'pending_review' END,
		published_at = CASE WHEN coalesce((SELECT review.status FROM echo_moderation_reviews review WHERE review.echo_id = echo.id ORDER BY review.updated_at DESC LIMIT 1), 'pending') = 'approved'
			AND (echo.content_mode = 'text' OR NOT EXISTS (SELECT 1 FROM media_assets asset WHERE asset.echo_id = echo.id AND asset.deleted_at IS NULL AND asset.status <> 'approved'))
			THEN coalesce(echo.published_at, $2) ELSE echo.published_at END,
		updated_at = $2 WHERE echo.id = $1 RETURNING status`, echoID, now.UTC()).Scan(&status)
	if err != nil {
		return err
	}
	if status == "published" {
		jobID, idErr := uuidv7.New()
		if idErr != nil {
			return idErr
		}
		_, err = tx.Exec(ctx, `INSERT INTO jobs (id, queue, job_type, payload, max_attempts)
			VALUES ($1, 'ranking', 'rank_echo', jsonb_build_object('echo_id', $2::text), 3)`, jobID.String(), echoID)
	}
	return err
}

func insertModerationNotification(ctx context.Context, tx pgx.Tx, actionID, echoID string, now time.Time) error {
	var userID string
	if err := tx.QueryRow(ctx, `SELECT author_user_id::text FROM echoes WHERE id = $1`, echoID).Scan(&userID); err != nil {
		return err
	}
	return insertModerationNotificationForUser(ctx, tx, actionID, userID, echoID, now)
}

func insertModerationNotificationForUser(ctx context.Context, tx pgx.Tx, actionID, userID, echoID string, now time.Time) error {
	notificationID, err := uuidv7.New()
	if err != nil {
		return err
	}
	command, err := tx.Exec(ctx, `INSERT INTO notifications
		(id, user_id, type, echo_id, payload, expires_at, collapse_key)
		VALUES ($1, $2, 'moderation', nullif($3, '')::uuid,
			jsonb_build_object('preview', 'A moderation decision is ready to review.', 'moderation_action_id', $4::text),
			$5::timestamptz + interval '90 days', 'moderation:' || $4::text)`, notificationID.String(), userID, echoID, actionID, now.UTC())
	if err != nil || command.RowsAffected() != 1 {
		return err
	}
	deliveries, err := tx.Exec(ctx, `INSERT INTO notification_deliveries (notification_id, device_id)
		SELECT $1, device.id FROM notifications notification JOIN devices device ON device.user_id = notification.user_id
		LEFT JOIN notification_preferences preference ON preference.user_id = device.user_id
		WHERE notification.id = $1 AND device.disabled_at IS NULL AND device.push_token_ciphertext IS NOT NULL
		AND coalesce(preference.moderation_enabled, true) ON CONFLICT DO NOTHING`, notificationID.String())
	if err != nil {
		return err
	}
	if deliveries.RowsAffected() == 0 {
		return nil
	}
	jobID, err := uuidv7.New()
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO jobs (id, queue, job_type, payload, max_attempts)
		VALUES ($1, 'notifications', 'send_push', jsonb_build_object('notification_id', $2::text), 5)`, jobID.String(), notificationID.String())
	return err
}

func reserveAdminIdempotency(ctx context.Context, tx pgx.Tx, userID, key, resource string, request any, now time.Time) (bool, error) {
	raw, _ := json.Marshal(struct {
		Resource string `json:"resource"`
		Request  any    `json:"request"`
	}{resource, request})
	hash := sha256.Sum256(raw)
	id, err := uuidv7.New()
	if err != nil {
		return false, err
	}
	command, err := tx.Exec(ctx, `INSERT INTO idempotency_keys (id, user_id, key, request_hash, expires_at)
		VALUES ($1, $2, $3, $4, $5) ON CONFLICT (user_id, key) DO NOTHING`, id.String(), userID, key, hash[:], now.Add(24*time.Hour).UTC())
	if err != nil {
		return false, err
	}
	if command.RowsAffected() == 1 {
		return true, nil
	}
	var existing []byte
	if err = tx.QueryRow(ctx, `SELECT request_hash FROM idempotency_keys WHERE user_id = $1 AND key = $2`, userID, key).Scan(&existing); err != nil {
		return false, err
	}
	if string(existing) != string(hash[:]) {
		return false, ErrAdminInvalid
	}
	return false, nil
}

func (s *PostgresStore) CreateAdminInvite(ctx context.Context, actor AdminSessionRecord, request AdminInviteRequest, idempotencyKey string, now time.Time) error {
	if actor.Role != "admin" || !request.Confirmed || strings.TrimSpace(request.Code) == "" || strings.TrimSpace(request.ReasonCode) == "" || request.MaxUses < 1 || idempotencyKey == "" {
		return ErrAdminUnauthorized
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	fresh, err := reserveAdminIdempotency(ctx, tx, actor.UserID, idempotencyKey, "invite", request, now)
	if err != nil || !fresh {
		return err
	}
	id, err := uuidv7.New()
	if err != nil {
		return err
	}
	hash := sha256.Sum256([]byte(strings.ToUpper(strings.TrimSpace(request.Code))))
	if _, err = tx.Exec(ctx, `INSERT INTO invite_codes (id, code_hash, created_by_user_id, max_uses, expires_at) VALUES ($1, $2, $3, $4, $5)`, id.String(), hash[:], actor.UserID, request.MaxUses, request.ExpiresAt); err != nil {
		return err
	}
	auditID, err := uuidv7.New()
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_logs (id, actor_user_id, action, target_table, target_id, reason, metadata) VALUES ($1, $2, 'invite_create', 'invite_codes', $3, $4, jsonb_build_object('max_uses', $5))`, auditID.String(), actor.UserID, id.String(), request.ReasonCode, request.MaxUses)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) GetAdminMediaObject(ctx context.Context, actor AdminSessionRecord, reviewID string) (string, error) {
	if actor.Role != "moderator" && actor.Role != "admin" {
		return "", ErrAdminUnauthorized
	}
	var key string
	err := s.pool.QueryRow(ctx, `SELECT a.object_key FROM media_moderation_reviews r
		JOIN media_assets a ON a.id = r.media_asset_id
		WHERE r.id = $1 AND r.status IN ('needs_review', 'failed') AND a.deleted_at IS NULL`, reviewID).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrAdminNotFound
	}
	return key, err
}
