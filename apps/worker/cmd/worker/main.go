package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"git.subcult.tv/subculture-collective/echo-trails/apps/worker/internal/jobs"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	queue := jobs.NewMemoryQueue([]jobs.Job{
		jobs.New("dev-process-media", jobs.TypeProcessMedia, "33333333-3333-4333-8333-333333333333"),
		jobs.New("dev-generate-transcript", jobs.TypeGenerateTranscript, "33333333-3333-4333-8333-333333333333"),
		jobs.New("dev-scan-media-safety", jobs.TypeScanMediaSafety, "33333333-3333-4333-8333-333333333333"),
	})
	processor := jobs.Processor{Queue: queue, Logger: logger}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for {
		if err := processor.ProcessOne(ctx); err != nil {
			if err == jobs.ErrNoJobs {
				logger.Info("worker drained dev queue")
				return
			}
			logger.Error("worker process-one failed", "error", err)
			os.Exit(1)
		}
	}
}
