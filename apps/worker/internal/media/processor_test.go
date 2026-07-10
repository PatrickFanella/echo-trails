package media

import (
	"context"
	"errors"
	"testing"

	"git.subcult.tv/subculture-collective/echo-trails/apps/worker/internal/jobs"
)

func TestBuildResultStripsMetadataAndCreatesImageDerivatives(t *testing.T) {
	result, err := BuildResult(Asset{ID: "asset-image", MediaType: "image", ContentType: "image/jpeg", ObjectKey: "media/original/asset-image/original.jpg", Status: AssetStatusUploaded, Metadata: map[string]string{"gps": "37,-122"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != AssetStatusPendingReview || !result.MetadataStripped {
		t.Fatalf("bad result status/metadata: %+v", result)
	}
	if len(result.Derivatives) != 2 || result.Derivatives[0].DerivativeType != DerivativeThumbnail || result.Derivatives[1].DerivativeType != DerivativePreview {
		t.Fatalf("bad derivatives: %+v", result.Derivatives)
	}
	for _, derivative := range result.Derivatives {
		if !derivative.MetadataSafe || derivative.MediaAssetID != "asset-image" {
			t.Fatalf("unsafe derivative: %+v", derivative)
		}
	}
}

func TestBuildResultCreatesAudioWaveformOnly(t *testing.T) {
	result, err := BuildResult(Asset{ID: "asset-audio", MediaType: "audio", ContentType: "audio/mpeg", ObjectKey: "media/original/asset-audio/original.mp3", Status: AssetStatusUploaded})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Derivatives) != 1 || result.Derivatives[0].DerivativeType != DerivativeWaveform {
		t.Fatalf("bad audio derivatives: %+v", result.Derivatives)
	}
}

func TestBuildResultCreatesVideoPreviewThumbnailAndWaveform(t *testing.T) {
	result, err := BuildResult(Asset{ID: "asset-video", MediaType: "video", ContentType: "video/mp4", ObjectKey: "media/original/asset-video/original.mp4", Status: AssetStatusProcessing})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Derivatives) != 3 {
		t.Fatalf("bad video derivatives: %+v", result.Derivatives)
	}
}

func TestBuildResultRejectsUnsupportedOrUnuploadedAsset(t *testing.T) {
	for _, asset := range []Asset{
		{ID: "asset-text", MediaType: "text", ContentType: "text/plain", ObjectKey: "media/original/asset-text/original", Status: AssetStatusUploaded},
		{ID: "asset-image", MediaType: "image", ContentType: "image/jpeg", ObjectKey: "media/original/asset-image/original.jpg", Status: "pending_upload"},
		{ID: "../asset-image", MediaType: "image", ContentType: "image/jpeg", ObjectKey: "media/original/asset-image/original.jpg", Status: AssetStatusUploaded},
		{ID: "asset-image", MediaType: "image", ContentType: "video/mp4", ObjectKey: "media/original/asset-image/original.jpg", Status: AssetStatusUploaded},
		{ID: "asset-audio", MediaType: "audio", ContentType: "image/png", ObjectKey: "media/original/asset-audio/original.mp3", Status: AssetStatusUploaded},
		{ID: "asset-video", MediaType: "video", ContentType: "audio/wav", ObjectKey: "media/original/asset-video/original.mp4", Status: AssetStatusUploaded},
	} {
		_, err := BuildResult(asset)
		if !errors.Is(err, ErrUnsupportedMedia) {
			t.Fatalf("err = %v, want ErrUnsupportedMedia", err)
		}
	}
}

func TestProcessorReturnsMisconfiguredErrorWhenCatalogMissing(t *testing.T) {
	processor := Processor{}
	_, err := processor.ProcessAsset(context.Background(), "asset-image")
	if !errors.Is(err, ErrProcessorMisconfigured) {
		t.Fatalf("err = %v, want ErrProcessorMisconfigured", err)
	}
}

func TestProcessorPersistsResultAndClearsMetadata(t *testing.T) {
	catalog := NewMemoryCatalog([]Asset{{ID: "asset-image", MediaType: "image", ContentType: "image/jpeg", ObjectKey: "media/original/asset-image/original.jpg", Status: AssetStatusUploaded, Metadata: map[string]string{"camera": "phone"}}})
	processor := Processor{Catalog: catalog}

	result, err := processor.ProcessAsset(context.Background(), "asset-image")
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Results["asset-image"].Status != AssetStatusPendingReview || result.Status != AssetStatusPendingReview {
		t.Fatalf("result not persisted: %+v", catalog.Results)
	}
	if catalog.Assets["asset-image"].Metadata != nil {
		t.Fatalf("metadata was not cleared: %+v", catalog.Assets["asset-image"].Metadata)
	}
}

type failingPipeline struct{ err error }

func (p failingPipeline) Process(context.Context, Asset) (Result, error) { return Result{}, p.err }

type failureRecordingCatalog struct {
	*MemoryCatalog
	assetID string
	failure error
}

func (c *failureRecordingCatalog) SaveProcessingFailure(_ context.Context, assetID string, err error) error {
	c.assetID, c.failure = assetID, err
	return nil
}

func TestProcessorPersistsPrivateReviewHoldOnPipelineFailure(t *testing.T) {
	failure := errors.New("malware scanner unavailable")
	catalog := &failureRecordingCatalog{MemoryCatalog: NewMemoryCatalog([]Asset{{ID: "asset-image", MediaType: "image", ContentType: "image/jpeg", ObjectKey: "media/original/asset-image/original.jpg", Status: AssetStatusUploaded}})}
	_, err := (Processor{Catalog: catalog, Pipeline: failingPipeline{err: failure}}).ProcessAsset(context.Background(), "asset-image")
	if !errors.Is(err, failure) || catalog.assetID != "asset-image" || !errors.Is(catalog.failure, failure) {
		t.Fatalf("failure hold not persisted: err=%v asset=%q saved=%v", err, catalog.assetID, catalog.failure)
	}
}

func TestBuildResultAllowsFailClosedRetry(t *testing.T) {
	_, err := BuildResult(Asset{ID: "asset-image", MediaType: "image", ContentType: "image/jpeg", ObjectKey: "media/original/asset-image/original.jpg", Status: AssetStatusPendingReview})
	if err != nil {
		t.Fatalf("pending-review retry rejected: %v", err)
	}
}

func TestJobHandlerProcessesMediaJobs(t *testing.T) {
	catalog := NewMemoryCatalog([]Asset{{ID: "asset-image", MediaType: "image", ContentType: "image/jpeg", ObjectKey: "media/original/asset-image/original.jpg", Status: AssetStatusUploaded}})
	handler := JobHandler{Processor: Processor{Catalog: catalog}}

	if err := handler.HandleJob(context.Background(), jobs.New("job-2", jobs.TypeProcessMedia, "asset-image")); err != nil {
		t.Fatal(err)
	}
	if catalog.Results["asset-image"].Status != AssetStatusPendingReview {
		t.Fatalf("media job was not processed: %+v", catalog.Results)
	}
}
