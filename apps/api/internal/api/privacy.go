package api

import "time"

type ConsentVersion struct {
	TermsVersion   string    `json:"terms_version"`
	PrivacyVersion string    `json:"privacy_version"`
	Disclosure     string    `json:"disclosure"`
	EffectiveAt    time.Time `json:"effective_at"`
}

type ConsentAcceptanceRequest struct {
	TermsVersion   string `json:"terms_version"`
	PrivacyVersion string `json:"privacy_version"`
}

type PrivacyExportRequest struct{}

type PrivacyJobStatus struct {
	JobID       string    `json:"job_id"`
	UserID      string    `json:"user_id"`
	Kind        string    `json:"kind"`
	Status      string    `json:"status"`
	RequestedAt time.Time `json:"requested_at"`
	DownloadURL string    `json:"download_url,omitempty"`
	ObjectKey   string    `json:"-"`
}

type AccountDeletionRequest struct {
	Reason string `json:"reason,omitempty"`
}

type AnalyticsOptOutRequest struct {
	OptOut bool `json:"opt_out"`
}

type AnalyticsEventEnvelope struct {
	UserID           string            `json:"user_id,omitempty"`
	EventName        string            `json:"event_name"`
	PlaceID          string            `json:"place_id,omitempty"`
	Properties       map[string]string `json:"properties,omitempty"`
	AllowsExactCoord bool              `json:"allows_exact_coordinates"`
}

func AnalyticsEventAllowsKey(key string) bool {
	allowed := map[string]bool{"user_id": true, "event_name": true, "place_id": true, "properties": true, "allows_exact_coordinates": true}
	return allowed[key]
}

func CurrentConsentVersion() ConsentVersion {
	return ConsentVersion{TermsVersion: "2026-05-31-beta", PrivacyVersion: "2026-05-31-beta", Disclosure: "Self-hosted beta template; legal review required before public launch.", EffectiveAt: time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)}
}

func NewPrivacyExportJob(userID string, now time.Time) PrivacyJobStatus {
	return PrivacyJobStatus{JobID: "privacy-export-" + userID, UserID: userID, Kind: "export", Status: "queued", RequestedAt: now.UTC()}
}

func NewAccountDeletionJob(userID string, now time.Time) PrivacyJobStatus {
	return PrivacyJobStatus{JobID: "account-delete-" + userID, UserID: userID, Kind: "delete", Status: "queued", RequestedAt: now.UTC()}
}

func (e AnalyticsEventEnvelope) SafeForCollection() bool {
	if e.EventName == "" || e.AllowsExactCoord {
		return false
	}
	allowed := map[string]bool{"surface": true, "action": true, "result": true, "place_id": true}
	for key := range e.Properties {
		if !allowed[key] {
			return false
		}
	}
	return true
}
