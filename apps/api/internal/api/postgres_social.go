package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/primitives/pagination"
	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/primitives/uuidv7"
	"github.com/jackc/pgx/v5"
)

func (s *PostgresStore) PutReaction(ctx context.Context, echoID string, request ReactionRequest, now time.Time) (ReactionSummary, error) {
	if !validSocialID(echoID) || !validSocialID(request.UserID) || !validReactionKind(request.Kind) {
		return ReactionSummary{}, ErrSocialInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ReactionSummary{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	authorID, err := socialEchoAuthor(ctx, tx, echoID)
	if err != nil {
		return ReactionSummary{}, err
	}
	hidden, blocked, err := socialVisibilityBlocked(ctx, tx, request.UserID, authorID, "echo", echoID)
	if err != nil {
		return ReactionSummary{}, err
	}
	if hidden || blocked {
		return ReactionSummary{}, ErrSocialHidden
	}
	var previousKind string
	_ = tx.QueryRow(ctx, `SELECT reaction_type FROM reactions WHERE echo_id = $1 AND user_id = $2`, echoID, request.UserID).Scan(&previousKind)
	_, err = tx.Exec(ctx, `INSERT INTO reactions (echo_id, user_id, reaction_type, created_at) VALUES ($1, $2, $3, $4)
		ON CONFLICT (echo_id, user_id) DO UPDATE SET reaction_type = EXCLUDED.reaction_type, created_at = EXCLUDED.created_at`,
		echoID, request.UserID, request.Kind, now.UTC())
	if err != nil {
		return ReactionSummary{}, err
	}
	if previousKind != request.Kind {
		if err = insertEngagementEvent(ctx, tx, echoID, request.UserID, "reaction", 1, "reaction:"+echoID+":"+request.UserID, now); err != nil {
			return ReactionSummary{}, err
		}
		if err = insertNotification(ctx, tx, authorID, request.UserID, NotificationReaction, echoID, "Someone reacted to your Echo.", now); err != nil {
			return ReactionSummary{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return ReactionSummary{}, err
	}
	return s.ReactionSummary(ctx, echoID)
}

func (s *PostgresStore) ReactionSummary(ctx context.Context, echoID string) (ReactionSummary, error) {
	known, err := s.EchoKnown(ctx, echoID)
	if err != nil {
		return ReactionSummary{}, err
	}
	if !known {
		return ReactionSummary{}, ErrSocialNotFound
	}
	summary := ReactionSummary{EchoID: echoID, Totals: emptyReactionTotals()}
	rows, err := s.pool.Query(ctx, `SELECT reaction_type, count(*)::integer FROM reactions WHERE echo_id = $1 GROUP BY reaction_type`, echoID)
	if err != nil {
		return ReactionSummary{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var count int
		if err = rows.Scan(&kind, &count); err != nil {
			return ReactionSummary{}, err
		}
		summary.Totals[kind] = count
	}
	return summary, rows.Err()
}

func (s *PostgresStore) EchoKnown(ctx context.Context, echoID string) (bool, error) {
	var known bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM echoes WHERE id = $1 AND status = 'published' AND deleted_at IS NULL)`, echoID).Scan(&known)
	return known, err
}

func (s *PostgresStore) RecordDetailView(ctx context.Context, echoID, userID string, now time.Time) error {
	if !validSocialID(echoID) || !validSocialID(userID) {
		return ErrSocialInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = socialEchoAuthor(ctx, tx, echoID); err != nil {
		return err
	}
	if err = insertEngagementEvent(ctx, tx, echoID, userID, "detail_view", 0.25, "detail_view:"+echoID+":"+userID, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) RegisterEcho(context.Context, string, string) error { return nil }

func (s *PostgresStore) AddReply(ctx context.Context, echoID string, request ReplyRequest, now time.Time) (Reply, error) {
	body := strings.TrimSpace(request.Body)
	if !validSocialID(echoID) || !validSocialID(request.UserID) || body == "" || len([]rune(body)) > 500 {
		return Reply{}, ErrSocialInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Reply{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	authorID, err := socialEchoAuthor(ctx, tx, echoID)
	if err != nil {
		return Reply{}, err
	}
	hidden, blocked, err := socialVisibilityBlocked(ctx, tx, request.UserID, authorID, "echo", echoID)
	if err != nil {
		return Reply{}, err
	}
	if hidden || blocked {
		return Reply{}, ErrSocialHidden
	}
	parentAuthorID := ""
	if request.ReplyTo != "" {
		err = tx.QueryRow(ctx, `SELECT author_user_id::text FROM replies
			WHERE id = $1 AND echo_id = $2 AND parent_reply_id IS NULL AND status = 'published'`, request.ReplyTo, echoID).Scan(&parentAuthorID)
		if errors.Is(err, pgx.ErrNoRows) {
			return Reply{}, ErrSocialInvalid
		}
		if err != nil {
			return Reply{}, err
		}
		_, blocked, blockErr := socialVisibilityBlocked(ctx, tx, request.UserID, parentAuthorID, "reply", request.ReplyTo)
		if blockErr != nil {
			return Reply{}, blockErr
		}
		if blocked {
			return Reply{}, ErrSocialHidden
		}
	}
	replyID, err := uuidv7.New()
	if err != nil {
		return Reply{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO replies (id, echo_id, author_user_id, parent_reply_id, body_text, status, created_at, updated_at)
		VALUES ($1, $2, $3, nullif($4, '')::uuid, $5, 'published', $6, $6)`, replyID.String(), echoID, request.UserID, request.ReplyTo, body, now.UTC())
	if err != nil {
		return Reply{}, err
	}
	revisionID, err := uuidv7.New()
	if err != nil {
		return Reply{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO reply_revisions (id, reply_id, revision_number, body_text, created_at) VALUES ($1, $2, 1, $3, $4)`, revisionID.String(), replyID.String(), body, now.UTC()); err != nil {
		return Reply{}, err
	}
	if err = insertEngagementEvent(ctx, tx, echoID, request.UserID, "reply", 3, "reply:"+replyID.String(), now); err != nil {
		return Reply{}, err
	}
	if err = insertNotification(ctx, tx, authorID, request.UserID, NotificationReply, echoID, "Someone replied to your Echo.", now); err != nil {
		return Reply{}, err
	}
	if parentAuthorID != "" && parentAuthorID != authorID {
		if err = insertNotification(ctx, tx, parentAuthorID, request.UserID, NotificationReply, echoID, "Someone replied to your Reply.", now); err != nil {
			return Reply{}, err
		}
	}
	if err = syncReplyMentions(ctx, tx, echoID, replyID.String(), request.UserID, body, authorID, now); err != nil {
		return Reply{}, err
	}
	var authorHandle string
	if err = tx.QueryRow(ctx, `SELECT handle FROM user_profiles WHERE user_id = $1`, request.UserID).Scan(&authorHandle); err != nil {
		return Reply{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Reply{}, err
	}
	return Reply{ID: replyID.String(), EchoID: echoID, UserID: request.UserID, AuthorHandle: authorHandle, Body: body, ReplyTo: request.ReplyTo, PostedAt: now.UTC()}, nil
}

func (s *PostgresStore) ListReplies(ctx context.Context, echoID, viewerID string) ([]Reply, error) {
	page, err := s.ListReplyPage(ctx, echoID, viewerID, "", 50)
	return page.Replies, err
}

func (s *PostgresStore) ListReplyPage(ctx context.Context, echoID, viewerID, cursorValue string, limit int) (ReplyPage, error) {
	known, err := s.EchoKnown(ctx, echoID)
	if err != nil {
		return ReplyPage{}, err
	}
	if !known {
		return ReplyPage{}, ErrSocialNotFound
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	query := `SELECT r.id::text, r.echo_id::text, r.author_user_id::text, p.handle,
		coalesce(r.parent_reply_id::text, ''), CASE WHEN r.status = 'deleted' THEN 'Reply deleted' ELSE r.body_text END,
		(r.edited_at IS NOT NULL), (r.status = 'deleted'), r.created_at
		FROM replies r
		JOIN user_profiles p ON p.user_id = r.author_user_id
		WHERE r.echo_id = $1 AND r.status IN ('published', 'deleted')
		AND ($2 = '' OR NOT EXISTS (SELECT 1 FROM blocks b WHERE
			(b.blocker_user_id = nullif($2, '')::uuid AND b.blocked_user_id = r.author_user_id) OR
			(b.blocker_user_id = r.author_user_id AND b.blocked_user_id = nullif($2, '')::uuid)))
		AND ($2 = '' OR NOT EXISTS (SELECT 1 FROM hidden_targets h WHERE h.user_id = nullif($2, '')::uuid AND h.target_type = 'reply' AND h.target_id = r.id))
	`
	args := []any{echoID, viewerID, limit + 1}
	if cursorValue != "" {
		cursor, decodeErr := pagination.Decode(cursorValue)
		if decodeErr != nil {
			return ReplyPage{}, decodeErr
		}
		query += ` AND (r.created_at, r.id) > ($4, $5)`
		args = append(args, cursor.After, cursor.ID.String())
	}
	query += ` ORDER BY r.created_at, r.id LIMIT $3`
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return ReplyPage{}, err
	}
	defer rows.Close()
	result := []Reply{}
	for rows.Next() {
		var reply Reply
		if err = rows.Scan(&reply.ID, &reply.EchoID, &reply.UserID, &reply.AuthorHandle, &reply.ReplyTo, &reply.Body, &reply.Edited, &reply.Hidden, &reply.PostedAt); err != nil {
			return ReplyPage{}, err
		}
		result = append(result, reply)
	}
	if err = rows.Err(); err != nil {
		return ReplyPage{}, err
	}
	page := ReplyPage{Replies: result}
	if len(result) > limit {
		page.Replies = result[:limit]
		last := page.Replies[len(page.Replies)-1]
		id, parseErr := uuidv7.Parse(last.ID)
		if parseErr != nil {
			return ReplyPage{}, parseErr
		}
		page.NextCursor, err = pagination.Encode(pagination.Cursor{After: last.PostedAt, ID: id})
		if err != nil {
			return ReplyPage{}, err
		}
	}
	return page, nil
}

func (s *PostgresStore) UpdateReply(ctx context.Context, replyID, userID, body string, now time.Time) (Reply, error) {
	body = strings.TrimSpace(body)
	if !validSocialID(replyID) || !validSocialID(userID) || body == "" || len([]rune(body)) > 500 {
		return Reply{}, ErrSocialInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Reply{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var reply Reply
	var status string
	err = tx.QueryRow(ctx, `SELECT r.id::text, r.echo_id::text, r.author_user_id::text, p.handle, coalesce(r.parent_reply_id::text, ''),
		r.body_text, r.status, r.created_at FROM replies r JOIN user_profiles p ON p.user_id = r.author_user_id WHERE r.id = $1 FOR UPDATE OF r`, replyID).
		Scan(&reply.ID, &reply.EchoID, &reply.UserID, &reply.AuthorHandle, &reply.ReplyTo, &reply.Body, &status, &reply.PostedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Reply{}, ErrSocialNotFound
	}
	if err != nil {
		return Reply{}, err
	}
	if reply.UserID != userID {
		return Reply{}, ErrSocialForbidden
	}
	if status != "published" {
		return Reply{}, ErrSocialInvalid
	}
	if reply.Body == body {
		return reply, nil
	}
	var revisionNumber int
	if err = tx.QueryRow(ctx, `SELECT coalesce(max(revision_number), 0) + 1 FROM reply_revisions WHERE reply_id = $1`, replyID).Scan(&revisionNumber); err != nil {
		return Reply{}, err
	}
	revisionID, err := uuidv7.New()
	if err != nil {
		return Reply{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO reply_revisions (id, reply_id, revision_number, body_text, created_at) VALUES ($1, $2, $3, $4, $5)`, revisionID.String(), replyID, revisionNumber, body, now.UTC()); err != nil {
		return Reply{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE replies SET body_text = $2, edited_at = $3, updated_at = $3 WHERE id = $1`, replyID, body, now.UTC()); err != nil {
		return Reply{}, err
	}
	if err = syncReplyMentions(ctx, tx, reply.EchoID, replyID, userID, body, "", now); err != nil {
		return Reply{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Reply{}, err
	}
	reply.Body = body
	reply.Edited = true
	return reply, nil
}

func (s *PostgresStore) DeleteReply(ctx context.Context, replyID, userID string, now time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	result, err := tx.Exec(ctx, `UPDATE replies SET status = 'deleted', body_text = 'Reply deleted', deleted_at = $3, updated_at = $3
		WHERE id = $1 AND author_user_id = $2 AND status = 'published'`, replyID, userID, now.UTC())
	if err != nil {
		return err
	}
	if result.RowsAffected() > 0 {
		if _, err = tx.Exec(ctx, `DELETE FROM reply_mentions WHERE reply_id = $1`, replyID); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	var authorID string
	err = tx.QueryRow(ctx, `SELECT author_user_id::text FROM replies WHERE id = $1`, replyID).Scan(&authorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrSocialNotFound
	}
	if err != nil {
		return err
	}
	if authorID != userID {
		return ErrSocialForbidden
	}
	return ErrSocialInvalid
}

func (s *PostgresStore) ReplyHistory(ctx context.Context, replyID, userID string) ([]ReplyRevision, error) {
	var authorID string
	err := s.pool.QueryRow(ctx, `SELECT author_user_id::text FROM replies WHERE id = $1`, replyID).Scan(&authorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSocialNotFound
	}
	if err != nil {
		return nil, err
	}
	if authorID != userID {
		return nil, ErrSocialForbidden
	}
	rows, err := s.pool.Query(ctx, `SELECT revision_number, body_text, created_at FROM reply_revisions WHERE reply_id = $1 ORDER BY revision_number`, replyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ReplyRevision{}
	for rows.Next() {
		var revision ReplyRevision
		if err = rows.Scan(&revision.RevisionNumber, &revision.Body, &revision.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, revision)
	}
	return result, rows.Err()
}

func (s *PostgresStore) AddStitch(context.Context, StitchRequest, time.Time) (Stitch, error) {
	return Stitch{}, ErrSocialInvalid
}

func (s *PostgresStore) ValidateStitchSource(ctx context.Context, userID, sourceEchoID, quotedReplyID string, acknowledged bool) error {
	var authorID string
	err := s.pool.QueryRow(ctx, `SELECT author_user_id::text FROM echoes WHERE id = $1 AND status = 'published' AND deleted_at IS NULL`, sourceEchoID).Scan(&authorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrSocialNotFound
	}
	if err != nil {
		return err
	}
	_, blocked, err := socialVisibilityBlocked(ctx, s.pool, userID, authorID, "echo", sourceEchoID)
	if err != nil {
		return err
	}
	if blocked {
		return ErrSocialHidden
	}
	if quotedReplyID == "" {
		return nil
	}
	var eligible bool
	if err = s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM replies r WHERE r.id = $1 AND r.echo_id = $2 AND r.status = 'published'
		AND NOT EXISTS (SELECT 1 FROM blocks b WHERE
			(b.blocker_user_id = $3 AND b.blocked_user_id = r.author_user_id) OR
			(b.blocker_user_id = r.author_user_id AND b.blocked_user_id = $3)))`, quotedReplyID, sourceEchoID, userID).Scan(&eligible); err != nil {
		return err
	}
	if !eligible {
		return ErrSocialHidden
	}
	var alreadyAcknowledged bool
	if err = s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_acknowledgements WHERE user_id = $1 AND acknowledgement_type = 'stitch_reply_snapshot_warning')`, userID).Scan(&alreadyAcknowledged); err != nil {
		return err
	}
	if alreadyAcknowledged {
		return nil
	}
	if !acknowledged {
		return ErrSocialInvalid
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO user_acknowledgements (user_id, acknowledgement_type) VALUES ($1, 'stitch_reply_snapshot_warning') ON CONFLICT DO NOTHING`, userID)
	return err
}

func (s *PostgresStore) LinkStitch(ctx context.Context, sourceEchoID, targetEchoID, userID, quotedReplyID string, now time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var sourceAuthorID, sourceTitle, sourceHandle string
	err = tx.QueryRow(ctx, `SELECT e.author_user_id::text, e.title, p.handle::text FROM echoes e
		JOIN user_profiles p ON p.user_id = e.author_user_id WHERE e.id = $1 AND e.status = 'published' AND e.deleted_at IS NULL`, sourceEchoID).
		Scan(&sourceAuthorID, &sourceTitle, &sourceHandle)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrSocialNotFound
	}
	if err != nil {
		return err
	}
	var targetAuthorID string
	if err = tx.QueryRow(ctx, `SELECT author_user_id::text FROM echoes WHERE id = $1 AND status = 'draft'`, targetEchoID).Scan(&targetAuthorID); errors.Is(err, pgx.ErrNoRows) {
		return ErrSocialNotFound
	} else if err != nil {
		return err
	}
	if targetAuthorID != userID {
		return ErrSocialForbidden
	}
	metadata := map[string]any{"source_title": sourceTitle, "source_author_handle": sourceHandle}
	snapshotStatus := "not_requested"
	var revisionNumber *int
	if quotedReplyID != "" {
		var revision int
		var replyBody, replyAuthorHandle string
		err = tx.QueryRow(ctx, `SELECT rr.revision_number, rr.body_text, p.handle::text FROM replies r
			JOIN user_profiles p ON p.user_id = r.author_user_id
			JOIN LATERAL (SELECT revision_number, body_text FROM reply_revisions WHERE reply_id = r.id ORDER BY revision_number DESC LIMIT 1) rr ON true
			WHERE r.id = $1 AND r.echo_id = $2 AND r.status = 'published'`, quotedReplyID, sourceEchoID).
			Scan(&revision, &replyBody, &replyAuthorHandle)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSocialHidden
		}
		if err != nil {
			return err
		}
		revisionNumber = &revision
		metadata["quoted_reply_body"] = replyBody
		metadata["quoted_reply_author_handle"] = replyAuthorHandle
		metadata["quoted_reply_revision"] = revision
		snapshotStatus = "queued"
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	linkID, err := uuidv7.New()
	if err != nil {
		return err
	}
	insertResult, err := tx.Exec(ctx, `INSERT INTO echo_links
		(id, source_echo_id, target_echo_id, link_type, quoted_reply_id, metadata, source_reply_revision, snapshot_status, created_at)
		VALUES ($1, $2, $3, 'stitch', nullif($4, '')::uuid, $5, $6, $7, $8)
		ON CONFLICT (source_echo_id, target_echo_id, link_type) DO NOTHING`, linkID.String(), sourceEchoID, targetEchoID, quotedReplyID, metadataJSON, revisionNumber, snapshotStatus, now.UTC())
	if err != nil {
		return err
	}
	if insertResult.RowsAffected() == 0 {
		return tx.Commit(ctx)
	}
	if quotedReplyID != "" {
		jobID, idErr := uuidv7.New()
		if idErr != nil {
			return idErr
		}
		_, err = tx.Exec(ctx, `INSERT INTO jobs (id, queue, job_type, payload, max_attempts)
			VALUES ($1, 'media', 'render_reply_snapshot', jsonb_build_object('link_id', $2::text, 'reply_id', $3::text, 'revision_number', $4::integer), 3)`,
			jobID.String(), linkID.String(), quotedReplyID, *revisionNumber)
		if err != nil {
			return err
		}
	}
	if err = insertEngagementEvent(ctx, tx, sourceEchoID, userID, "stitch", 5, "stitch:"+targetEchoID, now); err != nil {
		return err
	}
	if err = insertNotification(ctx, tx, sourceAuthorID, userID, NotificationStitch, sourceEchoID, "Someone stitched your Echo.", now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) AddReport(ctx context.Context, request ReportRequest, now time.Time) (Report, error) {
	category, details, valid := normalizeReport(request)
	if !validSocialID(request.ReporterID) || !validReportTarget(request.TargetType) || !valid {
		return Report{}, ErrSocialInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Report{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if request.TargetType == ReportTargetUser && request.TargetID == "" {
		handle, handleErr := ValidateHandle(request.TargetHandle)
		if handleErr != nil {
			return Report{}, ErrSocialInvalid
		}
		if err = tx.QueryRow(ctx, `SELECT user_id::text FROM user_profiles WHERE handle = $1`, handle).Scan(&request.TargetID); errors.Is(err, pgx.ErrNoRows) {
			return Report{}, ErrSocialNotFound
		} else if err != nil {
			return Report{}, err
		}
	}
	if !validSocialID(request.TargetID) || request.ReporterID == request.TargetID {
		return Report{}, ErrSocialInvalid
	}
	reportID, err := uuidv7.New()
	if err != nil {
		return Report{}, err
	}
	column := map[string]string{ReportTargetEcho: "echo_id", ReportTargetReply: "reply_id", ReportTargetUser: "reported_user_id"}[request.TargetType]
	query := `INSERT INTO reports (id, reporter_user_id, ` + column + `, category, details, status, created_at)
		VALUES ($1, $2, $3, $4, nullif($5, ''), 'open', $6)`
	if _, err = tx.Exec(ctx, query, reportID.String(), request.ReporterID, request.TargetID, category, details, now.UTC()); err != nil {
		if isForeignKeyViolation(err) {
			return Report{}, ErrSocialNotFound
		}
		return Report{}, err
	}
	if request.TargetType != ReportTargetUser {
		if _, err = tx.Exec(ctx, `INSERT INTO hidden_targets (user_id, target_type, target_id) VALUES ($1, $2, $3)
			ON CONFLICT DO NOTHING`, request.ReporterID, request.TargetType, request.TargetID); err != nil {
			return Report{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return Report{}, err
	}
	return Report{ID: reportID.String(), ReporterID: request.ReporterID, TargetType: request.TargetType, TargetID: request.TargetID, Category: category, Details: details, Reason: details, Status: "pending_review", CreatedAt: now.UTC()}, nil
}

func (s *PostgresStore) Hide(ctx context.Context, request VisibilityRequest) error {
	if !validSocialID(request.UserID) || !validSocialID(request.TargetID) {
		return ErrSocialInvalid
	}
	targetType := "echo"
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM echoes WHERE id = $1)`, request.TargetID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		targetType = "reply"
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM replies WHERE id = $1)`, request.TargetID).Scan(&exists); err != nil {
			return err
		}
	}
	if !exists {
		return ErrSocialNotFound
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO hidden_targets (user_id, target_type, target_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, request.UserID, targetType, request.TargetID)
	return err
}

func (s *PostgresStore) Block(ctx context.Context, request VisibilityRequest) error {
	if !validSocialID(request.UserID) || !validSocialID(request.TargetID) || request.UserID == request.TargetID {
		return ErrSocialInvalid
	}
	result, err := s.pool.Exec(ctx, `INSERT INTO blocks (blocker_user_id, blocked_user_id) SELECT $1, id FROM users WHERE id = $2 ON CONFLICT DO NOTHING`, request.UserID, request.TargetID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrSocialNotFound
	}
	return nil
}

func (s *PostgresStore) Notifications(ctx context.Context, userID string) ([]Notification, error) {
	page, err := s.NotificationPage(ctx, userID, "", 100)
	return page.Notifications, err
}

func (s *PostgresStore) NotificationPage(ctx context.Context, userID, cursorValue string, limit int) (NotificationPage, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	query := `SELECT n.id::text, n.user_id::text, n.type, coalesce(n.actor_user_id::text, ''),
		CASE WHEN n.type IN ('moderation', 'moderation_update', 'moderation_action')
			THEN coalesce(n.payload->>'moderation_action_id', '')
			ELSE coalesce(n.echo_id::text, coalesce(n.reply_id::text, coalesce(n.place_id::text, ''))) END,
		coalesce(n.payload->>'preview', ''), n.created_at, n.read_at
		FROM notifications n WHERE n.user_id = $1 AND (n.expires_at IS NULL OR n.expires_at > now())
		AND NOT EXISTS (SELECT 1 FROM blocks b WHERE b.blocker_user_id = n.user_id AND b.blocked_user_id = n.actor_user_id)`
	args := []any{userID}
	if cursorValue != "" {
		cursor, err := pagination.Decode(cursorValue)
		if err != nil {
			return NotificationPage{}, ErrSocialInvalid
		}
		query += ` AND (n.created_at, n.id) < ($2, $3)`
		args = append(args, cursor.After, cursor.ID.String())
	}
	query += ` ORDER BY n.created_at DESC, n.id DESC LIMIT ` + fmt.Sprintf("%d", limit+1)
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return NotificationPage{}, err
	}
	defer rows.Close()
	result := make([]Notification, 0, limit+1)
	for rows.Next() {
		var notification Notification
		if err = rows.Scan(&notification.ID, &notification.UserID, &notification.Kind, &notification.ActorID, &notification.TargetID, &notification.Preview, &notification.CreatedAt, &notification.ReadAt); err != nil {
			return NotificationPage{}, err
		}
		result = append(result, notification)
	}
	if err = rows.Err(); err != nil {
		return NotificationPage{}, err
	}
	page := NotificationPage{Notifications: result}
	if len(result) > limit {
		page.Notifications = result[:limit]
		last := page.Notifications[len(page.Notifications)-1]
		id, parseErr := uuidv7.Parse(last.ID)
		if parseErr != nil {
			return NotificationPage{}, parseErr
		}
		page.NextCursor, err = pagination.Encode(pagination.Cursor{After: last.CreatedAt, ID: id})
		if err != nil {
			return NotificationPage{}, err
		}
	}
	return page, nil
}

type socialQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func socialEchoAuthor(ctx context.Context, queryer socialQueryer, echoID string) (string, error) {
	var authorID string
	err := queryer.QueryRow(ctx, `SELECT author_user_id::text FROM echoes WHERE id = $1 AND status = 'published' AND deleted_at IS NULL`, echoID).Scan(&authorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrSocialNotFound
	}
	return authorID, err
}

func socialVisibilityBlocked(ctx context.Context, queryer socialQueryer, actorID, authorID, targetType, targetID string) (bool, bool, error) {
	var hidden, blocked bool
	err := queryer.QueryRow(ctx, `SELECT
		EXISTS (SELECT 1 FROM hidden_targets WHERE user_id = $1 AND target_type = $3 AND target_id = $4),
		EXISTS (SELECT 1 FROM blocks WHERE (blocker_user_id = $1 AND blocked_user_id = $2) OR (blocker_user_id = $2 AND blocked_user_id = $1))`,
		actorID, authorID, targetType, targetID).Scan(&hidden, &blocked)
	return hidden, blocked, err
}

// syncReplyMentions derives mentions from server-owned profile handles and
// limits resolution to the Echo author and users who have already participated
// in the thread. Unknown and out-of-thread @handles remain ordinary text.
func syncReplyMentions(ctx context.Context, tx pgx.Tx, echoID, replyID, actorID, body, alreadyNotifiedUserID string, now time.Time) error {
	handles := parseReplyMentionHandles(body)
	previous := map[string]bool{}
	rows, err := tx.Query(ctx, `SELECT mentioned_user_id::text FROM reply_mentions WHERE reply_id = $1`, replyID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var userID string
		if err = rows.Scan(&userID); err != nil {
			rows.Close()
			return err
		}
		previous[userID] = true
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM reply_mentions WHERE reply_id = $1`, replyID); err != nil {
		return err
	}
	if len(handles) == 0 {
		return nil
	}
	rows, err = tx.Query(ctx, `SELECT DISTINCT profile.user_id::text
		FROM user_profiles profile
		JOIN users mentioned_user ON mentioned_user.id = profile.user_id
		WHERE profile.handle::text = ANY($2::text[])
		AND mentioned_user.status IN ('active', 'restricted')
		AND profile.user_id <> $3
		AND (
			profile.user_id = (SELECT author_user_id FROM echoes WHERE id = $1)
			OR EXISTS (SELECT 1 FROM replies participant
				WHERE participant.echo_id = $1 AND participant.author_user_id = profile.user_id
				AND participant.status IN ('published', 'deleted'))
		)
		AND NOT EXISTS (SELECT 1 FROM blocks block WHERE
			(block.blocker_user_id = profile.user_id AND block.blocked_user_id = $3)
			OR (block.blocker_user_id = $3 AND block.blocked_user_id = profile.user_id))
		ORDER BY profile.user_id::text`, echoID, handles, actorID)
	if err != nil {
		return err
	}
	mentionedUserIDs := []string{}
	for rows.Next() {
		var userID string
		if err = rows.Scan(&userID); err != nil {
			rows.Close()
			return err
		}
		mentionedUserIDs = append(mentionedUserIDs, userID)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, mentionedUserID := range mentionedUserIDs {
		if _, err = tx.Exec(ctx, `INSERT INTO reply_mentions (reply_id, mentioned_user_id, created_at)
			VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, replyID, mentionedUserID, now.UTC()); err != nil {
			return err
		}
		if !previous[mentionedUserID] && mentionedUserID != alreadyNotifiedUserID {
			if err = insertNotification(ctx, tx, mentionedUserID, actorID, NotificationMention, echoID, "Someone mentioned you in a Reply.", now); err != nil {
				return err
			}
		}
	}
	return nil
}

func insertNotification(ctx context.Context, tx pgx.Tx, userID, actorID, kind, echoID, preview string, now time.Time) error {
	if userID == actorID {
		return nil
	}
	var blocked bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM blocks WHERE blocker_user_id = $1 AND blocked_user_id = $2)`, userID, actorID).Scan(&blocked); err != nil || blocked {
		return err
	}
	id, err := uuidv7.New()
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO notifications (id, user_id, type, actor_user_id, echo_id, payload, created_at, expires_at, collapse_key)
		VALUES ($1, $2, $3, $4, $5, jsonb_build_object('preview', $6::text), $7, $8, $9)`, id.String(), userID, kind, actorID, echoID, preview, now.UTC(), now.Add(90*24*time.Hour).UTC(), kind+":"+echoID)
	if err != nil {
		return err
	}
	deliveries, err := tx.Exec(ctx, `INSERT INTO notification_deliveries (notification_id, device_id)
		SELECT $1, d.id FROM devices d
		LEFT JOIN notification_preferences p ON p.user_id = d.user_id
		WHERE d.user_id = $2 AND d.disabled_at IS NULL AND d.push_token_ciphertext IS NOT NULL
		AND coalesce(p.direct_activity_enabled, true)
		AND NOT EXISTS (SELECT 1 FROM notification_mutes m WHERE m.user_id = d.user_id AND m.echo_id = $3
			AND (m.muted_until IS NULL OR m.muted_until > $4))
		ON CONFLICT DO NOTHING`, id.String(), userID, echoID, now.UTC())
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
		VALUES ($1, 'notifications', 'send_push', jsonb_build_object('notification_id', $2::text), 5)`, jobID.String(), id.String())
	return err
}

func insertEngagementEvent(ctx context.Context, tx pgx.Tx, echoID, actorID, eventType string, weight float64, dedupeKey string, now time.Time) error {
	id, err := uuidv7.New()
	if err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `INSERT INTO echo_engagement_events (id, echo_id, actor_user_id, event_type, event_weight, occurred_at, dedupe_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (dedupe_key) WHERE dedupe_key IS NOT NULL DO NOTHING`,
		id.String(), echoID, actorID, eventType, weight, now.UTC(), dedupeKey)
	if err != nil || result.RowsAffected() == 0 {
		return err
	}
	jobID, err := uuidv7.New()
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO jobs (id, queue, job_type, payload, max_attempts)
		VALUES ($1, 'ranking', 'rank_echo', jsonb_build_object('echo_id', $2::text), 3)`, jobID.String(), echoID)
	return err
}

func isForeignKeyViolation(err error) bool {
	var postgresError interface{ SQLState() string }
	return errors.As(err, &postgresError) && postgresError.SQLState() == "23503"
}
