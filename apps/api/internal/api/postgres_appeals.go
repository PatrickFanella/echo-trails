package api

import (
	"context"
	"errors"
	"strings"
	"time"

	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/primitives/pagination"
	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/primitives/uuidv7"
	"github.com/jackc/pgx/v5"
)

func (s *PostgresStore) CreateAppeal(ctx context.Context, userID string, request AppealRequest, idempotencyKey string, now time.Time) (Appeal, error) {
	request.Body = strings.TrimSpace(request.Body)
	if request.ModerationActionID == "" || request.Body == "" || len([]rune(request.Body)) > 2000 || idempotencyKey == "" {
		return Appeal{}, ErrAppealInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Appeal{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	fresh, err := reserveAdminIdempotency(ctx, tx, userID, idempotencyKey, "appeal_create", request, now)
	if err != nil {
		return Appeal{}, err
	}
	if !fresh {
		var existing Appeal
		err = tx.QueryRow(ctx, `SELECT id::text, moderation_action_id::text, body_text, status, created_at FROM appeals WHERE user_id = $1 AND moderation_action_id = $2 ORDER BY created_at DESC LIMIT 1`, userID, request.ModerationActionID).Scan(&existing.ID, &existing.ModerationActionID, &existing.Body, &existing.Status, &existing.CreatedAt)
		return existing, err
	}
	var owns bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM moderation_actions action
		LEFT JOIN echoes echo ON echo.id = action.echo_id
		LEFT JOIN replies reply ON reply.id = action.reply_id
		WHERE action.id = $1 AND (action.target_user_id = $2 OR echo.author_user_id = $2 OR reply.author_user_id = $2)
	)`, request.ModerationActionID, userID).Scan(&owns); err != nil || !owns {
		if err != nil {
			return Appeal{}, err
		}
		return Appeal{}, ErrAppealInvalid
	}
	id, err := uuidv7.New()
	if err != nil {
		return Appeal{}, err
	}
	appeal := Appeal{ID: id.String(), ModerationActionID: request.ModerationActionID, Body: request.Body, Status: "open", CreatedAt: now.UTC()}
	_, err = tx.Exec(ctx, `INSERT INTO appeals (id, user_id, moderation_action_id, body_text, body_revisions, created_at) VALUES ($1, $2, $3, $4, jsonb_build_array(jsonb_build_object('body', $4::text, 'created_at', $5::timestamptz)), $5)`, appeal.ID, userID, appeal.ModerationActionID, appeal.Body, appeal.CreatedAt)
	if err != nil {
		return Appeal{}, err
	}
	return appeal, tx.Commit(ctx)
}

func (s *PostgresStore) ListAppeals(ctx context.Context, userID, cursorValue string, limit int) (AppealPage, error) {
	if limit < 1 || limit > 50 {
		limit = 20
	}
	before, beforeID := time.Now().UTC().Add(time.Hour), "ffffffff-ffff-ffff-ffff-ffffffffffff"
	if cursorValue != "" {
		cursor, err := pagination.Decode(cursorValue)
		if err != nil {
			return AppealPage{}, ErrAppealInvalid
		}
		before, beforeID = cursor.After, cursor.ID.String()
	}
	rows, err := s.pool.Query(ctx, `SELECT id::text, moderation_action_id::text, body_text, status, created_at FROM appeals WHERE user_id = $1 AND (created_at, id) < ($2, $3::uuid) ORDER BY created_at DESC, id DESC LIMIT $4`, userID, before, beforeID, limit+1)
	if err != nil {
		return AppealPage{}, err
	}
	defer rows.Close()
	page := AppealPage{Appeals: []Appeal{}}
	for rows.Next() {
		var appeal Appeal
		if err = rows.Scan(&appeal.ID, &appeal.ModerationActionID, &appeal.Body, &appeal.Status, &appeal.CreatedAt); err != nil {
			return AppealPage{}, err
		}
		page.Appeals = append(page.Appeals, appeal)
	}
	if err = rows.Err(); err != nil {
		return AppealPage{}, err
	}
	if len(page.Appeals) > limit {
		last := page.Appeals[limit-1]
		id, parseErr := uuidv7.Parse(last.ID)
		if parseErr != nil {
			return AppealPage{}, parseErr
		}
		page.Appeals = page.Appeals[:limit]
		page.NextCursor, err = pagination.Encode(pagination.Cursor{After: last.CreatedAt, ID: id})
	}
	return page, err
}

func (s *PostgresStore) UpdateAppeal(ctx context.Context, userID, appealID, body, idempotencyKey string, now time.Time) (Appeal, error) {
	body = strings.TrimSpace(body)
	if body == "" || len([]rune(body)) > 2000 || idempotencyKey == "" {
		return Appeal{}, ErrAppealInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Appeal{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	fresh, err := reserveAdminIdempotency(ctx, tx, userID, idempotencyKey, "appeal_update", map[string]string{"id": appealID, "body": body}, now)
	if err != nil {
		return Appeal{}, err
	}
	var appeal Appeal
	if !fresh {
		err = tx.QueryRow(ctx, `SELECT id::text, moderation_action_id::text, body_text, status, created_at FROM appeals WHERE id = $1 AND user_id = $2`, appealID, userID).Scan(&appeal.ID, &appeal.ModerationActionID, &appeal.Body, &appeal.Status, &appeal.CreatedAt)
		return appeal, err
	}
	err = tx.QueryRow(ctx, `UPDATE appeals SET body_revisions = body_revisions || jsonb_build_array(jsonb_build_object('body', $3::text, 'created_at', $4::timestamptz)), body_text = $3 WHERE id = $1 AND user_id = $2 AND status = 'open' RETURNING id::text, moderation_action_id::text, body_text, status, created_at`, appealID, userID, body, now.UTC()).Scan(&appeal.ID, &appeal.ModerationActionID, &appeal.Body, &appeal.Status, &appeal.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Appeal{}, ErrSocialNotFound
	}
	if err != nil {
		return Appeal{}, err
	}
	return appeal, tx.Commit(ctx)
}
