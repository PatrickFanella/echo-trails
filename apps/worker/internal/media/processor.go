package media

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	ID          string
	MediaType   string
	ContentType string
	ObjectKey   string
	Status      string
	Metadata    map[string]string
}

type Derivative struct {
	MediaAssetID   string
	DerivativeType string
	ObjectKey      string
	ContentType    string
	MetadataSafe   bool
}

type Result struct {
	AssetID          string
	Status           string
	MetadataStripped bool
	Derivatives      []Derivative
}

type Catalog interface {
	LoadAsset(context.Context, string) (Asset, error)
	SaveResult(context.Context, Result) error
}

type Processor struct {
	Catalog Catalog
}

type JobHandler struct {
	Processor     Processor
	Transcription TranscriptionProcessor
	Moderation    ModerationProcessor
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
	result, err := BuildResult(asset)
	if err != nil {
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
	if asset.Status != AssetStatusUploaded && asset.Status != AssetStatusProcessing {
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
}

func NewMemoryCatalog(assets []Asset) *MemoryCatalog {
	catalog := &MemoryCatalog{Assets: map[string]Asset{}, Results: map[string]Result{}, Transcripts: map[string]Transcript{}, ModerationReviews: map[string]ModerationReview{}}
	for _, asset := range assets {
		catalog.Assets[asset.ID] = asset
	}
	return catalog
}

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
