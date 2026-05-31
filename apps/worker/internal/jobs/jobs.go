package jobs

import (
	"context"
	"errors"
	"log/slog"
	"sync"
)

const (
	TypeProcessMedia       = "process_media"
	TypeGenerateTranscript = "generate_transcript"
	TypeScanMediaSafety    = "scan_media_safety"
)

var ErrNoJobs = errors.New("no jobs available")

type Job struct {
	ID             string
	Type           string
	AssetID        string
	IdempotencyKey string
	Attempts       int
	MaxAttempts    int
}

func New(id, jobType, assetID string) Job {
	return Job{ID: id, Type: jobType, AssetID: assetID, IdempotencyKey: jobType + ":" + assetID, MaxAttempts: 3}
}

type Queue interface {
	Reserve(context.Context) (Job, error)
	Succeed(context.Context, Job) error
	Fail(context.Context, Job, error) error
}

type MemoryQueue struct {
	mu        sync.Mutex
	pending   []Job
	succeeded []Job
	failed    []Job
}

func NewMemoryQueue(jobs []Job) *MemoryQueue {
	return &MemoryQueue{pending: append([]Job(nil), jobs...)}
}

func (q *MemoryQueue) Reserve(context.Context) (Job, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.pending) == 0 {
		return Job{}, ErrNoJobs
	}
	job := q.pending[0]
	q.pending = q.pending[1:]
	return job, nil
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
		q.pending = append(q.pending, job)
		return nil
	}
	q.failed = append(q.failed, job)
	return nil
}

func (q *MemoryQueue) SucceededCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.succeeded)
}

type Processor struct {
	Queue  Queue
	Logger *slog.Logger
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
	case TypeProcessMedia, TypeGenerateTranscript, TypeScanMediaSafety:
		return true
	default:
		return false
	}
}
