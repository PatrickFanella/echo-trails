package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestSubscriptionsPreferencesDevicesAndMutesArePrivate(t *testing.T) {
	handler := NewHandler(NewStore(DefaultPlaces()))
	headers := authHeaders("user-private", "private@example.com")

	subscribe := doJSON(t, handler, http.MethodPut, "/places/"+PlacePrincePondID+"/subscription", `{"notifications_enabled":true}`, headers)
	if subscribe.Code != http.StatusOK {
		t.Fatalf("subscribe status = %d body=%s", subscribe.Code, subscribe.Body.String())
	}
	list := doJSON(t, handler, http.MethodGet, "/subscriptions", ``, headers)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), PlacePrincePondID) {
		t.Fatalf("subscription list = %d %s", list.Code, list.Body.String())
	}

	preferences := doJSON(t, handler, http.MethodPatch, "/notification-preferences", `{"place_alerts_enabled":false}`, headers)
	if preferences.Code != http.StatusOK || !strings.Contains(preferences.Body.String(), `"place_alerts_enabled":false`) {
		t.Fatalf("preferences = %d %s", preferences.Code, preferences.Body.String())
	}

	token := strings.Repeat("ab", 32)
	device := doJSON(t, handler, http.MethodPost, "/devices", `{"token":"`+token+`","environment":"sandbox","app_version":"1.0"}`, headers)
	if device.Code != http.StatusCreated || strings.Contains(device.Body.String(), token) {
		t.Fatalf("device response leaked token or failed: %d %s", device.Code, device.Body.String())
	}

	mute := doJSON(t, handler, http.MethodPut, "/mutes/places/"+PlacePrincePondID, `{}`, headers)
	if mute.Code != http.StatusNoContent {
		t.Fatalf("mute status = %d body=%s", mute.Code, mute.Body.String())
	}
}
