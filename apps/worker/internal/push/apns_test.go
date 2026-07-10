package push

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestIntegrationAPNsMockServer(t *testing.T) {
	if os.Getenv("ECHOTRAILS_INTEGRATION") != "1" {
		t.Skip("set ECHOTRAILS_INTEGRATION=1")
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("apns-collapse-id") != "place:integration" {
			t.Errorf("missing collapse id")
		}
		w.Header().Set("apns-id", "integration-apns-id")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	provider := testProviderWithClient(t, server.URL, server.Client())
	id, err := provider.Send(context.Background(), Notification{NotificationID: "notification", Token: strings.Repeat("c", 64), Environment: "sandbox", EchoID: "echo", CollapseKey: "place:integration"})
	if err != nil || id != "integration-apns-id" {
		t.Fatalf("id=%q err=%v", id, err)
	}
}

func TestAPNsProviderSendsSafeDeepLinkPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "bearer ") || r.Header.Get("apns-topic") != "com.example.EchoTrails" {
			t.Fatalf("missing APNs authentication headers: %+v", r.Header)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["deep_link"] != "/echoes/echo-1" {
			t.Fatalf("bad deep link payload: %+v", payload)
		}
		encoded, _ := json.Marshal(payload)
		if strings.Contains(string(encoded), "latitude") || strings.Contains(string(encoded), "longitude") {
			t.Fatalf("push payload leaked coordinates: %s", encoded)
		}
		w.Header().Set("apns-id", "apns-test-id")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	provider := testProvider(t, server.URL)
	apnsID, err := provider.Send(context.Background(), Notification{NotificationID: "notification-1", Token: strings.Repeat("a", 64), Environment: "sandbox", Preview: "Someone replied to your Echo.", EchoID: "echo-1", CollapseKey: "reply:echo-1"})
	if err != nil || apnsID != "apns-test-id" {
		t.Fatalf("apns id = %q error = %v", apnsID, err)
	}
}

func TestAPNsProviderRoutesModerationDecisionToAppealAction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["deep_link"] != "/moderation/action-1" {
			t.Fatalf("bad moderation deep link: %+v", payload)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	provider := testProvider(t, server.URL)
	if _, err := provider.Send(context.Background(), Notification{NotificationID: "moderation-1", Token: strings.Repeat("e", 64), Environment: "sandbox", EchoID: "echo-1", ActionID: "action-1"}); err != nil {
		t.Fatal(err)
	}
}

func TestPrivacyReleaseGateAPNsPayload(t *testing.T) {
	const exactLat = "41.797423456"
	const exactLon = "-88.015129876"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		assertPushPrivacyTree(t, "$", payload, exactLat, exactLon)
		encoded, _ := json.Marshal(payload)
		if !strings.Contains(string(encoded), "You have new activity on EchoTrails.") {
			t.Fatalf("coordinate-bearing preview was not made generic: %s", encoded)
		}
		w.Header().Set("apns-id", "privacy-release-apns-id")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	provider := testProvider(t, server.URL)
	_, err := provider.Send(context.Background(), Notification{
		NotificationID: "privacy-release-notification", Token: strings.Repeat("d", 64), Environment: "sandbox",
		Preview: "New activity at " + exactLat + ", " + exactLon, EchoID: "privacy-release-echo", CollapseKey: "privacy-release",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func assertPushPrivacyTree(t *testing.T, path string, value any, forbiddenValues ...string) {
	t.Helper()
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
			for _, forbidden := range []string{"latitude", "longitude", "exact_location", "coordinate", "token", "email", "moderation_evidence"} {
				if strings.Contains(normalized, forbidden) && key != "notification_id" {
					t.Fatalf("privacy release gate: forbidden APNs key at %s.%s", path, key)
				}
			}
			assertPushPrivacyTree(t, path+"."+key, child, forbiddenValues...)
		}
	case []any:
		for index, child := range node {
			assertPushPrivacyTree(t, path+fmt.Sprintf("[%d]", index), child, forbiddenValues...)
		}
	case string:
		for _, forbidden := range forbiddenValues {
			if strings.Contains(node, forbidden) {
				t.Fatalf("privacy release gate: exact coordinate in APNs payload at %s", path)
			}
		}
	}
}

func TestAPNsProviderDisablesUnregisteredToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusGone)
		_, _ = w.Write([]byte(`{"reason":"Unregistered"}`))
	}))
	defer server.Close()
	provider := testProvider(t, server.URL)
	_, err := provider.Send(context.Background(), Notification{Token: strings.Repeat("b", 64), Environment: "production", EchoID: "echo-1"})
	if !errors.Is(err, ErrInvalidDeviceToken) {
		t.Fatalf("error = %v", err)
	}
}

func TestReleaseDrillAPNsSandboxProductionDispatch(t *testing.T) {
	var sandboxCalls atomic.Int32
	var productionCalls atomic.Int32
	var dispatchStarted time.Time
	assertRequest := func(t *testing.T, r *http.Request, environment string) {
		t.Helper()
		if dispatchStarted.IsZero() || time.Since(dispatchStarted) >= time.Second {
			t.Fatalf("%s dispatch did not begin within one second", environment)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "bearer ") || r.Header.Get("apns-topic") != "com.example.EchoTrails" || r.Header.Get("apns-priority") != "10" || r.Header.Get("apns-push-type") != "alert" || r.Header.Get("apns-collapse-id") != "place:release-drill" {
			t.Fatalf("%s APNs headers: %+v", environment, r.Header)
		}
		if !strings.HasPrefix(r.URL.Path, "/3/device/") {
			t.Fatalf("%s APNs device path: %s", environment, r.URL.Path)
		}
		var payload any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		assertPushPrivacyTree(t, "$"+environment, payload)
	}
	sandbox := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sandboxCalls.Add(1)
		assertRequest(t, r, "sandbox")
		w.Header().Set("apns-id", "sandbox-release-drill")
		w.WriteHeader(http.StatusOK)
	}))
	defer sandbox.Close()
	production := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		productionCalls.Add(1)
		assertRequest(t, r, "production")
		w.WriteHeader(http.StatusGone)
		_, _ = w.Write([]byte(`{"reason":"Unregistered"}`))
	}))
	defer production.Close()

	provider := testProviderForEnvironments(t, sandbox.URL, production.URL)
	repository := &releaseDrillPushRepository{deliveries: map[string][]Notification{
		"sandbox-notification":    {{NotificationID: "sandbox-notification", DeviceID: "sandbox-device", Token: strings.Repeat("e", 64), Environment: "sandbox", EchoID: "echo-sandbox", CollapseKey: "place:release-drill"}},
		"production-notification": {{NotificationID: "production-notification", DeviceID: "production-device", Token: strings.Repeat("f", 64), Environment: "production", EchoID: "echo-production", CollapseKey: "place:release-drill"}},
	}}
	dispatcher := Dispatcher{Repository: repository, Provider: provider}
	dispatchStarted = time.Now()
	if err := dispatcher.Dispatch(context.Background(), "sandbox-notification"); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Dispatch(context.Background(), "production-notification"); err != nil {
		t.Fatal(err)
	}
	if sandboxCalls.Load() != 1 || productionCalls.Load() != 1 {
		t.Fatalf("environment routing sandbox=%d production=%d", sandboxCalls.Load(), productionCalls.Load())
	}
	if repository.sent != 1 || repository.disabled != 1 || repository.retried != 0 || repository.disabledDevice != "production-device" {
		t.Fatalf("dispatch outcomes: %+v", repository)
	}
	t.Logf("DRILL_EVIDENCE apns sandbox_calls=%d production_calls=%d sent=%d invalid_token_disabled=%d dispatch_under_60s=true", sandboxCalls.Load(), productionCalls.Load(), repository.sent, repository.disabled)
}

type releaseDrillPushRepository struct {
	deliveries     map[string][]Notification
	sent           int
	disabled       int
	retried        int
	disabledDevice string
}

func (r *releaseDrillPushRepository) Pending(_ context.Context, id string) ([]Notification, error) {
	return r.deliveries[id], nil
}
func (r *releaseDrillPushRepository) MarkSent(_ context.Context, _ Notification, _ string) error {
	r.sent++
	return nil
}
func (r *releaseDrillPushRepository) MarkRetry(_ context.Context, _ Notification, _ error) error {
	r.retried++
	return nil
}
func (r *releaseDrillPushRepository) DisableInvalidToken(_ context.Context, notification Notification, _ error) error {
	r.disabled++
	r.disabledDevice = notification.DeviceID
	return nil
}

func testProviderForEnvironments(t *testing.T, sandboxEndpoint, productionEndpoint string) *APNsProvider {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := NewAPNsProvider(APNsConfig{
		TeamID: "TEAM123", KeyID: "KEY123", BundleID: "com.example.EchoTrails", PrivateKeyPEM: pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}),
		SandboxEndpoint: sandboxEndpoint, ProductionEndpoint: productionEndpoint,
	})
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func testProvider(t *testing.T, endpoint string) *APNsProvider {
	return testProviderWithClient(t, endpoint, serverClient(endpoint))
}

func testProviderWithClient(t *testing.T, endpoint string, client *http.Client) *APNsProvider {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := NewAPNsProvider(APNsConfig{
		TeamID: "TEAM123", KeyID: "KEY123", BundleID: "com.example.EchoTrails",
		PrivateKeyPEM:   pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}),
		SandboxEndpoint: endpoint, ProductionEndpoint: endpoint, HTTPClient: client,
	})
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func serverClient(_ string) *http.Client { return http.DefaultClient }
