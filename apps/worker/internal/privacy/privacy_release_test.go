package privacy

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

func TestPrivacyReleaseGateExportFixture(t *testing.T) {
	const exactLat = 41.797423456
	const exactLon = -88.015129876
	for section, query := range exportSectionQueries {
		normalized := strings.ToLower(query)
		for _, forbidden := range []string{"creation_authorizations", "coordinate_ciphertext", "push_token", "token_hash", "auth_identities", "moderation_evidence"} {
			if strings.Contains(normalized, forbidden) {
				t.Fatalf("privacy export section %q selects forbidden data %q", section, forbidden)
			}
		}
	}

	data, err := marshalExportDocument("privacy-release-user", time.Date(2026, 7, 9, 18, 0, 0, 0, time.UTC), map[string]any{
		"profile":       map[string]any{"handle": "trail_friend", "display_name": "Trail Friend"},
		"echoes":        []any{map[string]any{"id": "echo-1", "place_id": "place-1", "title": "A memory"}},
		"replies":       []any{map[string]any{"id": "reply-1", "body_text": "Thanks for sharing"}},
		"subscriptions": []any{map[string]any{"place_id": "place-1", "notifications_enabled": true}},
		"consents":      []any{map[string]any{"type": "privacy", "version": "beta-1", "granted": true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var fixture any
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	assertExportPrivacyTree(t, "$export", fixture, exactLat, exactLon)
}

func assertExportPrivacyTree(t *testing.T, path string, value any, exactCoordinates ...float64) {
	t.Helper()
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
			for _, forbidden := range []string{"exact_lat", "exact_lon", "exact_location", "creation_coordinate", "coordinate_ciphertext", "push_token", "access_token", "refresh_token", "token_hash", "auth_code", "private_key", "moderation_evidence", "scanner_internal"} {
				if strings.Contains(normalized, forbidden) {
					t.Fatalf("privacy release gate: forbidden export key at %s.%s", path, key)
				}
			}
			assertExportPrivacyTree(t, path+"."+key, child, exactCoordinates...)
		}
	case []any:
		for index, child := range node {
			assertExportPrivacyTree(t, fmt.Sprintf("%s[%d]", path, index), child, exactCoordinates...)
		}
	case float64:
		for _, exact := range exactCoordinates {
			if math.Abs(node-exact) < 0.000000001 {
				t.Fatalf("privacy release gate: exact coordinate value at %s", path)
			}
		}
	case string:
		for _, exact := range exactCoordinates {
			if strings.Contains(node, fmt.Sprintf("%.9f", exact)) {
				t.Fatalf("privacy release gate: exact coordinate text at %s", path)
			}
		}
	}
}
