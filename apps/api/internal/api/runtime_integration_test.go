package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestIntegrationSMTPFixture(t *testing.T) {
	if os.Getenv("ECHOTRAILS_INTEGRATION") != "1" {
		t.Skip("set ECHOTRAILS_INTEGRATION=1")
	}
	sender, err := NewSMTPSender(SMTPConfig{Host: "127.0.0.1", Port: "51025", From: "integration@invalid.example", AuthLinkBaseURL: "http://127.0.0.1:8080", AllowInsecure: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = sender.SendAuthCode(context.Background(), "recipient@invalid.example", "request-1", "123456", time.Now().Add(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
}

func TestIntegrationAdminPlaceBoundaryAndRadiusAreAudited(t *testing.T) {
	if os.Getenv("ECHOTRAILS_INTEGRATION") != "1" {
		t.Skip("set ECHOTRAILS_INTEGRATION=1")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("ECHOTRAILS_INTEGRATION_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	const (
		adminID = "019f4a30-0001-7000-8000-000000000001"
		placeID = "019f4a30-0002-7000-8000-000000000002"
	)
	cleanup := func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM audit_logs WHERE actor_user_id = $1 OR target_id = $2`, adminID, placeID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM idempotency_keys WHERE user_id = $1`, adminID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM places WHERE id = $1`, placeID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, adminID)
	}
	cleanup()
	defer cleanup()
	if _, err = pool.Exec(ctx, `INSERT INTO users (id, email, status, role) VALUES ($1, 'boundary-admin@invalid.example', 'active', 'admin')`, adminID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO places (id, name, normalized_name, status, location, default_radius_meters)
		VALUES ($1, 'Boundary Integration Place', 'boundary integration place', 'pending_review',
		ST_SetSRID(ST_MakePoint(-88.01, 41.80), 4326)::geography, 100)`, placeID); err != nil {
		t.Fatal(err)
	}
	store, err := NewPostgresStore(pool, []byte("integration-exact-location-key-32-bytes"), []byte("integration-push-token-key-at-least-32"))
	if err != nil {
		t.Fatal(err)
	}
	actor := AdminSessionRecord{UserID: adminID, Role: "admin", ExpiresAt: time.Now().Add(time.Hour)}
	radius := 25.0
	boundary := json.RawMessage(`{"type":"Polygon","coordinates":[[[-88.011,41.799],[-88.009,41.799],[-88.009,41.801],[-88.011,41.801],[-88.011,41.799]]]}`)
	if err = store.ApplyAdminMutation(ctx, actor, "place", AdminMutationRequest{
		Action: "set_boundary", ReasonCode: "reviewed_osm_geometry", TargetID: placeID,
		BoundaryGeoJSON: boundary, RadiusMeters: &radius, Confirmed: true,
	}, "boundary-integration-1", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	var covers bool
	var storedRadius float64
	if err = pool.QueryRow(ctx, `SELECT ST_Covers(boundary, ST_SetSRID(ST_MakePoint(-88.01, 41.80), 4326)::geography), default_radius_meters FROM places WHERE id = $1`, placeID).Scan(&covers, &storedRadius); err != nil {
		t.Fatal(err)
	}
	if !covers || storedRadius != radius {
		t.Fatalf("boundary covers=%v radius=%v", covers, storedRadius)
	}
	pointRadius := 100.0
	if err = store.ApplyAdminMutation(ctx, actor, "place", AdminMutationRequest{
		Action: "set_radius", ReasonCode: "reviewed_point_fallback", TargetID: placeID,
		RadiusMeters: &pointRadius, Confirmed: true,
	}, "boundary-integration-2", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	var boundaryCleared bool
	if err = pool.QueryRow(ctx, `SELECT boundary IS NULL AND default_radius_meters = 100 FROM places WHERE id = $1`, placeID).Scan(&boundaryCleared); err != nil || !boundaryCleared {
		t.Fatalf("point fallback not applied: cleared=%v err=%v", boundaryCleared, err)
	}
	var auditCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE actor_user_id = $1 AND target_id = $2 AND action IN ('set_boundary', 'set_radius')`, adminID, placeID).Scan(&auditCount); err != nil || auditCount != 2 {
		t.Fatalf("audit count=%d err=%v", auditCount, err)
	}
}

func TestIntegrationAdminTextReviewPublishesOrCreatesAppealableLimit(t *testing.T) {
	if os.Getenv("ECHOTRAILS_INTEGRATION") != "1" {
		t.Skip("set ECHOTRAILS_INTEGRATION=1")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("ECHOTRAILS_INTEGRATION_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	const (
		adminID  = "019f4a32-0001-7000-8000-000000000001"
		authorID = "019f4a32-0002-7000-8000-000000000002"
		echoID   = "019f4a32-0003-7000-8000-000000000003"
		reviewID = "019f4a32-0004-7000-8000-000000000004"
	)
	cleanup := func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM jobs WHERE payload->>'echo_id' = $1 OR payload->>'notification_id' IN (SELECT id::text FROM notifications WHERE echo_id = $1)`, echoID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM notifications WHERE echo_id = $1`, echoID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM audit_logs WHERE actor_user_id = $1`, adminID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM idempotency_keys WHERE user_id IN ($1, $2)`, adminID, authorID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM appeals WHERE user_id = $1`, authorID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM moderation_actions WHERE moderator_user_id = $1`, adminID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM echoes WHERE id = $1`, echoID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id IN ($1, $2)`, adminID, authorID)
	}
	cleanup()
	defer cleanup()
	if _, err = pool.Exec(ctx, `INSERT INTO users (id, email, status, role) VALUES
		($1, 'review-admin@invalid.example', 'active', 'moderator'),
		($2, 'review-author@invalid.example', 'active', 'user')`, adminID, authorID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO echoes
		(id, author_user_id, place_id, title, content_mode, body_text, status, public_location)
		VALUES ($1, $2, $3, 'Manual text review', 'text', 'A safe story awaiting a human decision.', 'pending_review',
		ST_SetSRID(ST_MakePoint(-88.0153, 41.7976), 4326)::geography)`, echoID, authorID, PlacePrincePondID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO echo_moderation_reviews
		(id, echo_id, status, risk_level, scanner_name, failure_reason)
		VALUES ($1, $2, 'needs_review', 'unknown', 'integration-scanner', 'uncertain')`, reviewID, echoID); err != nil {
		t.Fatal(err)
	}
	store, err := NewPostgresStore(pool, []byte("integration-exact-location-key-32-bytes"), []byte("integration-push-token-key-at-least-32"))
	if err != nil {
		t.Fatal(err)
	}
	actor := AdminSessionRecord{UserID: adminID, Role: "moderator"}
	if err = store.ApplyAdminMutation(ctx, actor, "review", AdminMutationRequest{Action: "approve", ReasonCode: "human_safe", TargetID: reviewID, Confirmed: true}, "review-approve", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	var echoStatus, reviewStatus string
	if err = pool.QueryRow(ctx, `SELECT echo.status, review.status FROM echoes echo JOIN echo_moderation_reviews review ON review.echo_id = echo.id WHERE echo.id = $1`, echoID).Scan(&echoStatus, &reviewStatus); err != nil {
		t.Fatal(err)
	}
	if echoStatus != "published" || reviewStatus != "approved" {
		t.Fatalf("approved review echo=%s review=%s", echoStatus, reviewStatus)
	}
	if _, err = pool.Exec(ctx, `UPDATE echoes SET status = 'pending_review', published_at = NULL WHERE id = $1`, echoID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE echo_moderation_reviews SET status = 'needs_review', risk_level = 'unknown' WHERE id = $1`, reviewID); err != nil {
		t.Fatal(err)
	}
	if err = store.ApplyAdminMutation(ctx, actor, "review", AdminMutationRequest{Action: "reject", ReasonCode: "human_unsafe", TargetID: reviewID, Confirmed: true}, "review-reject", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	var actionID, notificationTarget string
	if err = pool.QueryRow(ctx, `SELECT action.id::text, notification.payload->>'moderation_action_id'
		FROM moderation_actions action JOIN notifications notification ON notification.echo_id = action.echo_id
		WHERE action.moderator_user_id = $1 AND action.echo_id = $2 ORDER BY action.created_at DESC LIMIT 1`, adminID, echoID).Scan(&actionID, &notificationTarget); err != nil {
		t.Fatal(err)
	}
	if actionID != notificationTarget {
		t.Fatalf("moderation activity target=%s action=%s", notificationTarget, actionID)
	}
	if _, err = store.CreateAppeal(ctx, authorID, AppealRequest{ModerationActionID: actionID, Body: "Please reconsider this decision."}, "review-appeal", time.Now().UTC()); err != nil {
		t.Fatalf("echo author could not appeal review: %v", err)
	}
}

func TestIntegrationAdminMediaApprovalRequiresSafeDerivativeAndPublishes(t *testing.T) {
	if os.Getenv("ECHOTRAILS_INTEGRATION") != "1" {
		t.Skip("set ECHOTRAILS_INTEGRATION=1")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("ECHOTRAILS_INTEGRATION_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	const (
		adminID       = "019f4a33-0001-7000-8000-000000000001"
		authorID      = "019f4a33-0002-7000-8000-000000000002"
		echoID        = "019f4a33-0003-7000-8000-000000000003"
		textReviewID  = "019f4a33-0004-7000-8000-000000000004"
		assetID       = "019f4a33-0005-7000-8000-000000000005"
		derivativeID  = "019f4a33-0006-7000-8000-000000000006"
		mediaReviewID = "019f4a33-0007-7000-8000-000000000007"
	)
	cleanup := func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM jobs WHERE payload->>'echo_id' = $1`, echoID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM audit_logs WHERE actor_user_id = $1`, adminID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM idempotency_keys WHERE user_id = $1`, adminID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM echoes WHERE id = $1`, echoID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id IN ($1, $2)`, adminID, authorID)
	}
	cleanup()
	defer cleanup()
	if _, err = pool.Exec(ctx, `INSERT INTO users (id, email, status, role) VALUES
		($1, 'media-admin@invalid.example', 'active', 'moderator'),
		($2, 'media-author@invalid.example', 'active', 'user')`, adminID, authorID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO echoes
		(id, author_user_id, place_id, title, content_mode, caption_text, status, public_location)
		VALUES ($1, $2, $3, 'Media review fixture', 'photo_set', 'A safe processed photo.', 'pending_review',
		ST_SetSRID(ST_MakePoint(-88.0153, 41.7976), 4326)::geography)`, echoID, authorID, PlacePrincePondID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO echo_moderation_reviews
		(id, echo_id, status, risk_level, scanner_name) VALUES ($1, $2, 'approved', 'low', 'ShieldGemma:pinned')`, textReviewID, echoID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO media_assets
		(id, owner_user_id, echo_id, media_type, storage_bucket, object_key, content_type, byte_size, status, malware_scan_status)
		VALUES ($1, $2, $3, 'image', 'integration', 'media/original/admin-review.jpg', 'image/jpeg', 1024, 'pending_review', 'clean')`, assetID, authorID, echoID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO media_derivatives
		(id, media_asset_id, derivative_type, storage_bucket, object_key, content_type, byte_size)
		VALUES ($1, $2, 'preview', 'integration', 'media/derivatives/admin-review.jpg', 'image/jpeg', 512)`, derivativeID, assetID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO media_moderation_reviews
		(id, media_asset_id, status, risk_level, scanner_name, failure_reason)
		VALUES ($1, $2, 'needs_review', 'unknown', 'ShieldGemma2:pinned', 'uncertain')`, mediaReviewID, assetID); err != nil {
		t.Fatal(err)
	}
	store, err := NewPostgresStore(pool, []byte("integration-exact-location-key-32-bytes"), []byte("integration-push-token-key-at-least-32"))
	if err != nil {
		t.Fatal(err)
	}
	if err = store.ApplyAdminMutation(ctx, AdminSessionRecord{UserID: adminID, Role: "moderator"}, "review", AdminMutationRequest{
		Action: "approve", ReasonCode: "human_media_safe", TargetID: mediaReviewID, Confirmed: true,
	}, "media-review-approve", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	var echoStatus, assetStatus, reviewStatus string
	if err = pool.QueryRow(ctx, `SELECT echo.status, asset.status, review.status FROM echoes echo
		JOIN media_assets asset ON asset.echo_id = echo.id JOIN media_moderation_reviews review ON review.media_asset_id = asset.id
		WHERE echo.id = $1`, echoID).Scan(&echoStatus, &assetStatus, &reviewStatus); err != nil {
		t.Fatal(err)
	}
	if echoStatus != "published" || assetStatus != "approved" || reviewStatus != "approved" {
		t.Fatalf("media approval echo=%s asset=%s review=%s", echoStatus, assetStatus, reviewStatus)
	}
}

func TestIntegrationModeratorRemovalSuppressesReplySnapshots(t *testing.T) {
	if os.Getenv("ECHOTRAILS_INTEGRATION") != "1" {
		t.Skip("set ECHOTRAILS_INTEGRATION=1")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("ECHOTRAILS_INTEGRATION_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	const (
		adminID  = "019f4a31-0001-7000-8000-000000000001"
		authorID = "019f4a31-0002-7000-8000-000000000002"
		sourceID = "019f4a31-0003-7000-8000-000000000003"
		targetID = "019f4a31-0004-7000-8000-000000000004"
		replyID  = "019f4a31-0005-7000-8000-000000000005"
		linkID   = "019f4a31-0006-7000-8000-000000000006"
	)
	cleanup := func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM audit_logs WHERE actor_user_id = $1`, adminID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM idempotency_keys WHERE user_id = $1`, adminID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM moderation_actions WHERE moderator_user_id = $1`, adminID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM echo_links WHERE id = $1`, linkID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM replies WHERE id = $1`, replyID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM echoes WHERE id IN ($1, $2)`, sourceID, targetID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id IN ($1, $2)`, adminID, authorID)
	}
	cleanup()
	defer cleanup()
	if _, err = pool.Exec(ctx, `INSERT INTO users (id, email, status, role) VALUES
		($1, 'snapshot-admin@invalid.example', 'active', 'moderator'),
		($2, 'snapshot-author@invalid.example', 'active', 'user')`, adminID, authorID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO echoes
		(id, author_user_id, place_id, title, content_mode, body_text, status, public_location, published_at)
		VALUES ($1, $3, $4, 'Snapshot source', 'text', 'Source', 'published', ST_SetSRID(ST_MakePoint(-88.01, 41.80), 4326)::geography, now()),
		($2, $3, $4, 'Snapshot target', 'text', 'Target', 'published', ST_SetSRID(ST_MakePoint(-88.01, 41.80), 4326)::geography, now())`, sourceID, targetID, authorID, PlacePrincePondID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO replies (id, echo_id, author_user_id, body_text) VALUES ($1, $2, $3, 'Quoted revision')`, replyID, sourceID, authorID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO echo_links (id, source_echo_id, target_echo_id, link_type, quoted_reply_id, snapshot_status)
		VALUES ($1, $2, $3, 'stitch', $4, 'ready')`, linkID, sourceID, targetID, replyID); err != nil {
		t.Fatal(err)
	}
	store, err := NewPostgresStore(pool, []byte("integration-exact-location-key-32-bytes"), []byte("integration-push-token-key-at-least-32"))
	if err != nil {
		t.Fatal(err)
	}
	request := AdminMutationRequest{Action: "remove", ReasonCode: "safety_removal", TargetType: "reply", TargetID: replyID, Confirmed: true}
	if err = store.ApplyAdminMutation(ctx, AdminSessionRecord{UserID: adminID, Role: "moderator"}, "moderation", request, "snapshot-suppression-1", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	var replyStatus, snapshotStatus, reason string
	if err = pool.QueryRow(ctx, `SELECT reply.status, link.snapshot_status, link.snapshot_suppression_reason
		FROM replies reply JOIN echo_links link ON link.quoted_reply_id = reply.id WHERE reply.id = $1`, replyID).Scan(&replyStatus, &snapshotStatus, &reason); err != nil {
		t.Fatal(err)
	}
	if replyStatus != "removed" || snapshotStatus != "suppressed" || reason != "moderator_safety_removal" {
		t.Fatalf("reply=%s snapshot=%s reason=%s", replyStatus, snapshotStatus, reason)
	}
	var actionID string
	if err = pool.QueryRow(ctx, `SELECT id::text FROM moderation_actions WHERE moderator_user_id = $1 AND reply_id = $2 ORDER BY created_at DESC LIMIT 1`, adminID, replyID).Scan(&actionID); err != nil {
		t.Fatal(err)
	}
	appeal, err := store.CreateAppeal(ctx, authorID, AppealRequest{ModerationActionID: actionID, Body: "Please review this removal."}, "snapshot-appeal-create", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err = store.ApplyAdminMutation(ctx, AdminSessionRecord{UserID: adminID, Role: "moderator"}, "appeal", AdminMutationRequest{
		Action: "accept", ReasonCode: "removal_reversed", TargetID: appeal.ID, Confirmed: true,
	}, "snapshot-suppression-appeal", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	var appealStatus string
	if err = pool.QueryRow(ctx, `SELECT reply.status, link.snapshot_status, appeal.status
		FROM replies reply JOIN echo_links link ON link.quoted_reply_id = reply.id
		JOIN appeals appeal ON appeal.id = $2 WHERE reply.id = $1`, replyID, appeal.ID).Scan(&replyStatus, &snapshotStatus, &appealStatus); err != nil {
		t.Fatal(err)
	}
	if replyStatus != "published" || snapshotStatus != "queued" || appealStatus != "accepted" {
		t.Fatalf("appeal reversal reply=%s snapshot=%s appeal=%s", replyStatus, snapshotStatus, appealStatus)
	}
}

func TestIntegrationReplyMentionsStayInsideThread(t *testing.T) {
	if os.Getenv("ECHOTRAILS_INTEGRATION") != "1" {
		t.Skip("set ECHOTRAILS_INTEGRATION=1")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("ECHOTRAILS_INTEGRATION_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	const (
		echoID        = "019f4a20-0001-7000-8000-000000000001"
		authorID      = "019f4a20-0002-7000-8000-000000000002"
		participantID = "019f4a20-0003-7000-8000-000000000003"
		actorID       = "019f4a20-0004-7000-8000-000000000004"
		outsiderID    = "019f4a20-0005-7000-8000-000000000005"
	)
	userIDs := []string{authorID, participantID, actorID, outsiderID}
	cleanup := func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM jobs WHERE payload->>'echo_id' = $1`, echoID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM echoes WHERE id = $1`, echoID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id IN ($1, $2, $3, $4)`, userIDs[0], userIDs[1], userIDs[2], userIDs[3])
	}
	cleanup()
	defer cleanup()
	for index, userID := range userIDs {
		if _, err = pool.Exec(ctx, `INSERT INTO users (id, email, status) VALUES ($1, $2, 'active')`, userID, fmt.Sprintf("mention-%d@invalid.example", index)); err != nil {
			t.Fatal(err)
		}
	}
	profiles := map[string]string{authorID: "thread_author", participantID: "trail_friend", actorID: "reply_actor", outsiderID: "outside_user"}
	for userID, handle := range profiles {
		if _, err = pool.Exec(ctx, `INSERT INTO user_profiles (user_id, handle, display_name) VALUES ($1, $2, $3)`, userID, handle, handle); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = pool.Exec(ctx, `INSERT INTO echoes
		(id, author_user_id, place_id, title, content_mode, body_text, status, public_location, created_at, published_at, updated_at)
		VALUES ($1, $2, $3, 'Mention integration thread', 'text', 'A published integration fixture.', 'published',
		ST_SetSRID(ST_MakePoint(-88.0153, 41.7976), 4326)::geography, now(), now(), now())`, echoID, authorID, PlacePrincePondID); err != nil {
		t.Fatal(err)
	}
	store, err := NewPostgresStore(pool, []byte("integration-exact-location-key-32-bytes"), []byte("integration-push-token-key-at-least-32"))
	if err != nil {
		t.Fatal(err)
	}
	parentReply, err := store.AddReply(ctx, echoID, ReplyRequest{UserID: participantID, Body: "I joined this thread."}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if parentReply.AuthorHandle != "trail_friend" {
		t.Fatalf("parent Reply author handle = %q, want trail_friend", parentReply.AuthorHandle)
	}
	reply, err := store.AddReply(ctx, echoID, ReplyRequest{UserID: actorID, ReplyTo: parentReply.ID, Body: "Hi @trail_friend and @TRAIL_FRIEND, @outside_user, and @thread_author."}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if reply.AuthorHandle != "reply_actor" {
		t.Fatalf("Reply author handle = %q, want reply_actor", reply.AuthorHandle)
	}
	page, err := store.ListReplyPage(ctx, echoID, authorID, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Replies) != 2 || page.Replies[0].AuthorHandle != "trail_friend" || page.Replies[1].AuthorHandle != "reply_actor" {
		t.Fatalf("listed Reply handles = %#v", page.Replies)
	}
	var mentionedHandles []string
	rows, err := pool.Query(ctx, `SELECT profile.handle::text FROM reply_mentions mention
		JOIN user_profiles profile ON profile.user_id = mention.mentioned_user_id
		WHERE mention.reply_id = $1 ORDER BY profile.handle`, reply.ID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var handle string
		if err = rows.Scan(&handle); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		mentionedHandles = append(mentionedHandles, handle)
	}
	rows.Close()
	if strings.Join(mentionedHandles, ",") != "thread_author,trail_friend" {
		t.Fatalf("persisted mention handles = %#v", mentionedHandles)
	}
	var participantMentions, authorMentions int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE user_id = $1 AND actor_user_id = $2 AND type = 'mention'`, participantID, actorID).Scan(&participantMentions); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE user_id = $1 AND actor_user_id = $2 AND type = 'mention'`, authorID, actorID).Scan(&authorMentions); err != nil {
		t.Fatal(err)
	}
	if participantMentions != 1 || authorMentions != 0 {
		t.Fatalf("mention notifications participant=%d author-duplicates=%d", participantMentions, authorMentions)
	}
	var parentReplyNotifications int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE user_id = $1 AND actor_user_id = $2 AND type = 'reply'`, participantID, actorID).Scan(&parentReplyNotifications); err != nil {
		t.Fatal(err)
	}
	if parentReplyNotifications != 1 {
		t.Fatalf("nested Reply notifications=%d, want 1", parentReplyNotifications)
	}
	updatedReply, err := store.UpdateReply(ctx, reply.ID, actorID, "Mention removed.", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if updatedReply.AuthorHandle != "reply_actor" {
		t.Fatalf("updated Reply author handle = %q, want reply_actor", updatedReply.AuthorHandle)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO blocks (blocker_user_id, blocked_user_id) VALUES ($1, $2)`, participantID, actorID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.UpdateReply(ctx, reply.ID, actorID, "Blocked @trail_friend mention.", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	var blockedMentionCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM reply_mentions WHERE reply_id = $1`, reply.ID).Scan(&blockedMentionCount); err != nil {
		t.Fatal(err)
	}
	if blockedMentionCount != 0 {
		t.Fatalf("blocked mention rows = %d", blockedMentionCount)
	}
	if _, err = store.AddReply(ctx, echoID, ReplyRequest{UserID: actorID, ReplyTo: parentReply.ID, Body: "Blocked nested reply."}, time.Now().UTC()); !errors.Is(err, ErrSocialHidden) {
		t.Fatalf("blocked nested Reply error=%v, want ErrSocialHidden", err)
	}
}

func TestIntegrationPublicProfileEchoesArePublishedPaginatedAndHydrated(t *testing.T) {
	if os.Getenv("ECHOTRAILS_INTEGRATION") != "1" {
		t.Skip("set ECHOTRAILS_INTEGRATION=1")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("ECHOTRAILS_INTEGRATION_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	const (
		userID    = "019f4a24-0001-7000-8000-000000000001"
		viewerID  = "019f4a24-0007-7000-8000-000000000007"
		placeID   = "019f4a24-0002-7000-8000-000000000002"
		sourceID  = "019f4a24-0003-7000-8000-000000000003"
		stitchID  = "019f4a24-0004-7000-8000-000000000004"
		limitedID = "019f4a24-0005-7000-8000-000000000005"
		linkID    = "019f4a24-0006-7000-8000-000000000006"
	)
	cleanup := func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM echoes WHERE id IN ($1, $2, $3)`, sourceID, stitchID, limitedID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM places WHERE id = $1`, placeID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id IN ($1, $2)`, userID, viewerID)
	}
	cleanup()
	defer cleanup()
	if _, err = pool.Exec(ctx, `INSERT INTO users (id, email, status) VALUES ($1, 'profile-feed@invalid.example', 'active')`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO users (id, email, status) VALUES ($1, 'profile-viewer@invalid.example', 'active')`, viewerID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO user_profiles (user_id, handle, display_name) VALUES ($1, 'profile_storyteller', 'Profile Storyteller')`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO places (id, name, normalized_name, status, location, default_radius_meters, category)
		VALUES ($1, 'Profile Feed Park', 'profile feed park', 'active', ST_SetSRID(ST_MakePoint(-88.01, 41.79), 4326)::geography, 100, 'park')`, placeID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO echoes
		(id, author_user_id, place_id, title, content_mode, body_text, status, public_location, created_at, published_at, updated_at)
		VALUES
		($1, $4, $5, 'Source profile Echo', 'text', 'First public story.', 'published', ST_SetSRID(ST_MakePoint(-88.01, 41.79), 4326)::geography, now() - interval '2 hours', now() - interval '2 hours', now()),
		($2, $4, $5, 'Stitched profile Echo', 'text', 'Second public story.', 'published', ST_SetSRID(ST_MakePoint(-88.01, 41.79), 4326)::geography, now() - interval '1 hour', now() - interval '1 hour', now()),
		($3, $4, $5, 'Limited profile Echo', 'text', 'Must remain private.', 'limited', ST_SetSRID(ST_MakePoint(-88.01, 41.79), 4326)::geography, now(), NULL, now())`, sourceID, stitchID, limitedID, userID, placeID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO echo_links (id, source_echo_id, target_echo_id, link_type, snapshot_status) VALUES ($1, $2, $3, 'stitch', 'ready')`, linkID, sourceID, stitchID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO echo_rank_snapshots
		(echo_id, algorithm_version, recency_score, engagement_score, locality_score, quality_score, subscription_score, risk_penalty, notability_score, ranking_reason)
		VALUES ($1, 'profile-test', 0.1, 0.1, 0.1, 1, 0, 0, 0.9, 'notable'),
		       ($2, 'profile-test', 0.8, 0.1, 0.1, 1, 0, 0, 0.2, 'recent')`, sourceID, stitchID); err != nil {
		t.Fatal(err)
	}
	store, err := NewPostgresStore(pool, []byte("integration-exact-location-key-32-bytes"), []byte("integration-push-token-key-at-least-32"))
	if err != nil {
		t.Fatal(err)
	}
	notablePage, err := store.ListPublishedByPlaceSortedPage(ctx, placeID, "", 1, "notable")
	if err != nil || len(notablePage.Records) != 1 || notablePage.Records[0].ID != sourceID || notablePage.NextCursor == "" {
		t.Fatalf("notable Place page = %+v err=%v", notablePage, err)
	}
	notableNext, err := store.ListPublishedByPlaceSortedPage(ctx, placeID, notablePage.NextCursor, 1, "notable")
	if err != nil || len(notableNext.Records) != 1 || notableNext.Records[0].ID != stitchID {
		t.Fatalf("notable Place second page = %+v err=%v", notableNext, err)
	}
	oldestPage, err := store.ListPublishedByPlaceSortedPage(ctx, placeID, "", 2, "oldest")
	if err != nil || len(oldestPage.Records) != 2 || oldestPage.Records[0].ID != sourceID || oldestPage.Records[1].ID != stitchID {
		t.Fatalf("oldest Place page = %+v err=%v", oldestPage, err)
	}
	placeHandler := NewHandlerWithConfig(store, HandlerConfig{Echoes: store})
	request := httptest.NewRequest(http.MethodGet, "/places/"+placeID+"/echoes?limit=1", nil)
	response := httptest.NewRecorder()
	placeHandler.ServeHTTP(response, request)
	var defaultPlacePage struct {
		Echoes []EchoResponse `json:"echoes"`
	}
	if response.Code != http.StatusOK || json.NewDecoder(response.Body).Decode(&defaultPlacePage) != nil || len(defaultPlacePage.Echoes) != 1 || defaultPlacePage.Echoes[0].ID != sourceID {
		t.Fatalf("default notable Place page status=%d body=%s decoded=%+v", response.Code, response.Body.String(), defaultPlacePage)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO hidden_targets (user_id, target_type, target_id) VALUES ($1, 'echo', $2)`, viewerID, stitchID); err != nil {
		t.Fatal(err)
	}
	visiblePage, err := store.ListVisiblePublishedByPlaceSortedPage(ctx, placeID, viewerID, "", 20, "recent")
	if err != nil || len(visiblePage.Records) != 1 || visiblePage.Records[0].ID != sourceID {
		t.Fatalf("hidden-filtered Place page = %+v err=%v", visiblePage, err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO blocks (blocker_user_id, blocked_user_id) VALUES ($1, $2)`, viewerID, userID); err != nil {
		t.Fatal(err)
	}
	visiblePage, err = store.ListVisiblePublishedByPlaceSortedPage(ctx, placeID, viewerID, "", 20, "recent")
	if err != nil || len(visiblePage.Records) != 0 {
		t.Fatalf("blocked-filtered Place page = %+v err=%v", visiblePage, err)
	}
	server := NewServer(Config{Store: store, Profiles: store})
	request = httptest.NewRequest(http.MethodGet, "/users/profile_storyteller/echoes?limit=1", nil)
	request.Header.Set("X-EchoTrails-User-ID", userID)
	response = httptest.NewRecorder()
	server.Router().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("profile Echo page status=%d body=%s", response.Code, response.Body.String())
	}
	var first struct {
		Echoes     []EchoResponse `json:"echoes"`
		NextCursor string         `json:"next_cursor"`
	}
	if err = json.NewDecoder(response.Body).Decode(&first); err != nil {
		t.Fatal(err)
	}
	if len(first.Echoes) != 1 || first.Echoes[0].ID != stitchID || first.Echoes[0].StitchSource == nil || first.Echoes[0].StitchSource.EchoID != sourceID || first.NextCursor == "" {
		t.Fatalf("first profile Echo page = %+v", first)
	}
	request = httptest.NewRequest(http.MethodGet, "/users/profile_storyteller/echoes?limit=1&cursor="+url.QueryEscape(first.NextCursor), nil)
	request.Header.Set("X-EchoTrails-User-ID", userID)
	response = httptest.NewRecorder()
	server.Router().ServeHTTP(response, request)
	var second struct {
		Echoes []EchoResponse `json:"echoes"`
	}
	if response.Code != http.StatusOK || json.NewDecoder(response.Body).Decode(&second) != nil || len(second.Echoes) != 1 || second.Echoes[0].ID != sourceID {
		t.Fatalf("second profile Echo page status=%d body=%s decoded=%+v", response.Code, response.Body.String(), second)
	}
	request = httptest.NewRequest(http.MethodGet, "/users/profile_storyteller/echoes?limit=20", nil)
	response = httptest.NewRecorder()
	server.Router().ServeHTTP(response, request)
	var public struct {
		Echoes []EchoResponse `json:"echoes"`
	}
	if response.Code != http.StatusOK || json.NewDecoder(response.Body).Decode(&public) != nil || len(public.Echoes) != 2 {
		t.Fatalf("public profile Echo page status=%d body=%s decoded=%+v", response.Code, response.Body.String(), public)
	}
	request = httptest.NewRequest(http.MethodGet, "/users/profile_storyteller/echoes?limit=20", nil)
	request.Header.Set("X-EchoTrails-User-ID", viewerID)
	response = httptest.NewRecorder()
	server.Router().ServeHTTP(response, request)
	var blocked struct {
		Echoes []EchoResponse `json:"echoes"`
	}
	if response.Code != http.StatusOK || json.NewDecoder(response.Body).Decode(&blocked) != nil || len(blocked.Echoes) != 0 {
		t.Fatalf("blocked profile Echo page status=%d body=%s decoded=%+v", response.Code, response.Body.String(), blocked)
	}
}

func TestIntegrationAccountBootstrapUsesInviteConsentAndProfileTruth(t *testing.T) {
	if os.Getenv("ECHOTRAILS_INTEGRATION") != "1" {
		t.Skip("set ECHOTRAILS_INTEGRATION=1")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("ECHOTRAILS_INTEGRATION_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	const userID = "019f4a25-0001-7000-8000-000000000001"
	_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	defer pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	if _, err = pool.Exec(ctx, `INSERT INTO users (id, email, beta_access_at) VALUES ($1, 'bootstrap@invalid.example', now())`, userID); err != nil {
		t.Fatal(err)
	}
	store, err := NewPostgresStore(pool, []byte("integration-exact-location-key-32-bytes"), []byte("integration-push-token-key-at-least-32"))
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.GetAccountAccessState(ctx, userID)
	if err != nil || !state.BetaAccessAccepted || state.ConsentAccepted || state.ProfileConfigured {
		t.Fatalf("initial access state=%+v err=%v", state, err)
	}
	current := CurrentConsentVersion()
	if err = store.AcceptConsent(ctx, userID, ConsentAcceptanceRequest{TermsVersion: current.TermsVersion, PrivacyVersion: current.PrivacyVersion}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err = store.UpsertProfile(ctx, userID, "bootstrap@invalid.example", UpsertProfileRequest{Handle: "bootstrap_user", DisplayName: "Bootstrap User"}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	state, err = store.GetAccountAccessState(ctx, userID)
	if err != nil || !state.BetaAccessAccepted || !state.ConsentAccepted || !state.ProfileConfigured {
		t.Fatalf("ready access state=%+v err=%v", state, err)
	}
	if state.QuoteWarningAcknowledged {
		t.Fatal("quote warning unexpectedly acknowledged")
	}
	if _, err = pool.Exec(ctx, `INSERT INTO user_acknowledgements (user_id, acknowledgement_type)
		VALUES ($1, 'stitch_reply_snapshot_warning')`, userID); err != nil {
		t.Fatal(err)
	}
	state, err = store.GetAccountAccessState(ctx, userID)
	if err != nil || !state.QuoteWarningAcknowledged {
		t.Fatalf("quote acknowledgement state=%+v err=%v", state, err)
	}
}

func TestIntegrationDeletionRequestImmediatelyRemovesPublicAccess(t *testing.T) {
	if os.Getenv("ECHOTRAILS_INTEGRATION") != "1" {
		t.Skip("set ECHOTRAILS_INTEGRATION=1")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("ECHOTRAILS_INTEGRATION_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	const (
		userID  = "019f4a25-0011-7000-8000-000000000011"
		placeID = "019f4a25-0012-7000-8000-000000000012"
		echoID  = "019f4a25-0013-7000-8000-000000000013"
		replyID = "019f4a25-0014-7000-8000-000000000014"
		assetID = "019f4a25-0015-7000-8000-000000000015"
	)
	cleanup := func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM jobs WHERE payload->>'privacy_job_id' IN (SELECT id::text FROM privacy_jobs WHERE user_id = $1)`, userID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM media_assets WHERE id = $1`, assetID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM replies WHERE id = $1`, replyID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM echoes WHERE id = $1`, echoID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM places WHERE id = $1`, placeID)
	}
	cleanup()
	defer cleanup()
	if _, err = pool.Exec(ctx, `INSERT INTO users (id, email, beta_access_at) VALUES ($1, 'delete-now@invalid.example', now())`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO user_profiles (user_id, handle, display_name) VALUES ($1, 'delete_now', 'Delete Now')`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO places (id, name, normalized_name, status, location, default_radius_meters)
		VALUES ($1, 'Deletion Test Park', 'deletion test park', 'active', ST_SetSRID(ST_MakePoint(-88.01, 41.79), 4326)::geography, 100)`, placeID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO echoes
		(id, author_user_id, place_id, title, content_mode, body_text, status, public_location, published_at)
		VALUES ($1, $2, $3, 'Public before deletion', 'text', 'Remove immediately.', 'published',
		ST_SetSRID(ST_MakePoint(-88.01, 41.79), 4326)::geography, now())`, echoID, userID, placeID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO replies (id, echo_id, author_user_id, body_text, status) VALUES ($1, $2, $3, 'Public Reply', 'published')`, replyID, echoID, userID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO media_assets
		(id, owner_user_id, echo_id, media_type, storage_bucket, object_key, content_type, byte_size, status)
		VALUES ($1, $2, $3, 'image', 'integration', 'privacy/delete-now.jpg', 'image/jpeg', 12, 'approved')`, assetID, userID, echoID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO sessions (id, user_id, token_hash, expires_at) VALUES ('019f4a25-0016-7000-8000-000000000016', $1, decode('aa', 'hex'), now() + interval '1 day')`, userID); err != nil {
		t.Fatal(err)
	}
	store, err := NewPostgresStore(pool, []byte("integration-exact-location-key-32-bytes"), []byte("integration-push-token-key-at-least-32"))
	if err != nil {
		t.Fatal(err)
	}
	device, err := store.RegisterDevice(ctx, userID, DeviceRegistrationRequest{Token: strings.Repeat("a", 64), Environment: "sandbox"}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	job, err := store.QueuePrivacyJob(ctx, userID, "delete", time.Now().UTC())
	if err != nil || job.Status != "queued" {
		t.Fatalf("delete job=%+v err=%v", job, err)
	}
	var userStatus, echoStatus, replyStatus, replyBody, assetStatus string
	var sessionRevoked, deviceDisabled bool
	var encryptedPushTokenPresent bool
	if err = pool.QueryRow(ctx, `SELECT u.status, e.status, r.status, r.body_text, a.status,
		(s.revoked_at IS NOT NULL), (d.disabled_at IS NOT NULL), (d.push_token_ciphertext IS NOT NULL)
		FROM users u JOIN echoes e ON e.author_user_id = u.id JOIN replies r ON r.author_user_id = u.id
		JOIN media_assets a ON a.owner_user_id = u.id JOIN sessions s ON s.user_id = u.id JOIN devices d ON d.id = $2
		WHERE u.id = $1`, userID, device.ID).Scan(&userStatus, &echoStatus, &replyStatus, &replyBody, &assetStatus, &sessionRevoked, &deviceDisabled, &encryptedPushTokenPresent); err != nil {
		t.Fatal(err)
	}
	if userStatus != "deleted" || echoStatus != "deleted" || replyStatus != "deleted" || replyBody != "Reply deleted" || assetStatus != "deleted" || !sessionRevoked || !deviceDisabled || encryptedPushTokenPresent {
		t.Fatalf("immediate deletion user=%s echo=%s reply=%s body=%q asset=%s session_revoked=%v device_disabled=%v token_present=%v",
			userStatus, echoStatus, replyStatus, replyBody, assetStatus, sessionRevoked, deviceDisabled, encryptedPushTokenPresent)
	}
	if _, found, profileErr := store.GetProfileByHandle(ctx, "delete_now"); profileErr != nil || found {
		t.Fatalf("deleted public profile found=%v err=%v", found, profileErr)
	}
}

func TestIntegrationMediaSubmitRequiresCompletedOrderedAttachments(t *testing.T) {
	if os.Getenv("ECHOTRAILS_INTEGRATION") != "1" {
		t.Skip("set ECHOTRAILS_INTEGRATION=1")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("ECHOTRAILS_INTEGRATION_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	const (
		userID   = "019f4a26-0001-7000-8000-000000000001"
		placeID  = "019f4a26-0002-7000-8000-000000000002"
		echoID   = "019f4a26-0003-7000-8000-000000000003"
		assetOne = "019f4a26-0004-7000-8000-000000000004"
		assetTwo = "019f4a26-0005-7000-8000-000000000005"
	)
	cleanup := func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM media_assets WHERE id IN ($1, $2)`, assetOne, assetTwo)
		_, _ = pool.Exec(context.Background(), `DELETE FROM echoes WHERE id = $1`, echoID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM places WHERE id = $1`, placeID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	}
	cleanup()
	defer cleanup()
	if _, err = pool.Exec(ctx, `INSERT INTO users (id, email, beta_access_at) VALUES ($1, 'media-submit@invalid.example', now())`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO user_profiles (user_id, handle, display_name) VALUES ($1, 'media_submit', 'Media Submit')`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO places (id, name, normalized_name, status, location, default_radius_meters, category)
		VALUES ($1, 'Media Submit Park', 'media submit park', 'active', ST_SetSRID(ST_MakePoint(-88.01, 41.79), 4326)::geography, 100, 'park')`, placeID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO echoes
		(id, author_user_id, place_id, title, content_mode, caption_text, status, public_location, sensitive_label)
		VALUES ($1, $2, $3, 'Ordered photo set', 'photo_set', 'Two completed photos.', 'draft',
		ST_SetSRID(ST_MakePoint(-88.01, 41.79), 4326)::geography, 'none')`, echoID, userID, placeID); err != nil {
		t.Fatal(err)
	}
	store, err := NewPostgresStore(pool, []byte("integration-exact-location-key-32-bytes"), []byte("integration-push-token-key-at-least-32"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Submit(ctx, echoID, userID, time.Now().UTC()); !errors.Is(err, ErrInvalidEcho) {
		t.Fatalf("empty photo submit err=%v want ErrInvalidEcho", err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO media_assets
		(id, owner_user_id, echo_id, media_type, storage_bucket, object_key, content_type, byte_size, status, sort_order)
		VALUES ($1, $2, $3, 'image', 'integration', 'media/original/one.jpg', 'image/jpeg', 1024, 'pending_upload', 0)`, assetOne, userID, echoID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Submit(ctx, echoID, userID, time.Now().UTC()); !errors.Is(err, ErrInvalidEcho) {
		t.Fatalf("pending photo submit err=%v want ErrInvalidEcho", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE media_assets SET status = 'uploaded' WHERE id = $1`, assetOne); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO media_assets
		(id, owner_user_id, echo_id, media_type, storage_bucket, object_key, content_type, byte_size, status, sort_order)
		VALUES ($1, $2, $3, 'video', 'integration', 'media/original/two.mp4', 'video/mp4', 1024, 'uploaded', 1)`, assetTwo, userID, echoID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Submit(ctx, echoID, userID, time.Now().UTC()); !errors.Is(err, ErrInvalidEcho) {
		t.Fatalf("mixed-type photo submit err=%v want ErrInvalidEcho", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE media_assets SET media_type = 'image', content_type = 'image/jpeg', object_key = 'media/original/two.jpg' WHERE id = $1`, assetTwo); err != nil {
		t.Fatal(err)
	}
	record, err := store.Submit(ctx, echoID, userID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != EchoStatusUploading {
		t.Fatalf("submitted media status=%q want %q", record.Status, EchoStatusUploading)
	}
}

func TestIntegrationSearchScaleP95(t *testing.T) {
	if os.Getenv("ECHOTRAILS_SCALE_TEST") != "1" {
		t.Skip("set ECHOTRAILS_SCALE_TEST=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, os.Getenv("ECHOTRAILS_INTEGRATION_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	userID := "019f4999-0002-7000-8000-000000000002"
	cleanup := func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM echoes WHERE author_user_id = $1`, userID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM places WHERE normalized_name LIKE 'benchmark place %' OR id = md5('benchmark-place-1')::uuid`)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	}
	cleanup()
	defer cleanup()
	if _, err = pool.Exec(ctx, `INSERT INTO users (id, email, beta_access_at) VALUES ($1, 'scale@invalid.example', now())`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO user_profiles (user_id, handle, display_name) VALUES ($1, 'scale_fixture', 'Scale Fixture')`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO places (id, name, normalized_name, status, location, default_radius_meters, category)
		SELECT md5('benchmark-place-' || value::text)::uuid, 'Benchmark Place ' || value, 'benchmark place ' || value,
		'active', ST_SetSRID(ST_MakePoint(-88.02 + (value % 100)::float8 / 10000, 41.79 + (value % 100)::float8 / 10000), 4326)::geography, 100, 'park'
		FROM generate_series(1, 10000) value`); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE places SET name = 'Benchmark Café' WHERE id = md5('benchmark-place-1')::uuid`); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO echoes (id, author_user_id, place_id, title, content_mode, body_text, status, public_location, sensitive_label, created_at, published_at, updated_at)
		SELECT overlay(overlay(md5('benchmark-echo-' || value::text) placing '7' from 13 for 1) placing '8' from 17 for 1)::uuid,
		$1, md5('benchmark-place-' || (((value - 1) % 10000) + 1)::text)::uuid,
		'Benchmark story ' || value, 'text', 'A searchable benchmark place story with enough safe content for ranking.', 'published',
		ST_SetSRID(ST_MakePoint(-88.01, 41.79), 4326)::geography, 'none', now() - ((value % 240) || ' hours')::interval,
		now() - ((value % 240) || ' hours')::interval, now() FROM generate_series(1, 100000) value`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO echo_rank_snapshots (echo_id, algorithm_version, recency_score, engagement_score, locality_score, quality_score, subscription_score, risk_penalty, notability_score, ranking_reason, evergreen_eligible)
		SELECT id, 'scale-fixture', .5, .5, 1, 1, 0, 0, .65, 'notable', false FROM echoes WHERE author_user_id = $1`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO media_assets
		(id, owner_user_id, echo_id, media_type, storage_bucket, object_key, content_type, byte_size, status)
		VALUES (md5('benchmark-transcript-asset')::uuid, $1,
		overlay(overlay(md5('benchmark-echo-1') placing '7' from 13 for 1) placing '8' from 17 for 1)::uuid,
		'audio', 'scale-private', 'scale/transcript.m4a', 'audio/mp4', 1, 'approved')`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO media_transcripts (id, media_asset_id, status, locale, transcript_text, model_name)
		VALUES (md5('benchmark-transcript')::uuid, md5('benchmark-transcript-asset')::uuid, 'succeeded', 'en', 'A café spokenneedle oral history available only in the transcript.', 'scale-whisper')`); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"places", "echoes", "echo_rank_snapshots", "media_assets", "media_transcripts"} {
		if _, err = pool.Exec(ctx, `ANALYZE `+table); err != nil {
			t.Fatal(err)
		}
	}
	store, err := NewPostgresStore(pool, []byte("integration-exact-location-key-32-bytes"), []byte("integration-push-token-key-at-least-32"))
	if err != nil {
		t.Fatal(err)
	}
	accentedPlaces, err := store.SearchPlaces(ctx, "benchmark cafe", false, nil, nil)
	if err != nil || len(accentedPlaces) == 0 || accentedPlaces[0].Name != "Benchmark Café" {
		t.Fatalf("unaccented Place search=%v err=%v", accentedPlaces, err)
	}
	transcriptResult, err := store.SearchDiscovery(ctx, SearchRequest{Query: "cafe spokenneedle", Scope: "city", Limit: 20})
	if err != nil || len(transcriptResult.Echoes) != 1 {
		t.Fatalf("transcript-only search count=%d err=%v", len(transcriptResult.Echoes), err)
	}
	request := SearchRequest{Query: "benchmark story", Scope: "city", Limit: 20}
	placeStarted := time.Now()
	if _, err = store.SearchPlaces(ctx, request.Query, false, nil, nil); err != nil {
		t.Fatal(err)
	}
	t.Logf("place search duration=%s", time.Since(placeStarted))
	firstPage, err := store.SearchDiscovery(ctx, request)
	if err != nil || len(firstPage.Echoes) != 20 || firstPage.NextCursor == "" {
		t.Fatalf("first search page count=%d cursor=%q err=%v", len(firstPage.Echoes), firstPage.NextCursor, err)
	}
	request.Cursor = firstPage.NextCursor
	secondPage, err := store.SearchDiscovery(ctx, request)
	if err != nil || len(secondPage.Echoes) != 20 {
		t.Fatalf("second search page count=%d err=%v", len(secondPage.Echoes), err)
	}
	firstIDs := map[string]bool{}
	for _, result := range firstPage.Echoes {
		firstIDs[result.Echo.ID] = true
	}
	for _, result := range secondPage.Echoes {
		if firstIDs[result.Echo.ID] {
			t.Fatalf("search cursor repeated Echo %s", result.Echo.ID)
		}
	}
	request.Cursor = ""
	if _, err = store.SearchDiscovery(ctx, request); err != nil {
		t.Fatal(err)
	}
	durations := make([]time.Duration, 30)
	for index := range durations {
		started := time.Now()
		result, searchErr := store.SearchDiscovery(ctx, request)
		if searchErr != nil || len(result.Echoes) != 20 {
			t.Fatalf("search result count=%d err=%v", len(result.Echoes), searchErr)
		}
		durations[index] = time.Since(started)
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p95 := durations[28]
	t.Logf("search p95=%s with 10,000 Places and 100,000 Echoes", p95)
	if p95 >= 300*time.Millisecond {
		t.Fatalf("search p95 %s exceeds 300ms", p95)
	}
}

func TestIntegrationDownersGroveCreationPrivacyAndNegativePaths(t *testing.T) {
	if os.Getenv("ECHOTRAILS_INTEGRATION") != "1" {
		t.Skip("set ECHOTRAILS_INTEGRATION=1")
	}
	databaseURL := os.Getenv("ECHOTRAILS_INTEGRATION_DATABASE_URL")
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	userID := "019f4999-0001-7000-8000-000000000001"
	cleanup := func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM echoes WHERE author_user_id = $1`, userID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	}
	cleanup()
	_, err = pool.Exec(context.Background(), `INSERT INTO users (id, email, beta_access_at) VALUES ($1, 'integration-creation@invalid.example', now()) ON CONFLICT (id) DO UPDATE SET beta_access_at = now()`, userID)
	if err == nil {
		_, err = pool.Exec(context.Background(), `INSERT INTO user_profiles (user_id, handle, display_name) VALUES ($1, 'integration_story', 'Integration Story') ON CONFLICT (user_id) DO NOTHING`, userID)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	store, err := NewPostgresStore(pool, []byte("integration-exact-location-key-32-bytes"), []byte("integration-push-token-key-at-least-32"))
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandlerWithConfig(store, HandlerConfig{Echoes: store, Profiles: store, Authorizations: store, Social: store, Engagement: store, Discovery: store, Account: store, Appeals: store, CreationSigningKey: []byte("integration-creation-signing-key-32"), PublicLocationKey: []byte("integration-public-location-key-32")})
	headers := authHeaders(userID, "integration-creation@invalid.example")
	now := time.Now().UTC()
	proofBody := fmt.Sprintf(`{"place_id":"%s","latitude":41.7975912,"longitude":-88.0153297,"accuracy_meters":10,"captured_at":"%s"}`, PlacePrincePondID, now.Format(time.RFC3339Nano))
	proof := doJSON(t, handler, http.MethodPost, "/creation-tokens", proofBody, headers, map[string]string{"Idempotency-Key": "integration-proof"})
	if proof.Code != http.StatusCreated {
		t.Fatalf("proof status=%d body=%s", proof.Code, proof.Body.String())
	}
	var authorization struct {
		CreationToken string `json:"creation_token"`
	}
	if err = json.NewDecoder(proof.Body).Decode(&authorization); err != nil {
		t.Fatal(err)
	}
	createBody := fmt.Sprintf(`{"place_id":"%s","content_mode":"text","title":"Integration place memory","body_text":"A safe automated release-gate story.","creation_token":%q}`, PlacePrincePondID, authorization.CreationToken)
	created := doJSON(t, handler, http.MethodPost, "/echoes", createBody, headers, map[string]string{"Idempotency-Key": "integration-echo"})
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	if strings.Contains(created.Body.String(), "41.7975912") || strings.Contains(created.Body.String(), "exact_location") {
		t.Fatalf("public response leaked exact location: %s", created.Body.String())
	}
	restricted := fmt.Sprintf(`{"place_id":"%s","latitude":41.8065417,"longitude":-88.0122581,"accuracy_meters":10,"captured_at":"%s"}`, PlaceNorthHighSchoolID, now.Format(time.RFC3339Nano))
	response := doJSON(t, handler, http.MethodPost, "/creation-tokens", restricted, headers, map[string]string{"Idempotency-Key": "integration-restricted"})
	if response.Code < 400 || response.Code >= 500 {
		t.Fatalf("restricted place accepted: %d %s", response.Code, response.Body.String())
	}
	outside := fmt.Sprintf(`{"place_id":"%s","latitude":41.70,"longitude":-88.20,"accuracy_meters":10,"captured_at":"%s"}`, PlacePrincePondID, now.Format(time.RFC3339Nano))
	response = doJSON(t, handler, http.MethodPost, "/creation-tokens", outside, headers, map[string]string{"Idempotency-Key": "integration-outside"})
	if response.Code < 400 || response.Code >= 500 {
		t.Fatalf("outside proof accepted: %d %s", response.Code, response.Body.String())
	}
}

func TestIntegrationPrivateS3RoundTrip(t *testing.T) {
	if os.Getenv("ECHOTRAILS_INTEGRATION") != "1" {
		t.Skip("set ECHOTRAILS_INTEGRATION=1")
	}
	ctx := context.Background()
	config, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion("us-east-1"), awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("integration", "integration-secret", "")))
	if err != nil {
		t.Fatal(err)
	}
	client := s3.NewFromConfig(config, func(options *s3.Options) {
		options.BaseEndpoint = aws.String("http://127.0.0.1:59000")
		options.UsePathStyle = true
	})
	storage, _ := NewS3ObjectStorage(client)
	url, err := storage.PresignPut(ctx, "echotrails-integration", "integration/private.txt", "text/plain", 12, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest(http.MethodPut, url, strings.NewReader("private-data"))
	request.Header.Set("Content-Type", "text/plain")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode/100 != 2 {
		t.Fatalf("put status %d", response.StatusCode)
	}
	info, err := storage.Head(ctx, "echotrails-integration", "integration/private.txt")
	if err != nil || info.ByteSize != 12 {
		t.Fatalf("head=%+v err=%v", info, err)
	}
}
