package media

import (
	"context"
	"errors"
	"testing"

	"git.subcult.tv/subculture-collective/echo-trails/apps/worker/internal/jobs"
)

func TestTranscriptionProcessorSucceedsForAudio(t *testing.T) {
	catalog := NewMemoryCatalog([]Asset{{ID: "asset-audio", MediaType: "audio", ContentType: "audio/mpeg", ObjectKey: "media/original/asset-audio/original.mp3", Status: AssetStatusPendingReview}})
	processor := TranscriptionProcessor{Catalog: catalog, Transcriber: StaticTranscriber{Text: "storm steps story", Confidence: 0.91, ModelName: "fixture", Locale: "en"}}

	transcript, err := processor.ProcessAsset(context.Background(), "asset-audio")
	if err != nil {
		t.Fatal(err)
	}
	if transcript.Status != TranscriptStatusSucceeded || transcript.Text != "storm steps story" || transcript.Locale != "en" {
		t.Fatalf("bad transcript: %+v", transcript)
	}
	if catalog.Transcripts["asset-audio"].Text != "storm steps story" {
		t.Fatalf("transcript not saved: %+v", catalog.Transcripts)
	}
}

func TestTranscriptionProcessorSkipsImages(t *testing.T) {
	catalog := NewMemoryCatalog([]Asset{{ID: "asset-image", MediaType: "image", ContentType: "image/jpeg", ObjectKey: "media/original/asset-image/original.jpg", Status: AssetStatusPendingReview}})
	processor := TranscriptionProcessor{Catalog: catalog, Transcriber: StaticTranscriber{Text: "unused"}}

	transcript, err := processor.ProcessAsset(context.Background(), "asset-image")
	if err != nil {
		t.Fatal(err)
	}
	if transcript.Status != TranscriptStatusSkipped || transcript.FailureReason == "" {
		t.Fatalf("bad skipped transcript: %+v", transcript)
	}
}

func TestTranscriptionProcessorStoresFailure(t *testing.T) {
	catalog := NewMemoryCatalog([]Asset{{ID: "asset-video", MediaType: "video", ContentType: "video/mp4", ObjectKey: "media/original/asset-video/original.mp4", Status: AssetStatusProcessing}})
	processor := TranscriptionProcessor{Catalog: catalog, Transcriber: StaticTranscriber{Err: errors.New("fixture unavailable")}}

	transcript, err := processor.ProcessAsset(context.Background(), "asset-video")
	if err != nil {
		t.Fatal(err)
	}
	if transcript.Status != TranscriptStatusFailed || transcript.FailureReason != "fixture unavailable" {
		t.Fatalf("bad failed transcript: %+v", transcript)
	}
}

func TestJobHandlerRunsTranscriptionJob(t *testing.T) {
	catalog := NewMemoryCatalog([]Asset{{ID: "asset-audio", MediaType: "audio", ContentType: "audio/mpeg", ObjectKey: "media/original/asset-audio/original.mp3", Status: AssetStatusPendingReview}})
	handler := JobHandler{Transcription: TranscriptionProcessor{Catalog: catalog, Transcriber: StaticTranscriber{Text: "caption"}}}

	if err := handler.HandleJob(context.Background(), jobs.New("job-transcript", jobs.TypeGenerateTranscript, "asset-audio")); err != nil {
		t.Fatal(err)
	}
	if catalog.Transcripts["asset-audio"].Text != "caption" {
		t.Fatalf("transcription job did not save transcript: %+v", catalog.Transcripts)
	}
}
