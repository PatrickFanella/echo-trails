package api

import (
	"testing"
	"time"
)

func TestConsentAndPrivacyJobsUseVersionedBetaContracts(t *testing.T) {
	version := CurrentConsentVersion()
	if version.TermsVersion == "" || version.PrivacyVersion == "" || version.Disclosure == "" {
		t.Fatalf("bad consent version: %+v", version)
	}
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	if job := NewPrivacyExportJob("user-1", now); job.Kind != "export" || job.Status != "queued" {
		t.Fatalf("bad export job: %+v", job)
	}
	if job := NewAccountDeletionJob("user-1", now); job.Kind != "delete" || job.Status != "queued" {
		t.Fatalf("bad deletion job: %+v", job)
	}
}

func TestAnalyticsEnvelopeRejectsExactCoordinates(t *testing.T) {
	safe := AnalyticsEventEnvelope{EventName: "place_view", PlaceID: "place-active-1"}
	unsafe := AnalyticsEventEnvelope{EventName: "place_view", AllowsExactCoord: true}
	smuggled := AnalyticsEventEnvelope{EventName: "place_view", Properties: map[string]string{"lat": "37.7"}}
	if !safe.SafeForCollection() {
		t.Fatal("expected safe event")
	}
	if unsafe.SafeForCollection() {
		t.Fatal("exact coordinate event should be rejected")
	}
	if smuggled.SafeForCollection() {
		t.Fatal("coordinate-like properties should be rejected")
	}
	if AnalyticsEventAllowsKey("lat") || AnalyticsEventAllowsKey("longitude") {
		t.Fatal("top-level coordinate keys should not be accepted")
	}
}
