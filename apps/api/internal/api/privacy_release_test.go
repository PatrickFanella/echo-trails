package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestPrivacyReleaseGatePublicJSONLogsAndAnalytics recursively inspects values
// emitted by real handlers and serializers. Public Place coordinates are
// intentionally allowed; the private creation-proof values and secret-bearing
// fields are not.
func TestPrivacyReleaseGatePublicJSONLogsAndAnalytics(t *testing.T) {
	const exactLat = 41.797423456
	const exactLon = -88.015129876
	now := time.Date(2026, 7, 9, 18, 0, 0, 0, time.UTC)
	handler := NewHandlerWithConfig(NewStore(DefaultPlaces()), HandlerConfig{
		Clock: nowClock{now: now}, CreationSigningKey: []byte("privacy-release-signing-key"), PublicLocationKey: []byte("privacy-release-jitter-key"),
	})

	fixtures := map[string]*httptest.ResponseRecorder{}
	fixtures["place search"] = doJSON(t, handler, http.MethodGet, "/places/search?q=pond", ``)
	fixtures["nearby discovery"] = doJSON(t, handler, http.MethodGet, "/discovery/nearby?lat=41.7974&lon=-88.0151&radius_meters=500", ``)
	proof := fmt.Sprintf(`{"place_id":%q,"latitude":%.9f,"longitude":%.9f,"accuracy_meters":12,"captured_at":"2026-07-09T17:59:30Z"}`, PlacePrincePondID, exactLat, exactLon)
	fixtures["creation proof"] = doJSON(t, handler, http.MethodPost, "/creation-tokens", proof, authHeaders("privacy-release-user", "privacy-release@example.invalid"))

	for name, response := range fixtures {
		t.Run(name, func(t *testing.T) {
			if response.Code < 200 || response.Code >= 300 {
				t.Fatalf("fixture status=%d body=%s", response.Code, response.Body.String())
			}
			var value any
			if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
				t.Fatalf("decode fixture: %v", err)
			}
			assertPrivacySafeTree(t, "$", value, exactLat, exactLon)
		})
	}

	event := AnalyticsEventEnvelope{EventName: "place_view", PlaceID: PlacePrincePondID, Properties: map[string]string{"surface": "map", "action": "open"}}
	if !event.SafeForCollection() {
		t.Fatal("representative analytics fixture was rejected")
	}
	encodedEvent, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	var analyticsFixture any
	if err = json.Unmarshal(encodedEvent, &analyticsFixture); err != nil {
		t.Fatal(err)
	}
	assertPrivacySafeTree(t, "$analytics", analyticsFixture, exactLat, exactLon)

	var logOutput bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logOutput, nil))
	observability := NewObservability("privacy-release", logger)
	logged := observability.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	request := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/creation-tokens/lat/%.9f?longitude=%.9f&access_token=secret", exactLat, exactLon), nil)
	logged.ServeHTTP(httptest.NewRecorder(), request)
	var logFixture any
	if err = json.Unmarshal(logOutput.Bytes(), &logFixture); err != nil {
		t.Fatalf("decode structured log: %v", err)
	}
	assertPrivacySafeTree(t, "$log", logFixture, exactLat, exactLon)
	if !strings.Contains(logOutput.String(), `"path":"[redacted]"`) {
		t.Fatalf("sensitive log path was not redacted: %s", logOutput.String())
	}
}

type nowClock struct{ now time.Time }

func (c nowClock) Now() time.Time { return c.now }

func assertPrivacySafeTree(t *testing.T, path string, value any, exactCoordinates ...float64) {
	t.Helper()
	forbiddenKeys := []string{"exact_lat", "exact_lon", "exact_location", "creation_coordinate", "private_coordinate", "coordinate_ciphertext", "push_token", "access_token", "refresh_token", "auth_code", "private_key", "moderation_evidence", "scanner_internal"}
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
			for _, forbidden := range forbiddenKeys {
				if strings.Contains(normalized, forbidden) {
					t.Fatalf("privacy release gate: forbidden key at %s.%s", path, key)
				}
			}
			assertPrivacySafeTree(t, path+"."+key, child, exactCoordinates...)
		}
	case []any:
		for index, child := range node {
			assertPrivacySafeTree(t, fmt.Sprintf("%s[%d]", path, index), child, exactCoordinates...)
		}
	case float64:
		for _, exact := range exactCoordinates {
			if math.Abs(node-exact) < 0.000000001 {
				t.Fatalf("privacy release gate: exact coordinate value at %s", path)
			}
		}
	case string:
		for _, exact := range exactCoordinates {
			for _, rendered := range []string{fmt.Sprintf("%.9f", exact), fmt.Sprintf("%.8f", exact), fmt.Sprintf("%.7f", exact)} {
				if strings.Contains(node, rendered) {
					t.Fatalf("privacy release gate: exact coordinate text at %s", path)
				}
			}
		}
	}
}
