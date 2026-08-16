package media

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

type integrationObjectWriter struct{}

func (integrationObjectWriter) PutObject(context.Context, string, string, string, []byte) error {
	return nil
}

func (integrationObjectWriter) DeleteObject(context.Context, string, string) error { return nil }

func TestIntegrationModerationFixture(t *testing.T) {
	if os.Getenv("ECHOTRAILS_INTEGRATION") != "1" {
		t.Skip("set ECHOTRAILS_INTEGRATION=1")
	}
	service, err := NewHTTPMediaServices("http://127.0.0.1:58081", "integration-moderation-token", nil)
	if err != nil {
		t.Fatal(err)
	}
	review, err := service.ScanText(context.Background(), EchoText{EchoID: "integration", Title: "Safe place story"})
	if err != nil {
		t.Fatal(err)
	}
	if review.Status != ModerationStatusApproved || review.ScannerName == "" {
		t.Fatalf("unexpected review: %+v", review)
	}
}

func TestIntegrationAutomatedRejectionCreatesAppealableActivity(t *testing.T) {
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
		userID  = "019f4a41-0001-7000-8000-000000000001"
		echoID  = "019f4a41-0002-7000-8000-000000000002"
		placeID = "019f486a-0001-7000-8000-000000000001"
	)
	cleanup := func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM jobs WHERE payload->>'notification_id' IN (SELECT id::text FROM notifications WHERE echo_id = $1)`, echoID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM notifications WHERE echo_id = $1`, echoID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM moderation_actions WHERE echo_id = $1`, echoID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM echoes WHERE id = $1`, echoID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	}
	cleanup()
	defer cleanup()
	if _, err = pool.Exec(ctx, `INSERT INTO users (id, email, status) VALUES ($1, 'auto-review@invalid.example', 'active')`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO echoes
		(id, author_user_id, place_id, title, content_mode, body_text, status, public_location)
		VALUES ($1, $2, $3, 'Automated review fixture', 'text', 'Rejected synthetic policy fixture.', 'pending_review',
		ST_SetSRID(ST_MakePoint(-88.0153, 41.7976), 4326)::geography)`, echoID, userID, placeID); err != nil {
		t.Fatal(err)
	}
	catalog := &PostgresCatalog{pool: pool}
	if err = catalog.SaveEchoModerationReview(ctx, EchoModerationReview{EchoID: echoID, Status: ModerationStatusRejected, RiskLevel: RiskHigh, ScannerName: "ShieldGemma:pinned", Categories: map[string]bool{"unsafe": true}}); err != nil {
		t.Fatal(err)
	}
	var status, actionID, notificationActionID string
	if err = pool.QueryRow(ctx, `SELECT echo.status, action.id::text, notification.payload->>'moderation_action_id'
		FROM echoes echo JOIN moderation_actions action ON action.echo_id = echo.id
		JOIN notifications notification ON notification.echo_id = echo.id WHERE echo.id = $1`, echoID).Scan(&status, &actionID, &notificationActionID); err != nil {
		t.Fatal(err)
	}
	if status != "limited" || actionID == "" || actionID != notificationActionID {
		t.Fatalf("status=%s action=%s notification_action=%s", status, actionID, notificationActionID)
	}
}

func TestIntegrationQuoteSnapshotDoesNotBlockMediaStitchPublication(t *testing.T) {
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
		userID     = "019f4a42-0001-7000-8000-000000000001"
		placeID    = "019f4a42-0002-7000-8000-000000000002"
		echoID     = "019f4a42-0003-7000-8000-000000000003"
		photoID    = "019f4a42-0004-7000-8000-000000000004"
		quoteID    = "019f4a42-0005-7000-8000-000000000005"
		derivative = "019f4a42-0006-7000-8000-000000000006"
		reviewID   = "019f4a42-0007-7000-8000-000000000007"
		sourceID   = "019f4a42-0008-7000-8000-000000000008"
		replyID    = "019f4a42-0009-7000-8000-000000000009"
		linkID     = "019f4a42-000a-7000-8000-00000000000a"
	)
	cleanup := func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM jobs WHERE payload->>'echo_id' = $1`, echoID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM echoes WHERE id IN ($1, $2)`, echoID, sourceID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM places WHERE id = $1`, placeID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	}
	cleanup()
	defer cleanup()
	if _, err = pool.Exec(ctx, `INSERT INTO users (id, email, status) VALUES ($1, 'snapshot-media@invalid.example', 'active')`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO places (id, name, normalized_name, status, location, default_radius_meters, category)
		VALUES ($1, 'Snapshot Media Park', 'snapshot media park', 'active', ST_SetSRID(ST_MakePoint(-88.01, 41.79), 4326)::geography, 100, 'park')`, placeID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO echoes
		(id, author_user_id, place_id, title, content_mode, caption_text, status, public_location)
		VALUES ($1, $2, $3, 'Media Stitch', 'photo_set', 'A photo Stitch with a quoted Reply.', 'uploading',
		ST_SetSRID(ST_MakePoint(-88.01, 41.79), 4326)::geography)`, echoID, userID, placeID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO echoes
		(id, author_user_id, place_id, title, content_mode, body_text, status, public_location, published_at)
		VALUES ($1, $2, $3, 'Source Echo', 'text', 'Source body.', 'published',
		ST_SetSRID(ST_MakePoint(-88.01, 41.79), 4326)::geography, now())`, sourceID, userID, placeID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO replies (id, echo_id, author_user_id, body_text, status)
		VALUES ($1, $2, $3, 'Quoted source Reply.', 'published')`, replyID, sourceID, userID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO echo_links
		(id, source_echo_id, target_echo_id, link_type, quoted_reply_id, snapshot_status)
		VALUES ($1, $2, $3, 'stitch', $4, 'queued')`, linkID, sourceID, echoID, replyID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO media_assets
		(id, owner_user_id, echo_id, media_type, storage_bucket, object_key, content_type, byte_size, status, sort_order)
		VALUES ($1, $2, $3, 'image', 'integration', 'media/derivatives/photo.jpg', 'image/jpeg', 1024, 'approved', 0),
		($4, $2, $3, 'quote_snapshot', 'integration', 'media/derivatives/quote.png', 'image/png', 1024, 'approved', 0)`, photoID, userID, echoID, quoteID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO media_derivatives
		(id, media_asset_id, derivative_type, storage_bucket, object_key, content_type, byte_size)
		VALUES ($1, $2, 'preview', 'integration', 'media/derivatives/photo-preview.jpg', 'image/jpeg', 512)`, derivative, photoID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO echo_moderation_reviews
		(id, echo_id, status, risk_level, scanner_name) VALUES ($1, $2, 'approved', 'low', 'ShieldGemma:pinned')`, reviewID, echoID); err != nil {
		t.Fatal(err)
	}
	reconcile := func() {
		tx, beginErr := pool.Begin(ctx)
		if beginErr != nil {
			t.Fatal(beginErr)
		}
		catalog := &PostgresCatalog{pool: pool}
		if reconcileErr := catalog.reconcileMediaTx(ctx, tx, photoID); reconcileErr != nil {
			_ = tx.Rollback(ctx)
			t.Fatal(reconcileErr)
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			t.Fatal(commitErr)
		}
	}
	reconcile()
	var status string
	if err = pool.QueryRow(ctx, `SELECT status FROM echoes WHERE id = $1`, echoID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "processing" {
		t.Fatalf("queued snapshot media Stitch status=%q want processing", status)
	}
	if _, err = pool.Exec(ctx, `UPDATE echo_links SET snapshot_status = 'ready', quote_snapshot_media_asset_id = $2 WHERE id = $1`, linkID, quoteID); err != nil {
		t.Fatal(err)
	}
	reconcile()
	if err = pool.QueryRow(ctx, `SELECT status FROM echoes WHERE id = $1`, echoID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "published" {
		t.Fatalf("ready snapshot media Stitch status=%q want published", status)
	}
}

func TestIntegrationTextStitchPublishesOnlyAfterSnapshotIsStored(t *testing.T) {
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
		userID   = "019f4a43-0001-7000-8000-000000000001"
		placeID  = "019f4a43-0002-7000-8000-000000000002"
		sourceID = "019f4a43-0003-7000-8000-000000000003"
		targetID = "019f4a43-0004-7000-8000-000000000004"
		replyID  = "019f4a43-0005-7000-8000-000000000005"
		linkID   = "019f4a43-0006-7000-8000-000000000006"
	)
	cleanup := func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM jobs WHERE payload->>'echo_id' = $1`, targetID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM echoes WHERE id IN ($1, $2)`, targetID, sourceID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM places WHERE id = $1`, placeID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	}
	cleanup()
	defer cleanup()
	if _, err = pool.Exec(ctx, `INSERT INTO users (id, email, status) VALUES ($1, 'snapshot-text@invalid.example', 'active')`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO places (id, name, normalized_name, status, location, default_radius_meters, category)
		VALUES ($1, 'Snapshot Text Park', 'snapshot text park', 'active', ST_SetSRID(ST_MakePoint(-88.01, 41.79), 4326)::geography, 100, 'park')`, placeID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO echoes
		(id, author_user_id, place_id, title, content_mode, body_text, status, public_location, published_at)
		VALUES ($1, $2, $3, 'Source Echo', 'text', 'Source body.', 'published', ST_SetSRID(ST_MakePoint(-88.01, 41.79), 4326)::geography, now()),
		($4, $2, $3, 'Text Stitch', 'text', 'Target body.', 'processing', ST_SetSRID(ST_MakePoint(-88.01, 41.79), 4326)::geography, NULL)`, sourceID, userID, placeID, targetID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO replies (id, echo_id, author_user_id, body_text, status)
		VALUES ($1, $2, $3, 'The immutable quoted Reply.', 'published')`, replyID, sourceID, userID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO echo_links
		(id, source_echo_id, target_echo_id, link_type, quoted_reply_id, metadata, source_reply_revision, snapshot_status)
		VALUES ($1, $2, $3, 'stitch', $4,
		jsonb_build_object('quoted_reply_body', 'The immutable quoted Reply.', 'quoted_reply_author_handle', 'snapshot_author', 'source_title', 'Source Echo', 'source_author_handle', 'snapshot_author'),
		1, 'queued')`, linkID, sourceID, targetID, replyID); err != nil {
		t.Fatal(err)
	}
	catalog := &PostgresCatalog{pool: pool, objects: integrationObjectWriter{}, bucket: "integration"}
	if err = catalog.SaveEchoModerationReview(ctx, EchoModerationReview{EchoID: targetID, Status: ModerationStatusApproved, RiskLevel: RiskLow, ScannerName: "ShieldGemma:pinned"}); err != nil {
		t.Fatal(err)
	}
	var status string
	if err = pool.QueryRow(ctx, `SELECT status FROM echoes WHERE id = $1`, targetID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "processing" {
		t.Fatalf("queued text snapshot status=%q want processing", status)
	}
	snapshot := QuoteSnapshot{LinkID: linkID, TargetEchoID: targetID, OwnerUserID: userID, Body: "The immutable quoted Reply.", ReplyAuthorHandle: "snapshot_author", SourceEchoTitle: "Source Echo", SourceAuthorHandle: "snapshot_author"}
	pngData, err := RenderQuoteSnapshotPNG(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err = catalog.SaveQuoteSnapshot(ctx, RenderedQuoteSnapshot{QuoteSnapshot: snapshot, PNG: pngData, Width: QuoteSnapshotWidth, Height: QuoteSnapshotHeight}); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT status FROM echoes WHERE id = $1`, targetID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "published" {
		t.Fatalf("stored text snapshot status=%q want published", status)
	}
	if err = catalog.FailQuoteSnapshot(ctx, linkID, context.DeadlineExceeded); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT status FROM echoes WHERE id = $1`, targetID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "pending_review" {
		t.Fatalf("failed snapshot status=%q want pending_review", status)
	}
}

func TestReleaseDrillModerationHostOutageFailsClosed(t *testing.T) {
	// Acquire a real local endpoint and then remove it. This exercises the HTTP
	// adapter's outage path without depending on a reserved port being unused.
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	endpoint := server.URL
	server.Close()

	services, err := NewHTTPMediaServices(endpoint, "release-drill-token", &http.Client{})
	if err != nil {
		t.Fatal(err)
	}
	catalog := NewMemoryCatalog([]Asset{{
		ID: "moderation-outage-asset", StorageBucket: "private", ObjectKey: "media/original/outage.jpg",
		MediaType: "image", ContentType: "image/jpeg", Status: AssetStatusPendingReview,
	}})
	review, err := (ModerationProcessor{Catalog: catalog, Scanner: services}).ProcessAsset(context.Background(), "moderation-outage-asset")
	if err != nil {
		t.Fatalf("persist fail-closed review: %v", err)
	}
	persisted := catalog.ModerationReviews["moderation-outage-asset"]
	if review.Status != ModerationStatusNeedsReview || review.RiskLevel != RiskUnknown || review.ScannerName != "scanner-unavailable" {
		t.Fatalf("outage did not fail closed: %+v", review)
	}
	if persisted.Status != ModerationStatusNeedsReview || persisted.FailureReason == "" {
		t.Fatalf("fail-closed evidence was not persisted: %+v", persisted)
	}
	if catalog.Assets["moderation-outage-asset"].Status != AssetStatusPendingReview {
		t.Fatalf("outage changed private asset state: %+v", catalog.Assets["moderation-outage-asset"])
	}
	t.Logf("DRILL_EVIDENCE moderation_outage status=%s risk=%s scanner=%s media_public=false", review.Status, review.RiskLevel, review.ScannerName)
}
