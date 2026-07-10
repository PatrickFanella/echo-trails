package jobs

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestIntegrationPostgresRedisRelay(t *testing.T) {
	databaseURL := os.Getenv("ECHOTRAILS_INTEGRATION_DATABASE_URL")
	redisURL := os.Getenv("ECHOTRAILS_INTEGRATION_REDIS_URL")
	if databaseURL == "" || redisURL == "" {
		t.Skip("integration services not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	id := integrationUUID(t)
	stream := "echotrails:integration:" + id
	queue, err := NewRedisQueue(ctx, RedisConfig{URL: redisURL, StreamName: stream, GroupName: "workers", Consumer: "integration"})
	if err != nil {
		t.Fatal(err)
	}
	defer queue.Close()
	tracked, err := NewPostgresTrackedQueue(queue, pool)
	if err != nil {
		t.Fatal(err)
	}
	// Make this fixture the oldest due job. The integration database is
	// intentionally reusable between runs, so unrelated queued work may exist
	// and RelayDue correctly relays it as well.
	_, err = pool.Exec(ctx, `INSERT INTO jobs (id, queue, job_type, payload, max_attempts, run_after)
		VALUES ($1, 'ranking', 'rank_echo', jsonb_build_object('echo_id', $2::text), 3, '2000-01-01T00:00:00Z')`, id, id)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM jobs WHERE id = $1`, id)
	if err = tracked.RelayDue(ctx); err != nil {
		t.Fatal(err)
	}
	job, err := tracked.Reserve(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if job.ID != id || job.AssetID != id || job.Type != TypeRankEcho {
		t.Fatalf("wrong relayed job: %+v", job)
	}
	if err = tracked.Succeed(ctx, job); err != nil {
		t.Fatal(err)
	}
	var status string
	if err = pool.QueryRow(ctx, `SELECT status FROM jobs WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "succeeded" {
		t.Fatalf("status = %s", status)
	}
	// Simulate Redis committing the terminal result while the Postgres status
	// write was lost. The next relay repairs authoritative state before looking
	// for due work.
	if _, err = pool.Exec(ctx, `UPDATE jobs SET status = 'running', completed_at = NULL WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if err = tracked.RelayDue(ctx); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT status FROM jobs WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "succeeded" {
		t.Fatalf("reconciled status = %s", status)
	}
}

func TestIntegrationDisabledPushDeliveryLeavesAuthoritativeJobQueued(t *testing.T) {
	databaseURL := os.Getenv("ECHOTRAILS_INTEGRATION_DATABASE_URL")
	redisURL := os.Getenv("ECHOTRAILS_INTEGRATION_REDIS_URL")
	if databaseURL == "" || redisURL == "" {
		t.Skip("integration services not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	id := integrationUUID(t)
	queue, err := NewRedisQueue(ctx, RedisConfig{URL: redisURL, StreamName: "echotrails:integration:push:" + id, GroupName: "workers", Consumer: "integration"})
	if err != nil {
		t.Fatal(err)
	}
	defer queue.Close()
	tracked, err := NewPostgresTrackedQueue(queue, pool)
	if err != nil {
		t.Fatal(err)
	}
	tracked.SetPushDeliveryEnabled(false)
	_, err = pool.Exec(ctx, `INSERT INTO jobs (id, queue, job_type, payload, max_attempts, run_after)
		VALUES ($1, 'notifications', 'send_push', jsonb_build_object('notification_id', $2::text), 5, '2000-01-01T00:00:00Z')`, id, id)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM jobs WHERE id = $1`, id)
	if err = tracked.RelayDue(ctx); err != nil {
		t.Fatal(err)
	}
	var status string
	if err = pool.QueryRow(ctx, `SELECT status FROM jobs WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "queued" {
		t.Fatalf("disabled push job status = %s, want queued", status)
	}
}

func integrationUUID(t *testing.T) string {
	t.Helper()
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		t.Fatal(err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}
