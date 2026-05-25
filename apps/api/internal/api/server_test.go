package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fixedClock struct{ now time.Time }

func (f fixedClock) Now() time.Time { return f.now }

func doJSON(t *testing.T, handler http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func authHeaders(userID, email string) map[string]string {
	return map[string]string{"X-EchoTrails-User-ID": userID, "X-EchoTrails-User-Email": email}
}

func TestProfileEndpointsMeAndPublicHandle(t *testing.T) {
	server := NewServer(Config{Clock: fixedClock{now: time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)}})

	create := doJSON(t, server.Router(), http.MethodPatch, "/me", `{"handle":"TrailVoice","display_name":"Trail Voice","bio":"Public place stories"}`, authHeaders("usr_1", "private@example.com"))
	if create.Code != http.StatusOK {
		t.Fatalf("create status = %d body=%s", create.Code, create.Body.String())
	}

	var me MeProfile
	if err := json.NewDecoder(create.Body).Decode(&me); err != nil {
		t.Fatal(err)
	}
	if me.UserID != "usr_1" || me.Email != "private@example.com" || me.Handle != "trailvoice" {
		t.Fatalf("unexpected /me response: %+v", me)
	}

	getMe := doJSON(t, server.Router(), http.MethodGet, "/me", ``, authHeaders("usr_1", "private@example.com"))
	if getMe.Code != http.StatusOK {
		t.Fatalf("get /me status = %d", getMe.Code)
	}

	public := doJSON(t, server.Router(), http.MethodGet, "/users/trailvoice", ``, nil)
	if public.Code != http.StatusOK {
		t.Fatalf("public status = %d body=%s", public.Code, public.Body.String())
	}
	var publicBody map[string]any
	if err := json.NewDecoder(public.Body).Decode(&publicBody); err != nil {
		t.Fatal(err)
	}
	if publicBody["handle"] != "trailvoice" || publicBody["display_name"] != "Trail Voice" || publicBody["bio"] != "Public place stories" {
		t.Fatalf("unexpected public body: %+v", publicBody)
	}
	if _, ok := publicBody["email"]; ok {
		t.Fatalf("public profile leaked email: %+v", publicBody)
	}
	if _, ok := publicBody["user_id"]; ok {
		t.Fatalf("public profile leaked user_id: %+v", publicBody)
	}
	if _, ok := publicBody["handle_change_available_at"]; ok {
		t.Fatalf("public profile leaked private cooldown field: %+v", publicBody)
	}
}

func TestProfileEndpointsRejectInvalidAndTakenHandles(t *testing.T) {
	server := NewServer(Config{Clock: fixedClock{now: time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)}})

	invalid := doJSON(t, server.Router(), http.MethodPatch, "/me", `{"handle":"bad handle","display_name":"Bad"}`, authHeaders("usr_1", "one@example.com"))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid status = %d", invalid.Code)
	}

	first := doJSON(t, server.Router(), http.MethodPatch, "/me", `{"handle":"unique","display_name":"One"}`, authHeaders("usr_1", "one@example.com"))
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d", first.Code)
	}
	second := doJSON(t, server.Router(), http.MethodPatch, "/me", `{"handle":"UNIQUE","display_name":"Two"}`, authHeaders("usr_2", "two@example.com"))
	if second.Code != http.StatusConflict {
		t.Fatalf("taken status = %d", second.Code)
	}
}

func TestProfileEndpointsRequireAuthenticatedMe(t *testing.T) {
	server := NewServer(Config{})
	resp := doJSON(t, server.Router(), http.MethodPatch, "/me", `{"handle":"anon","display_name":"Anon"}`, nil)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", resp.Code)
	}
}
