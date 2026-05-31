package media

import (
	"context"
	"testing"

	"git.subcult.tv/subculture-collective/echo-trails/apps/worker/internal/jobs"
)

func TestModerationApprovesOnlyExplicitLowRisk(t *testing.T) {
	catalog := NewMemoryCatalog([]Asset{{ID: "asset-safe", MediaType: "image", ContentType: "image/png", ObjectKey: "media/original/asset-safe/original.png", Status: AssetStatusPendingReview}})
	processor := ModerationProcessor{Catalog: catalog, Scanner: StaticSafetyScanner{RiskLevel: RiskLow, ScannerName: "fixture"}}

	review, err := processor.ProcessAsset(context.Background(), "asset-safe")
	if err != nil {
		t.Fatal(err)
	}
	if review.Status != ModerationStatusApproved || review.RiskLevel != RiskLow {
		t.Fatalf("bad safe review: %+v", review)
	}
}

func TestModerationConservativelyHandlesUncertainAndRiskyAssets(t *testing.T) {
	for _, tc := range []struct {
		name       string
		risk       string
		wantStatus string
	}{
		{name: "unknown", risk: RiskUnknown, wantStatus: ModerationStatusNeedsReview},
		{name: "medium", risk: RiskMedium, wantStatus: ModerationStatusNeedsReview},
		{name: "high", risk: RiskHigh, wantStatus: ModerationStatusRejected},
	} {
		t.Run(tc.name, func(t *testing.T) {
			catalog := NewMemoryCatalog([]Asset{{ID: "asset-risk", MediaType: "video", ContentType: "video/mp4", ObjectKey: "media/original/asset-risk/original.mp4", Status: AssetStatusPendingReview}})
			processor := ModerationProcessor{Catalog: catalog, Scanner: StaticSafetyScanner{RiskLevel: tc.risk}}
			review, err := processor.ProcessAsset(context.Background(), "asset-risk")
			if err != nil {
				t.Fatal(err)
			}
			if review.Status != tc.wantStatus {
				t.Fatalf("status = %s, want %s", review.Status, tc.wantStatus)
			}
		})
	}
}

func TestModerationScannerFailureNeedsReview(t *testing.T) {
	catalog := NewMemoryCatalog([]Asset{{ID: "asset-fail", MediaType: "audio", ContentType: "audio/mpeg", ObjectKey: "media/original/asset-fail/original.mp3", Status: AssetStatusPendingReview}})
	processor := ModerationProcessor{Catalog: catalog, Scanner: StaticSafetyScanner{Err: ErrScannerUnavailable}}

	review, err := processor.ProcessAsset(context.Background(), "asset-fail")
	if err != nil {
		t.Fatal(err)
	}
	if review.Status != ModerationStatusNeedsReview || review.RiskLevel != RiskUnknown || review.FailureReason == "" {
		t.Fatalf("bad scanner failure review: %+v", review)
	}
}

func TestJobHandlerRunsModerationJob(t *testing.T) {
	catalog := NewMemoryCatalog([]Asset{{ID: "asset-safe", MediaType: "image", ContentType: "image/jpeg", ObjectKey: "media/original/asset-safe/original.jpg", Status: AssetStatusPendingReview}})
	handler := JobHandler{Moderation: ModerationProcessor{Catalog: catalog, Scanner: StaticSafetyScanner{RiskLevel: RiskLow}}}

	if err := handler.HandleJob(context.Background(), jobs.New("job-scan", jobs.TypeScanMediaSafety, "asset-safe")); err != nil {
		t.Fatal(err)
	}
	if catalog.ModerationReviews["asset-safe"].Status != ModerationStatusApproved {
		t.Fatalf("moderation job did not save review: %+v", catalog.ModerationReviews)
	}
}
