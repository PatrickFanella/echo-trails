package jobs

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type failingHandler struct{}

func (failingHandler) HandleJob(context.Context, Job) error { return errors.New("handler failed") }

func TestProcessorSucceedsValidMediaJob(t *testing.T) {
	queue := NewMemoryQueue([]Job{New("job-1", TypeProcessMedia, "asset-1")})
	processor := Processor{Queue: queue}

	if err := processor.ProcessOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if queue.SucceededCount() != 1 {
		t.Fatalf("succeeded count = %d, want 1", queue.SucceededCount())
	}
}

func TestProcessorReturnsNoJobs(t *testing.T) {
	processor := Processor{Queue: NewMemoryQueue(nil)}

	if err := processor.ProcessOne(context.Background()); err != ErrNoJobs {
		t.Fatalf("err = %v, want ErrNoJobs", err)
	}
}

func TestProcessorAcceptsTranscriptionAndModerationSkeletonJobs(t *testing.T) {
	queue := NewMemoryQueue([]Job{
		New("job-transcript", TypeGenerateTranscript, "asset-1"),
		New("job-scan", TypeScanMediaSafety, "asset-1"),
	})
	processor := Processor{Queue: queue}

	if err := processor.ProcessOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := processor.ProcessOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if queue.SucceededCount() != 2 {
		t.Fatalf("succeeded count = %d, want 2", queue.SucceededCount())
	}
}

func TestQueueEnqueueIsIdempotent(t *testing.T) {
	queue := NewMemoryQueue(nil)
	job := New("job-1", TypeProcessMedia, "asset-1")

	if err := queue.Enqueue(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if err := queue.Enqueue(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if queue.PendingCount() != 1 {
		t.Fatalf("pending count = %d, want 1", queue.PendingCount())
	}
}

func TestQueueEnqueueIsIdempotentAfterSuccess(t *testing.T) {
	queue := NewMemoryQueue(nil)
	job := New("job-1", TypeProcessMedia, "asset-1")
	if err := queue.Enqueue(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	processor := Processor{Queue: queue}
	if err := processor.ProcessOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := queue.Enqueue(context.Background(), New("job-2", TypeProcessMedia, "asset-1")); err != nil {
		t.Fatal(err)
	}
	if queue.PendingCount() != 0 {
		t.Fatalf("pending count = %d, want 0", queue.PendingCount())
	}
}

func TestQueueRetryBackoffAndDeadLetter(t *testing.T) {
	queue := NewMemoryQueue([]Job{{ID: "bad", Type: "bad", AssetID: "asset-1", IdempotencyKey: "bad:asset-1", MaxAttempts: 2}})
	queue.clock = fixedClock{now: time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)}
	processor := Processor{Queue: queue}

	if err := processor.ProcessOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if queue.PendingCount() != 1 || queue.FailedCount() != 0 {
		t.Fatalf("retry state pending=%d failed=%d", queue.PendingCount(), queue.FailedCount())
	}
	if err := processor.ProcessOne(context.Background()); err != ErrNoJobs {
		t.Fatalf("err = %v, want ErrNoJobs while backoff active", err)
	}
	queue.clock = fixedClock{now: time.Date(2026, 5, 31, 12, 0, 2, 0, time.UTC)}
	if err := processor.ProcessOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if queue.FailedCount() != 1 {
		t.Fatalf("failed count = %d, want 1", queue.FailedCount())
	}
}

func TestRedisConfigRequiresExplicitConnectionSettings(t *testing.T) {
	if (RedisConfig{URL: "redis://127.0.0.1:6379", StreamName: "media_jobs", GroupName: "workers", Consumer: "dev"}).Valid() != true {
		t.Fatal("complete redis config should be valid")
	}
	if (RedisConfig{URL: "redis://127.0.0.1:6379", StreamName: "media_jobs"}).Valid() {
		t.Fatal("partial redis config should be invalid")
	}
}

func TestProcessorRetriesHandlerFailure(t *testing.T) {
	queue := NewMemoryQueue([]Job{New("job-1", TypeProcessMedia, "asset-1")})
	processor := Processor{Queue: queue, Handler: failingHandler{}}

	if err := processor.ProcessOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if queue.PendingCount() != 1 || queue.SucceededCount() != 0 {
		t.Fatalf("bad retry state pending=%d succeeded=%d", queue.PendingCount(), queue.SucceededCount())
	}
}
