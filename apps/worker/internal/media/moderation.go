package media

import (
	"context"
	"errors"
	"strings"
)

const (
	ModerationStatusPending     = "pending"
	ModerationStatusScanning    = "scanning"
	ModerationStatusApproved    = "approved"
	ModerationStatusNeedsReview = "needs_review"
	ModerationStatusRejected    = "rejected"
	ModerationStatusFailed      = "failed"

	RiskUnknown = "unknown"
	RiskLow     = "low"
	RiskMedium  = "medium"
	RiskHigh    = "high"
)

var ErrScannerUnavailable = errors.New("safety scanner unavailable")

type ModerationReview struct {
	AssetID       string
	Status        string
	RiskLevel     string
	Categories    map[string]bool
	ScannerName   string
	FailureReason string
}

type SafetyScanner interface {
	Scan(context.Context, Asset) (ModerationReview, error)
}

type ModerationCatalog interface {
	LoadAsset(context.Context, string) (Asset, error)
	SaveModerationReview(context.Context, ModerationReview) error
}

type ModerationProcessor struct {
	Catalog ModerationCatalog
	Scanner SafetyScanner
}

type StaticSafetyScanner struct {
	RiskLevel   string
	Categories  map[string]bool
	ScannerName string
	Err         error
}

func (s StaticSafetyScanner) Scan(_ context.Context, asset Asset) (ModerationReview, error) {
	if s.Err != nil {
		return ModerationReview{}, s.Err
	}
	risk := normalizeRisk(s.RiskLevel)
	status := ModerationStatusNeedsReview
	if risk == RiskLow {
		status = ModerationStatusApproved
	}
	if risk == RiskHigh {
		status = ModerationStatusRejected
	}
	scannerName := strings.TrimSpace(s.ScannerName)
	if scannerName == "" {
		scannerName = "static-fixture"
	}
	return ModerationReview{AssetID: asset.ID, Status: status, RiskLevel: risk, Categories: cloneCategories(s.Categories), ScannerName: scannerName}, nil
}

func (s StaticSafetyScanner) ScanText(_ context.Context, content EchoText) (EchoModerationReview, error) {
	if s.Err != nil {
		return EchoModerationReview{}, s.Err
	}
	risk := normalizeRisk(s.RiskLevel)
	return EchoModerationReview{EchoID: content.EchoID, Status: conservativeStatus(ModerationStatusApproved, risk), RiskLevel: risk, Categories: cloneCategories(s.Categories), ScannerName: strings.TrimSpace(s.ScannerName)}, nil
}

func (p ModerationProcessor) ProcessAsset(ctx context.Context, assetID string) (ModerationReview, error) {
	if p.Catalog == nil || p.Scanner == nil {
		return ModerationReview{}, ErrProcessorMisconfigured
	}
	asset, err := p.Catalog.LoadAsset(ctx, assetID)
	if err != nil {
		return ModerationReview{}, err
	}
	review, err := p.Scanner.Scan(ctx, asset)
	if err != nil {
		failed := ModerationReview{AssetID: asset.ID, Status: ModerationStatusNeedsReview, RiskLevel: RiskUnknown, ScannerName: "scanner-unavailable", FailureReason: err.Error()}
		return failed, p.Catalog.SaveModerationReview(ctx, failed)
	}
	review.AssetID = asset.ID
	review.RiskLevel = normalizeRisk(review.RiskLevel)
	review.Status = conservativeStatus(review.Status, review.RiskLevel)
	return review, p.Catalog.SaveModerationReview(ctx, review)
}

func conservativeStatus(status, risk string) string {
	switch risk {
	case RiskLow:
		if status == ModerationStatusApproved {
			return ModerationStatusApproved
		}
		return ModerationStatusNeedsReview
	case RiskHigh:
		return ModerationStatusRejected
	case RiskMedium, RiskUnknown:
		return ModerationStatusNeedsReview
	default:
		return ModerationStatusNeedsReview
	}
}

func normalizeRisk(risk string) string {
	switch strings.ToLower(strings.TrimSpace(risk)) {
	case RiskLow, RiskMedium, RiskHigh:
		return strings.TrimSpace(risk)
	default:
		return RiskUnknown
	}
}

func cloneCategories(categories map[string]bool) map[string]bool {
	if len(categories) == 0 {
		return nil
	}
	clone := map[string]bool{}
	for key, value := range categories {
		if strings.TrimSpace(key) == "" {
			continue
		}
		clone[key] = value
	}
	return clone
}
