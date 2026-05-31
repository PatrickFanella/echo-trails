package media

import (
	"context"
	"errors"
	"strings"
)

const (
	TranscriptStatusPending    = "pending"
	TranscriptStatusProcessing = "processing"
	TranscriptStatusSucceeded  = "succeeded"
	TranscriptStatusFailed     = "failed"
	TranscriptStatusSkipped    = "skipped"

	DefaultTranscriptLocale = "und"
)

var ErrTranscriptUnsupported = errors.New("transcript unsupported")

type Transcript struct {
	AssetID       string
	Status        string
	Locale        string
	Text          string
	Confidence    float64
	ModelName     string
	FailureReason string
}

type Transcriber interface {
	Transcribe(context.Context, Asset) (Transcript, error)
}

type TranscriptCatalog interface {
	LoadAsset(context.Context, string) (Asset, error)
	SaveTranscript(context.Context, Transcript) error
}

type TranscriptionProcessor struct {
	Catalog     TranscriptCatalog
	Transcriber Transcriber
	Locale      string
}

type StaticTranscriber struct {
	Text       string
	Confidence float64
	ModelName  string
	Locale     string
	Err        error
}

func (t StaticTranscriber) Transcribe(_ context.Context, asset Asset) (Transcript, error) {
	if t.Err != nil {
		return Transcript{}, t.Err
	}
	locale := strings.TrimSpace(t.Locale)
	if locale == "" {
		locale = DefaultTranscriptLocale
	}
	text := strings.TrimSpace(t.Text)
	if text == "" {
		return Transcript{}, ErrTranscriptUnsupported
	}
	confidence := t.Confidence
	if confidence <= 0 || confidence > 1 {
		confidence = 0.5
	}
	modelName := strings.TrimSpace(t.ModelName)
	if modelName == "" {
		modelName = "static-fixture"
	}
	return Transcript{AssetID: asset.ID, Status: TranscriptStatusSucceeded, Locale: locale, Text: text, Confidence: confidence, ModelName: modelName}, nil
}

func (p TranscriptionProcessor) ProcessAsset(ctx context.Context, assetID string) (Transcript, error) {
	if p.Catalog == nil || p.Transcriber == nil {
		return Transcript{}, ErrProcessorMisconfigured
	}
	asset, err := p.Catalog.LoadAsset(ctx, assetID)
	if err != nil {
		return Transcript{}, err
	}
	if !transcriptionEligible(asset) {
		transcript := Transcript{AssetID: asset.ID, Status: TranscriptStatusSkipped, Locale: p.locale(), FailureReason: "asset is not audio or video"}
		return transcript, p.Catalog.SaveTranscript(ctx, transcript)
	}
	transcript, err := p.Transcriber.Transcribe(ctx, asset)
	if err != nil {
		failed := Transcript{AssetID: asset.ID, Status: TranscriptStatusFailed, Locale: p.locale(), FailureReason: err.Error()}
		return failed, p.Catalog.SaveTranscript(ctx, failed)
	}
	transcript.AssetID = asset.ID
	if strings.TrimSpace(transcript.Locale) == "" {
		transcript.Locale = p.locale()
	}
	if transcript.Status == "" {
		transcript.Status = TranscriptStatusSucceeded
	}
	return transcript, p.Catalog.SaveTranscript(ctx, transcript)
}

func (p TranscriptionProcessor) locale() string {
	if strings.TrimSpace(p.Locale) == "" {
		return DefaultTranscriptLocale
	}
	return strings.TrimSpace(p.Locale)
}

func transcriptionEligible(asset Asset) bool {
	return (asset.MediaType == "audio" || asset.MediaType == "video") && (asset.Status == AssetStatusUploaded || asset.Status == AssetStatusProcessing || asset.Status == AssetStatusPendingReview)
}
