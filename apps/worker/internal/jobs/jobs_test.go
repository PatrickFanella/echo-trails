package jobs

import (
	"context"
	"testing"
)

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
