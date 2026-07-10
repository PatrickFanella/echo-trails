package ranking

import (
	"context"
	"math"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestIntegrationTrustWeightedEngagementUsesAccountState(t *testing.T) {
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
		echoID        = "019f4a40-0001-7000-8000-000000000001"
		authorID      = "019f4a40-0002-7000-8000-000000000002"
		establishedID = "019f4a40-0003-7000-8000-000000000003"
		newID         = "019f4a40-0004-7000-8000-000000000004"
		restrictedID  = "019f4a40-0005-7000-8000-000000000005"
		bannedID      = "019f4a40-0006-7000-8000-000000000006"
		placeID       = "019f486a-0001-7000-8000-000000000001"
	)
	userIDs := []string{authorID, establishedID, newID, restrictedID, bannedID}
	cleanup := func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM echoes WHERE id = $1`, echoID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = ANY($1::uuid[])`, userIDs)
	}
	cleanup()
	defer cleanup()
	if _, err = pool.Exec(ctx, `INSERT INTO users (id, email, status, created_at) VALUES
		($1, 'rank-author@invalid.example', 'active', now() - interval '60 days'),
		($2, 'rank-established@invalid.example', 'active', now() - interval '60 days'),
		($3, 'rank-new@invalid.example', 'active', now()),
		($4, 'rank-restricted@invalid.example', 'restricted', now() - interval '60 days'),
		($5, 'rank-banned@invalid.example', 'banned', now() - interval '60 days')`, authorID, establishedID, newID, restrictedID, bannedID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO echoes
		(id, author_user_id, place_id, title, content_mode, body_text, status, public_location, published_at)
		VALUES ($1, $2, $3, 'Trust weighted fixture', 'text',
		'A sufficiently complete safe story for deterministic quality scoring.', 'published',
		ST_SetSRID(ST_MakePoint(-88.0153, 41.7976), 4326)::geography, now())`, echoID, authorID, placeID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO echo_engagement_events
		(id, echo_id, actor_user_id, event_type, event_weight, dedupe_key) VALUES
		('019f4a40-0011-7000-8000-000000000011', $1, $2, 'stitch', 10, 'rank-established'),
		('019f4a40-0012-7000-8000-000000000012', $1, $3, 'stitch', 10, 'rank-new'),
		('019f4a40-0013-7000-8000-000000000013', $1, $4, 'stitch', 10, 'rank-restricted'),
		('019f4a40-0014-7000-8000-000000000014', $1, $5, 'stitch', 10, 'rank-banned')`, echoID, establishedID, newID, restrictedID, bannedID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO user_trust_scores (user_id, weight, reason) VALUES
		($1, 1.0, 'fixture attempts to override new-account cap'),
		($2, 1.0, 'fixture attempts to override restriction cap'),
		($3, 1.0, 'fixture attempts to override ban')`, newID, restrictedID, bannedID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO replies (id, echo_id, author_user_id, body_text, status)
		VALUES ('019f4a40-0021-7000-8000-000000000021', $1, $2, 'A participant with prior activity at this Place.', 'published')`, echoID, establishedID); err != nil {
		t.Fatal(err)
	}
	engine, err := NewEngine(pool)
	if err != nil {
		t.Fatal(err)
	}
	if err = engine.RecomputeEcho(ctx, echoID); err != nil {
		t.Fatal(err)
	}
	var engagement, locality float64
	if err = pool.QueryRow(ctx, `SELECT engagement_score::float8, locality_score::float8 FROM echo_rank_snapshots WHERE echo_id = $1`, echoID).Scan(&engagement, &locality); err != nil {
		t.Fatal(err)
	}
	// (10*1.0 + 10*0.5 + 10*0.25 + 10*0) / 20 = 0.875.
	if math.Abs(engagement-0.875) > 0.000001 {
		t.Fatalf("engagement score=%f want 0.875", engagement)
	}
	if math.Abs(locality-0.35) > 0.000001 {
		t.Fatalf("locality score=%f want 0.35", locality)
	}
	t.Logf("RELEASE_EVIDENCE trust_weighted_engagement=%0.3f community_locality=%0.2f", engagement, locality)
}
