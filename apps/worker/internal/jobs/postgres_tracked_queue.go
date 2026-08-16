package jobs

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresTrackedQueue relays the authoritative Postgres outbox into Redis
// Streams and mirrors terminal/retry state back to Postgres.
type PostgresTrackedQueue struct {
	redis               *RedisQueue
	pool                *pgxpool.Pool
	pushDeliveryEnabled bool
}

func NewPostgresTrackedQueue(redisQueue *RedisQueue, pool *pgxpool.Pool) (*PostgresTrackedQueue, error) {
	if redisQueue == nil || pool == nil {
		return nil, errors.New("Redis queue and Postgres pool are required")
	}
	return &PostgresTrackedQueue{redis: redisQueue, pool: pool, pushDeliveryEnabled: true}, nil
}

// SetPushDeliveryEnabled leaves notification jobs queued in authoritative
// Postgres when APNs is unavailable. Activity remains readable from the API,
// and enabling APNs later relays the same jobs without manufacturing retries.
func (q *PostgresTrackedQueue) SetPushDeliveryEnabled(enabled bool) {
	q.pushDeliveryEnabled = enabled
}

func (q *PostgresTrackedQueue) RelayDue(ctx context.Context) error {
	if err := q.reconcileTerminalResults(ctx); err != nil {
		return err
	}
	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `SELECT id::text, job_type,
		coalesce(payload->>'asset_id', payload->>'link_id', payload->>'notification_id', payload->>'echo_id', payload->>'privacy_job_id', ''), attempts, max_attempts, run_after
		FROM jobs WHERE queue IN ('media', 'notifications', 'ranking', 'privacy', 'moderation')
		AND ($1::boolean OR queue <> 'notifications') AND status = 'queued' AND run_after <= now()
		ORDER BY run_after, id FOR UPDATE SKIP LOCKED LIMIT 100`, q.pushDeliveryEnabled)
	if err != nil {
		return err
	}
	jobsToRelay := []Job{}
	for rows.Next() {
		var job Job
		if err = rows.Scan(&job.ID, &job.Type, &job.AssetID, &job.Attempts, &job.MaxAttempts, &job.AvailableAt); err != nil {
			rows.Close()
			return err
		}
		job.IdempotencyKey = "postgres-job:" + job.ID
		jobsToRelay = append(jobsToRelay, job)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, job := range jobsToRelay {
		if err = q.redis.Enqueue(ctx, job); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE jobs SET status = 'running', locked_by = $2, locked_at = now(), updated_at = now() WHERE id = $1`, job.ID, q.redis.consumer); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (q *PostgresTrackedQueue) reconcileTerminalResults(ctx context.Context) error {
	rows, err := q.pool.Query(ctx, `SELECT id::text FROM jobs
		WHERE status IN ('queued', 'running') ORDER BY updated_at LIMIT 500`)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	keys := make([]string, len(ids))
	for index, id := range ids {
		keys[index] = "postgres-job:" + id
	}
	statuses, err := q.redis.ResultStatuses(ctx, keys)
	if err != nil {
		return err
	}
	for _, id := range ids {
		status := statuses["postgres-job:"+id]
		if status != "succeeded" && status != "dead" {
			continue
		}
		if _, err = q.pool.Exec(ctx, `UPDATE jobs SET status = $2, completed_at = coalesce(completed_at, now()),
			locked_at = NULL, locked_by = NULL, updated_at = now()
			WHERE id = $1 AND status IN ('queued', 'running')`, id, status); err != nil {
			return err
		}
	}
	return nil
}

func (q *PostgresTrackedQueue) Enqueue(ctx context.Context, job Job) error {
	return q.redis.Enqueue(ctx, job)
}
func (q *PostgresTrackedQueue) Reserve(ctx context.Context) (Job, error) {
	return q.redis.Reserve(ctx)
}
func (q *PostgresTrackedQueue) Succeed(ctx context.Context, job Job) error {
	if err := q.redis.Succeed(ctx, job); err != nil {
		return err
	}
	_, err := q.pool.Exec(ctx, `UPDATE jobs SET status = 'succeeded', completed_at = now(), updated_at = now(), locked_at = NULL, locked_by = NULL WHERE id = $1`, job.ID)
	return err
}
func (q *PostgresTrackedQueue) Fail(ctx context.Context, job Job, jobErr error) error {
	if err := q.redis.Fail(ctx, job, jobErr); err != nil {
		return err
	}
	nextAttempt := job.Attempts + 1
	status := "queued"
	var completedAt *time.Time
	var runAfter *time.Time
	if nextAttempt >= job.MaxAttempts {
		status = "dead"
		now := time.Now().UTC()
		completedAt = &now
	} else {
		// Redis owns the retry schedule, but keep Postgres authoritative and
		// observable by mirroring the same availability timestamp. This also
		// lets the orphan requeue path recover a job if Redis is unavailable.
		nextRun := q.redis.now().Add(backoff(nextAttempt))
		runAfter = &nextRun
	}
	_, err := q.pool.Exec(ctx, `UPDATE jobs SET status = $2, attempts = $3, last_error = $4,
		completed_at = $5, run_after = coalesce($6, run_after), locked_at = NULL, locked_by = NULL,
		updated_at = now() WHERE id = $1`, job.ID, status, nextAttempt, safeJobError(jobErr), completedAt, runAfter)
	return err
}

func (q *PostgresTrackedQueue) RequeueOrphans(ctx context.Context) error {
	_, err := q.pool.Exec(ctx, `UPDATE jobs SET status = 'queued', locked_by = NULL, locked_at = NULL, run_after = now(), updated_at = now()
		WHERE status = 'running' AND locked_at < now() - interval '30 minutes'`)
	return err
}

var _ Queue = (*PostgresTrackedQueue)(nil)
