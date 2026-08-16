package media

import (
	"context"
	"strings"
)

type EchoText struct {
	EchoID  string `json:"echo_id"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	Caption string `json:"caption"`
}

type EchoModerationReview struct {
	EchoID        string
	Status        string
	RiskLevel     string
	Categories    map[string]bool
	ScannerName   string
	FailureReason string
}

type TextSafetyScanner interface {
	ScanText(context.Context, EchoText) (EchoModerationReview, error)
}

type TextModerationCatalog interface {
	LoadEchoText(context.Context, string) (EchoText, error)
	SaveEchoModerationReview(context.Context, EchoModerationReview) error
}

type TextModerationProcessor struct {
	Catalog TextModerationCatalog
	Scanner TextSafetyScanner
}

func (p TextModerationProcessor) ProcessEcho(ctx context.Context, echoID string) (EchoModerationReview, error) {
	if p.Catalog == nil || p.Scanner == nil {
		return EchoModerationReview{}, ErrProcessorMisconfigured
	}
	content, err := p.Catalog.LoadEchoText(ctx, echoID)
	if err != nil {
		return EchoModerationReview{}, err
	}
	review, err := p.Scanner.ScanText(ctx, content)
	if err != nil {
		review = EchoModerationReview{EchoID: echoID, Status: ModerationStatusNeedsReview, RiskLevel: RiskUnknown, ScannerName: "scanner-unavailable", FailureReason: err.Error()}
	} else {
		review.EchoID = echoID
		review.RiskLevel = normalizeRisk(review.RiskLevel)
		review.Status = conservativeStatus(review.Status, review.RiskLevel)
		if strings.TrimSpace(review.ScannerName) == "" {
			review.ScannerName = "unknown-scanner"
		}
	}
	return review, p.Catalog.SaveEchoModerationReview(ctx, review)
}
