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
)

func (s *PostgresStore) ClaimInvite(ctx context.Context, userID, code, city string, now time.Time) (BetaAccessResponse, error) {
	if !strings.EqualFold(strings.TrimSpace(city), "Downers Grove") {
		return BetaAccessResponse{State: BetaAccessCityUnavailable, City: strings.TrimSpace(city), Message: "This city is not open for beta yet."}, ErrInviteInvalid
	}
	hash := sha256.Sum256([]byte(strings.ToUpper(strings.TrimSpace(code))))
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return BetaAccessResponse{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var inviteID string
	var maxUses, uses int
	var expiresAt *time.Time
	err = tx.QueryRow(ctx, `SELECT id::text, max_uses, uses_count, expires_at FROM invite_codes WHERE code_hash = $1 FOR UPDATE`, hash[:]).Scan(&inviteID, &maxUses, &uses, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) || uses >= maxUses || (expiresAt != nil && !expiresAt.After(now.UTC())) {
		return BetaAccessResponse{}, ErrInviteInvalid
	}
	if err != nil {
		return BetaAccessResponse{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE invite_codes SET uses_count = uses_count + 1,
		claimed_by_user_id = CASE WHEN max_uses = 1 THEN $2 ELSE claimed_by_user_id END,
		claimed_at = CASE WHEN max_uses = 1 THEN $3 ELSE claimed_at END WHERE id = $1`, inviteID, userID, now.UTC()); err != nil {
		return BetaAccessResponse{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE users SET beta_access_at = coalesce(beta_access_at, $2), updated_at = $2 WHERE id = $1`, userID, now.UTC()); err != nil {
		return BetaAccessResponse{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return BetaAccessResponse{}, err
	}
	return BetaAccessResponse{State: BetaAccessAccepted, City: "Downers Grove", Message: "Invite accepted."}, nil
}

func (s *PostgresStore) CreationAllowed(ctx context.Context, userID string) (bool, error) {
	var allowed bool
	err := s.pool.QueryRow(ctx, `SELECT beta_access_at IS NOT NULL AND status IN ('active', 'restricted') FROM users WHERE id = $1`, userID).Scan(&allowed)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return allowed, err
}

func (s *PostgresStore) GetAccountAccessState(ctx context.Context, userID string) (AccountAccessState, error) {
	current := CurrentConsentVersion()
	var state AccountAccessState
	err := s.pool.QueryRow(ctx, `SELECT
		u.beta_access_at IS NOT NULL AND u.status IN ('active', 'restricted'),
		EXISTS (SELECT 1 FROM consents terms WHERE terms.user_id = u.id AND terms.consent_type = 'terms' AND terms.version = $2 AND terms.granted)
		AND EXISTS (SELECT 1 FROM consents privacy WHERE privacy.user_id = u.id AND privacy.consent_type = 'privacy' AND privacy.version = $3 AND privacy.granted),
		EXISTS (SELECT 1 FROM user_profiles profile WHERE profile.user_id = u.id),
		EXISTS (SELECT 1 FROM user_acknowledgements acknowledgement WHERE acknowledgement.user_id = u.id
			AND acknowledgement.acknowledgement_type = 'stitch_reply_snapshot_warning')
		FROM users u WHERE u.id = $1`, userID, current.TermsVersion, current.PrivacyVersion).
		Scan(&state.BetaAccessAccepted, &state.ConsentAccepted, &state.ProfileConfigured, &state.QuoteWarningAcknowledged)
	if errors.Is(err, pgx.ErrNoRows) {
		return AccountAccessState{}, ErrAccountInvalid
	}
	return state, err
}

func (s *PostgresStore) AcceptConsent(ctx context.Context, userID string, request ConsentAcceptanceRequest, now time.Time) error {
	current := CurrentConsentVersion()
	if request.TermsVersion != current.TermsVersion || request.PrivacyVersion != current.PrivacyVersion {
		return ErrAccountInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, consent := range []struct{ kind, version string }{{"terms", request.TermsVersion}, {"privacy", request.PrivacyVersion}} {
		id, idErr := uuidv7.New()
		if idErr != nil {
			return idErr
		}
		if _, err = tx.Exec(ctx, `INSERT INTO consents (id, user_id, consent_type, version, granted, recorded_at, source)
			VALUES ($1, $2, $3, $4, true, $5, 'ios') ON CONFLICT (user_id, consent_type, version) DO UPDATE SET granted = true, recorded_at = EXCLUDED.recorded_at`, id.String(), userID, consent.kind, consent.version, now.UTC()); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) QueuePrivacyJob(ctx context.Context, userID, kind string, now time.Time) (PrivacyJobStatus, error) {
	if kind != "export" && kind != "delete" {
		return PrivacyJobStatus{}, ErrAccountInvalid
	}
	id, err := uuidv7.New()
	if err != nil {
		return PrivacyJobStatus{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return PrivacyJobStatus{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `INSERT INTO privacy_jobs (id, user_id, job_type, status, requested_at) VALUES ($1, $2, $3, 'queued', $4)`, id.String(), userID, kind, now.UTC()); err != nil {
		return PrivacyJobStatus{}, err
	}
	queueID, err := uuidv7.New()
	if err != nil {
		return PrivacyJobStatus{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO jobs (id, queue, job_type, payload, max_attempts)
		VALUES ($1, 'privacy', $2, jsonb_build_object('privacy_job_id', $3::text), 3)`, queueID.String(), "privacy_"+kind, id.String()); err != nil {
		return PrivacyJobStatus{}, err
	}
	if kind == "delete" {
		// Public removal and credential revocation are synchronous. The worker
		// performs retryable object deletion and final anonymization afterward.
		if _, err = tx.Exec(ctx, `UPDATE users SET status = 'deleted', deleted_at = coalesce(deleted_at, $2), updated_at = $2 WHERE id = $1`, userID, now.UTC()); err != nil {
			return PrivacyJobStatus{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE sessions SET revoked_at = coalesce(revoked_at, $2) WHERE user_id = $1`, userID, now.UTC()); err != nil {
			return PrivacyJobStatus{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE devices SET disabled_at = coalesce(disabled_at, $2), push_token_ciphertext = NULL WHERE user_id = $1`, userID, now.UTC()); err != nil {
			return PrivacyJobStatus{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE echoes SET status = 'deleted', title = 'Deleted Echo',
			body_text = CASE WHEN content_mode = 'text' THEN 'Echo deleted' ELSE NULL END,
			caption_text = NULL, deleted_at = coalesce(deleted_at, $2), updated_at = $2 WHERE author_user_id = $1`, userID, now.UTC()); err != nil {
			return PrivacyJobStatus{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE replies SET status = 'deleted', body_text = 'Reply deleted',
			deleted_at = coalesce(deleted_at, $2), updated_at = $2 WHERE author_user_id = $1`, userID, now.UTC()); err != nil {
			return PrivacyJobStatus{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE media_assets SET status = 'deleted', deleted_at = coalesce(deleted_at, $2)
			WHERE owner_user_id = $1 OR id IN (
				SELECT quote_snapshot_media_asset_id FROM echo_links
				WHERE quoted_reply_id IN (SELECT id FROM replies WHERE author_user_id = $1)
				AND quote_snapshot_media_asset_id IS NOT NULL
			)`, userID, now.UTC()); err != nil {
			return PrivacyJobStatus{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE echo_links SET snapshot_status = 'suppressed', snapshot_suppressed_at = $2,
			snapshot_suppression_reason = 'privacy_deletion',
			metadata = metadata - 'quoted_reply_body' - 'quoted_reply_author_handle'
			WHERE quoted_reply_id IN (SELECT id FROM replies WHERE author_user_id = $1)`, userID, now.UTC()); err != nil {
			return PrivacyJobStatus{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return PrivacyJobStatus{}, err
	}
	return PrivacyJobStatus{JobID: id.String(), UserID: userID, Kind: kind, Status: "queued", RequestedAt: now.UTC()}, nil
}

func (s *PostgresStore) GetPrivacyJob(ctx context.Context, userID, jobID string) (PrivacyJobStatus, error) {
	var job PrivacyJobStatus
	var objectKey string
	err := s.pool.QueryRow(ctx, `SELECT id::text, user_id::text, job_type, status, requested_at, coalesce(result_object_key, '')
		FROM privacy_jobs WHERE id = $1 AND user_id = $2`, jobID, userID).Scan(&job.JobID, &job.UserID, &job.Kind, &job.Status, &job.RequestedAt, &objectKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return PrivacyJobStatus{}, ErrEngagementNotFound
	}
	if err == nil && objectKey != "" {
		job.DownloadURL = "/privacy/exports/" + job.JobID + "/download"
		job.ObjectKey = objectKey
	}
	return job, err
}

func (s *PostgresStore) SetAnalyticsOptOut(ctx context.Context, userID string, optOut bool, now time.Time) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO user_privacy_settings (user_id, analytics_opt_out, updated_at) VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE SET analytics_opt_out = EXCLUDED.analytics_opt_out, updated_at = EXCLUDED.updated_at`, userID, optOut, now.UTC())
	return err
}

func (s *PostgresStore) GetAnalyticsOptOut(ctx context.Context, userID string) (bool, error) {
	var optedOut bool
	err := s.pool.QueryRow(ctx, `SELECT coalesce((SELECT analytics_opt_out FROM user_privacy_settings WHERE user_id = $1), false)`, userID).Scan(&optedOut)
	return optedOut, err
}

func (s *PostgresStore) RecordAnalyticsEvent(ctx context.Context, userID string, event AnalyticsEventEnvelope, now time.Time) error {
	if !event.SafeForCollection() {
		return ErrAccountInvalid
	}
	var optedOut bool
	if err := s.pool.QueryRow(ctx, `SELECT coalesce((SELECT analytics_opt_out FROM user_privacy_settings WHERE user_id = $1), false)`, userID).Scan(&optedOut); err != nil || optedOut {
		return err
	}
	properties, err := json.Marshal(event.Properties)
	if err != nil {
		return err
	}
	id, err := uuidv7.New()
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO analytics_events (id, user_id, event_name, occurred_at, properties)
		VALUES ($1, nullif($2, '')::uuid, $3, $4, $5)`, id.String(), userID, event.EventName, now.UTC(), properties)
	return err
}

func (s *PostgresStore) ListAccountSessionPage(ctx context.Context, userID, cursorValue string, limit int) (AccountSessionPage, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	query := `SELECT id::text, device_name, created_at, last_seen_at, expires_at FROM sessions
		WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > now()`
	args := []any{userID, limit + 1}
	if cursorValue != "" {
		cursor, decodeErr := pagination.Decode(cursorValue)
		if decodeErr != nil {
			return AccountSessionPage{}, decodeErr
		}
		query += ` AND (created_at, id) < ($3, $4)`
		args = append(args, cursor.After, cursor.ID.String())
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT $2`
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return AccountSessionPage{}, err
	}
	defer rows.Close()
	result := make([]AccountSession, 0, limit+1)
	for rows.Next() {
		var session AccountSession
		if err = rows.Scan(&session.ID, &session.DeviceName, &session.CreatedAt, &session.LastSeenAt, &session.ExpiresAt); err != nil {
			return AccountSessionPage{}, err
		}
		result = append(result, session)
	}
	if err = rows.Err(); err != nil {
		return AccountSessionPage{}, err
	}
	page := AccountSessionPage{Sessions: result}
	if len(result) > limit {
		page.Sessions = result[:limit]
		last := page.Sessions[len(page.Sessions)-1]
		id, parseErr := uuidv7.Parse(last.ID)
		if parseErr != nil {
			return AccountSessionPage{}, parseErr
		}
		page.NextCursor, err = pagination.Encode(pagination.Cursor{After: last.CreatedAt, ID: id})
		if err != nil {
			return AccountSessionPage{}, err
		}
	}
	return page, nil
}

func (s *PostgresStore) RevokeAccountSession(ctx context.Context, userID, sessionID string, now time.Time) error {
	command, err := s.pool.Exec(ctx, `UPDATE sessions SET revoked_at = $3 WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`, sessionID, userID, now.UTC())
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrEngagementNotFound
	}
	return nil
}
