package media

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"git.subcult.tv/subculture-collective/echo-trails/apps/worker/internal/jobs"
)

const (
	AssetStatusUploaded      = "uploaded"
	AssetStatusProcessing    = "processing"
	AssetStatusPendingReview = "pending_review"
	AssetStatusFailed        = "failed"

	DerivativeThumbnail = "thumbnail"
	DerivativePreview   = "preview"
	DerivativeWaveform  = "waveform"
)

var ErrUnsupportedMedia = errors.New("unsupported media asset")
var ErrProcessorMisconfigured = errors.New("media processor misconfigured")

type Asset struct {
	ID            string
	StorageBucket string
	MediaType     string
	ContentType   string
	ObjectKey     string
	Status        string
	ByteSize      int64
	Metadata      map[string]string
}

type Derivative struct {
	MediaAssetID   string
	DerivativeType string
	ObjectKey      string
	ContentType    string
	MetadataSafe   bool
	Data           []byte
}

type Result struct {
	AssetID             string
	Status              string
	MetadataStripped    bool
	Derivatives         []Derivative
	SHA256              []byte
	DurationMS          int
	Width               int
	Height              int
	VerifiedContentType string
	MalwareScanStatus   string
}

type Catalog interface {
	LoadAsset(context.Context, string) (Asset, error)
	SaveResult(context.Context, Result) error
}

type ProcessingFailureCatalog interface {
	SaveProcessingFailure(context.Context, string, error) error
}

type Processor struct {
	Catalog  Catalog
	Pipeline ProcessingPipeline
}

type ProcessingPipeline interface {
	Process(context.Context, Asset) (Result, error)
}

type JobHandler struct {
	Processor      Processor
	Transcription  TranscriptionProcessor
	Moderation     ModerationProcessor
	Snapshots      QuoteSnapshotProcessor
	TextModeration TextModerationProcessor
}

func (h JobHandler) HandleJob(ctx context.Context, job jobs.Job) error {
	if job.Type != jobs.TypeProcessMedia {
		if job.Type == jobs.TypeGenerateTranscript {
			_, err := h.Transcription.ProcessAsset(ctx, job.AssetID)
			return err
		}
		if job.Type == jobs.TypeScanMediaSafety {
			_, err := h.Moderation.ProcessAsset(ctx, job.AssetID)
			return err
		}
		if job.Type == jobs.TypeRenderReplySnapshot {
			_, err := h.Snapshots.Process(ctx, job.AssetID)
			return err
		}
		if job.Type == jobs.TypeScanEchoSafety {
			_, err := h.TextModeration.ProcessEcho(ctx, job.AssetID)
			return err
		}
		return nil
	}
	_, err := h.Processor.ProcessAsset(ctx, job.AssetID)
	return err
}

func (p Processor) ProcessAsset(ctx context.Context, assetID string) (Result, error) {
	if p.Catalog == nil {
		return Result{}, ErrProcessorMisconfigured
	}
	asset, err := p.Catalog.LoadAsset(ctx, assetID)
	if err != nil {
		return Result{}, err
	}
	var result Result
	if p.Pipeline != nil {
		result, err = p.Pipeline.Process(ctx, asset)
	} else {
		result, err = BuildResult(asset)
	}
	if err != nil {
		if failures, ok := p.Catalog.(ProcessingFailureCatalog); ok {
			if saveErr := failures.SaveProcessingFailure(ctx, asset.ID, err); saveErr != nil {
				return Result{}, fmt.Errorf("process media: %v; save failure hold: %w", err, saveErr)
			}
		}
		return Result{}, err
	}
	if err := p.Catalog.SaveResult(ctx, result); err != nil {
		return Result{}, err
	}
	return result, nil
}

func BuildResult(asset Asset) (Result, error) {
	if !safeAssetID(asset.ID) || strings.TrimSpace(asset.ObjectKey) == "" {
		return Result{}, ErrUnsupportedMedia
	}
	if asset.Status != AssetStatusUploaded && asset.Status != AssetStatusProcessing && asset.Status != AssetStatusPendingReview {
		return Result{}, ErrUnsupportedMedia
	}
	derivatives, err := derivativePlan(asset)
	if err != nil {
		return Result{}, err
	}
	return Result{AssetID: asset.ID, Status: AssetStatusPendingReview, MetadataStripped: true, Derivatives: derivatives}, nil
}

func derivativePlan(asset Asset) ([]Derivative, error) {
	switch asset.MediaType {
	case "image":
		if asset.ContentType != "image/jpeg" && asset.ContentType != "image/png" && asset.ContentType != "image/heic" {
			return nil, ErrUnsupportedMedia
		}
		return []Derivative{
			newDerivative(asset, DerivativeThumbnail, "image/jpeg"),
			newDerivative(asset, DerivativePreview, "image/jpeg"),
		}, nil
	case "audio":
		if asset.ContentType != "audio/mpeg" && asset.ContentType != "audio/mp4" && asset.ContentType != "audio/wav" {
			return nil, ErrUnsupportedMedia
		}
		return []Derivative{newDerivative(asset, DerivativeWaveform, "application/json")}, nil
	case "video":
		if asset.ContentType != "video/mp4" && asset.ContentType != "video/quicktime" {
			return nil, ErrUnsupportedMedia
		}
		return []Derivative{
			newDerivative(asset, DerivativeThumbnail, "image/jpeg"),
			newDerivative(asset, DerivativePreview, "video/mp4"),
			newDerivative(asset, DerivativeWaveform, "application/json"),
		}, nil
	default:
		return nil, ErrUnsupportedMedia
	}
}

func safeAssetID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" || strings.Contains(id, "/") || strings.Contains(id, "..") {
		return false
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func newDerivative(asset Asset, kind, contentType string) Derivative {
	return Derivative{
		MediaAssetID:   asset.ID,
		DerivativeType: kind,
		ObjectKey:      fmt.Sprintf("media/derivatives/%s/%s", asset.ID, kind),
		ContentType:    contentType,
		MetadataSafe:   true,
	}
}

type MemoryCatalog struct {
	Assets            map[string]Asset
	Results           map[string]Result
	Transcripts       map[string]Transcript
	ModerationReviews map[string]ModerationReview
	QuoteSnapshots    map[string]QuoteSnapshot
	RenderedSnapshots map[string]RenderedQuoteSnapshot
	EchoTexts         map[string]EchoText
	EchoReviews       map[string]EchoModerationReview
}

func NewMemoryCatalog(assets []Asset) *MemoryCatalog {
	catalog := &MemoryCatalog{Assets: map[string]Asset{}, Results: map[string]Result{}, Transcripts: map[string]Transcript{}, ModerationReviews: map[string]ModerationReview{}, QuoteSnapshots: map[string]QuoteSnapshot{}, RenderedSnapshots: map[string]RenderedQuoteSnapshot{}, EchoTexts: map[string]EchoText{}, EchoReviews: map[string]EchoModerationReview{}}
	for _, asset := range assets {
		catalog.Assets[asset.ID] = asset
	}
	return catalog
}

func (c *MemoryCatalog) LoadEchoText(_ context.Context, echoID string) (EchoText, error) {
	content, ok := c.EchoTexts[echoID]
	if !ok {
		return EchoText{}, ErrUnsupportedMedia
	}
	return content, nil
}

func (c *MemoryCatalog) SaveEchoModerationReview(_ context.Context, review EchoModerationReview) error {
	c.EchoReviews[review.EchoID] = review
	return nil
}

func (c *MemoryCatalog) CleanupExpiredOriginals(context.Context, time.Time, int) (int, error) {
	return 0, nil
}

func (c *MemoryCatalog) LoadQuoteSnapshot(_ context.Context, linkID string) (QuoteSnapshot, error) {
	snapshot, ok := c.QuoteSnapshots[linkID]
	if !ok {
		return QuoteSnapshot{}, ErrUnsupportedMedia
	}
	return snapshot, nil
}

func (c *MemoryCatalog) SaveQuoteSnapshot(_ context.Context, rendered RenderedQuoteSnapshot) error {
	c.RenderedSnapshots[rendered.LinkID] = rendered
	return nil
}

func (c *MemoryCatalog) FailQuoteSnapshot(_ context.Context, _ string, _ error) error { return nil }

func (c *MemoryCatalog) SaveModerationReview(_ context.Context, review ModerationReview) error {
	if c.ModerationReviews == nil {
		c.ModerationReviews = map[string]ModerationReview{}
	}
	c.ModerationReviews[review.AssetID] = review
	return nil
}

func (c *MemoryCatalog) SaveTranscript(_ context.Context, transcript Transcript) error {
	if c.Transcripts == nil {
		c.Transcripts = map[string]Transcript{}
	}
	c.Transcripts[transcript.AssetID] = transcript
	return nil
}

func (c *MemoryCatalog) LoadAsset(_ context.Context, assetID string) (Asset, error) {
	asset, ok := c.Assets[assetID]
	if !ok {
		return Asset{}, ErrUnsupportedMedia
	}
	return asset, nil
}

func (c *MemoryCatalog) SaveResult(_ context.Context, result Result) error {
	if c.Results == nil {
		c.Results = map[string]Result{}
	}
	c.Results[result.AssetID] = result
	asset := c.Assets[result.AssetID]
	asset.Status = result.Status
	asset.Metadata = nil
	c.Assets[result.AssetID] = asset
	return nil
}
