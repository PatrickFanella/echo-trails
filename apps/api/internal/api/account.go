package api

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

var ErrInviteRequired = errors.New("beta invite required")
var ErrInviteInvalid = errors.New("invalid beta invite")
var ErrAccountInvalid = errors.New("invalid account request")

type AccountRepository interface {
	ClaimInvite(context.Context, string, string, string, time.Time) (BetaAccessResponse, error)
	CreationAllowed(context.Context, string) (bool, error)
	AcceptConsent(context.Context, string, ConsentAcceptanceRequest, time.Time) error
	QueuePrivacyJob(context.Context, string, string, time.Time) (PrivacyJobStatus, error)
	GetPrivacyJob(context.Context, string, string) (PrivacyJobStatus, error)
	GetAnalyticsOptOut(context.Context, string) (bool, error)
	SetAnalyticsOptOut(context.Context, string, bool, time.Time) error
	RecordAnalyticsEvent(context.Context, string, AnalyticsEventEnvelope, time.Time) error
	ListAccountSessionPage(context.Context, string, string, int) (AccountSessionPage, error)
	RevokeAccountSession(context.Context, string, string, time.Time) error
}

type AccountAccessState struct {
	BetaAccessAccepted       bool `json:"beta_access_accepted"`
	ConsentAccepted          bool `json:"consent_accepted"`
	ProfileConfigured        bool `json:"profile_configured"`
	QuoteWarningAcknowledged bool `json:"quote_warning_acknowledged"`
}

// accountAccessRepository is implemented by the production repository. It is
// intentionally separate from AccountRepository so unit-test repositories do
// not accidentally claim to enforce the release access gate.
type accountAccessRepository interface {
	GetAccountAccessState(context.Context, string) (AccountAccessState, error)
}

type AccountSession struct {
	ID         string     `json:"id"`
	DeviceName string     `json:"device_name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	ExpiresAt  time.Time  `json:"expires_at"`
}

type AccountSessionPage struct {
	Sessions   []AccountSession `json:"sessions"`
	NextCursor string           `json:"next_cursor,omitempty"`
}

type MemoryAccountRepository struct {
	mu      sync.Mutex
	invited map[string]bool
	jobs    map[string]PrivacyJobStatus
	optOut  map[string]bool
}

func NewMemoryAccountRepository() *MemoryAccountRepository {
	return &MemoryAccountRepository{invited: map[string]bool{}, jobs: map[string]PrivacyJobStatus{}, optOut: map[string]bool{}}
}

func (m *MemoryAccountRepository) ClaimInvite(_ context.Context, userID, code, city string, now time.Time) (BetaAccessResponse, error) {
	if strings.TrimSpace(code) == "" || !strings.EqualFold(strings.TrimSpace(city), "Downers Grove") {
		return BetaAccessResponse{}, ErrInviteInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.invited[userID] = true
	return BetaAccessResponse{State: BetaAccessAccepted, City: "Downers Grove", Message: "Invite accepted."}, nil
}
func (m *MemoryAccountRepository) CreationAllowed(_ context.Context, userID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.invited) == 0 {
		return true, nil
	}
	return m.invited[userID], nil
}
func (m *MemoryAccountRepository) AcceptConsent(context.Context, string, ConsentAcceptanceRequest, time.Time) error {
	return nil
}
func (m *MemoryAccountRepository) QueuePrivacyJob(_ context.Context, userID, kind string, now time.Time) (PrivacyJobStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job := PrivacyJobStatus{JobID: newOpaqueID("privacy"), UserID: userID, Kind: kind, Status: "queued", RequestedAt: now.UTC()}
	m.jobs[job.JobID] = job
	return job, nil
}
func (m *MemoryAccountRepository) GetPrivacyJob(_ context.Context, userID, jobID string) (PrivacyJobStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[jobID]
	if !ok || job.UserID != userID {
		return PrivacyJobStatus{}, ErrEngagementNotFound
	}
	return job, nil
}
func (m *MemoryAccountRepository) GetAnalyticsOptOut(_ context.Context, userID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.optOut[userID], nil
}
func (m *MemoryAccountRepository) SetAnalyticsOptOut(_ context.Context, userID string, value bool, _ time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.optOut[userID] = value
	return nil
}
func (m *MemoryAccountRepository) RecordAnalyticsEvent(_ context.Context, _ string, event AnalyticsEventEnvelope, _ time.Time) error {
	if !event.SafeForCollection() {
		return ErrAccountInvalid
	}
	return nil
}
func (m *MemoryAccountRepository) ListAccountSessionPage(context.Context, string, string, int) (AccountSessionPage, error) {
	return AccountSessionPage{Sessions: []AccountSession{}}, nil
}
func (m *MemoryAccountRepository) RevokeAccountSession(context.Context, string, string, time.Time) error {
	return nil
}
