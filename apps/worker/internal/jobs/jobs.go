package jobs

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

const (
	TypeProcessMedia        = "process_media"
	TypeGenerateTranscript  = "generate_transcript"
	TypeScanMediaSafety     = "scan_media_safety"
	TypeRenderReplySnapshot = "render_reply_snapshot"
	TypeSendPush            = "send_push"
	TypeRankEcho            = "rank_echo"
	TypePrivacyExport       = "privacy_export"
	TypePrivacyDelete       = "privacy_delete"
	TypeScanEchoSafety      = "scan_echo_safety"
)

var ErrNoJobs = errors.New("no jobs available")

type Job struct {
	ID             string
	Type           string
	AssetID        string
	IdempotencyKey string
	Attempts       int
	MaxAttempts    int
	AvailableAt    time.Time
	ReceiptID      string `json:"-"`
}

func New(id, jobType, assetID string) Job {
	return Job{ID: id, Type: jobType, AssetID: assetID, IdempotencyKey: jobType + ":" + assetID, MaxAttempts: 3}
}

type Queue interface {
	Enqueue(context.Context, Job) error
	Reserve(context.Context) (Job, error)
	Succeed(context.Context, Job) error
	Fail(context.Context, Job, error) error
}

type Clock interface{ Now() time.Time }

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

type MemoryQueue struct {
	mu        sync.Mutex
	pending   []Job
	succeeded []Job
	failed    []Job
	clock     Clock
	seen      map[string]struct{}
}

func NewMemoryQueue(jobs []Job) *MemoryQueue {
	queue := &MemoryQueue{pending: append([]Job(nil), jobs...), clock: SystemClock{}, seen: map[string]struct{}{}}
	for _, job := range jobs {
		if job.IdempotencyKey != "" {
			queue.seen[job.IdempotencyKey] = struct{}{}
		}
	}
	return queue
}

func (q *MemoryQueue) Enqueue(_ context.Context, job Job) error {
	if err := validateJob(job); err != nil {
		return err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.seen == nil {
		q.seen = map[string]struct{}{}
	}
	if _, ok := q.seen[job.IdempotencyKey]; ok {
		return nil
	}
	q.seen[job.IdempotencyKey] = struct{}{}
	q.pending = append(q.pending, job)
	return nil
}

func (q *MemoryQueue) Reserve(context.Context) (Job, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := q.now()
	for i, job := range q.pending {
		if !job.AvailableAt.IsZero() && job.AvailableAt.After(now) {
			continue
		}
		q.pending = append(q.pending[:i], q.pending[i+1:]...)
		return job, nil
	}
	if len(q.pending) == 0 {
		return Job{}, ErrNoJobs
	}
	return Job{}, ErrNoJobs
}

func (q *MemoryQueue) Succeed(_ context.Context, job Job) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.succeeded = append(q.succeeded, job)
	return nil
}

func (q *MemoryQueue) Fail(_ context.Context, job Job, _ error) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	job.Attempts++
	if job.Attempts < job.MaxAttempts {
		job.AvailableAt = q.now().Add(backoff(job.Attempts))
		q.pending = append(q.pending, job)
		return nil
	}
	q.failed = append(q.failed, job)
	return nil
}

func (q *MemoryQueue) FailedCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.failed)
}

func (q *MemoryQueue) PendingCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending)
}

func (q *MemoryQueue) now() time.Time {
	if q.clock == nil {
		return time.Now().UTC()
	}
	return q.clock.Now()
}

func (q *MemoryQueue) SucceededCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.succeeded)
}

func backoff(attempts int) time.Duration {
	if attempts <= 0 {
		return time.Second
	}
	if attempts > 5 {
		attempts = 5
	}
	return time.Duration(1<<uint(attempts-1)) * time.Second
}

type RedisConfig struct {
	URL        string
	StreamName string
	GroupName  string
	Consumer   string
}

func (c RedisConfig) Valid() bool {
	return c.URL != "" && c.StreamName != "" && c.GroupName != "" && c.Consumer != ""
}

type Processor struct {
	Queue   Queue
	Logger  *slog.Logger
	Handler Handler
}

type Handler interface {
	HandleJob(context.Context, Job) error
}

func (p Processor) ProcessOne(ctx context.Context) error {
	job, err := p.Queue.Reserve(ctx)
	if err != nil {
		return err
	}
	if p.Logger != nil {
		p.Logger.Info("processing job", "job_id", job.ID, "job_type", job.Type, "asset_id", job.AssetID)
	}
	if err := validateJob(job); err != nil {
		return p.Queue.Fail(ctx, job, err)
	}
	if p.Handler != nil {
		if err := p.Handler.HandleJob(ctx, job); err != nil {
			return p.Queue.Fail(ctx, job, err)
		}
	}
	return p.Queue.Succeed(ctx, job)
}

func validateJob(job Job) error {
	if !validJobType(job.Type) {
		return errors.New("unsupported job type")
	}
	if job.AssetID == "" || job.IdempotencyKey == "" {
		return errors.New("job missing idempotency fields")
	}
	if job.MaxAttempts <= 0 {
		return errors.New("job max attempts must be positive")
	}
	return nil
}

func validJobType(jobType string) bool {
	switch jobType {
	case TypeProcessMedia, TypeGenerateTranscript, TypeScanMediaSafety, TypeRenderReplySnapshot, TypeSendPush, TypeRankEcho, TypePrivacyExport, TypePrivacyDelete, TypeScanEchoSafety:
		return true
	default:
		return false
	}
}
