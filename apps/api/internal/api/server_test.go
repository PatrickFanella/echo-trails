package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fixedClock struct{ now time.Time }

func (f fixedClock) Now() time.Time { return f.now }

type captureSender struct {
	code      string
	requestID string
	email     string
	expiresAt time.Time
}

func (c *captureSender) SendAuthCode(_ context.Context, email, requestID, code string, expiresAt time.Time) error {
	c.email, c.requestID, c.code, c.expiresAt = email, requestID, code, expiresAt
	return nil
}

func doJSON(t *testing.T, handler http.Handler, method, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func TestEmailAuthAndSessionFlow(t *testing.T) {
	clock := fixedClock{now: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	sender := &captureSender{}
	server := NewServer(Config{Store: NewMemoryStore(), Sender: sender, Clock: clock})

	start := doJSON(t, server.Router(), http.MethodPost, "/auth/email/start", `{"email":"Test@Example.com"}`)
	if start.Code != http.StatusAccepted {
		t.Fatalf("start status = %d", start.Code)
	}
	if sender.code == "" || sender.requestID == "" {
		t.Fatal("expected captured code and request id")
	}

	verify := doJSON(t, server.Router(), http.MethodPost, "/auth/email/verify", `{"request_id":"`+sender.requestID+`","code":"`+sender.code+`","device_name":"iPhone"}`)
	if verify.Code != http.StatusOK {
		t.Fatalf("verify status = %d", verify.Code)
	}

	var vr VerifyEmailResponse
	if err := json.NewDecoder(verify.Body).Decode(&vr); err != nil {
		t.Fatal(err)
	}
	if vr.AccessToken == "" || vr.RefreshToken == "" || vr.SessionID == "" || vr.UserID == "" {
		t.Fatal("missing tokens")
	}

	refresh := doJSON(t, server.Router(), http.MethodPost, "/sessions/refresh", `{"refresh_token":"`+vr.RefreshToken+`"}`)
	if refresh.Code != http.StatusOK {
		t.Fatalf("refresh status = %d", refresh.Code)
	}
	var rrsp VerifyEmailResponse
	if err := json.NewDecoder(refresh.Body).Decode(&rrsp); err != nil {
		t.Fatal(err)
	}
	if rrsp.RefreshToken == vr.RefreshToken {
		t.Fatal("expected rotated refresh token")
	}

	logout := doJSON(t, server.Router(), http.MethodPost, "/sessions/logout", `{"refresh_token":"`+rrsp.RefreshToken+`"}`)
	if logout.Code != http.StatusOK {
		t.Fatalf("logout status = %d", logout.Code)
	}
	again := doJSON(t, server.Router(), http.MethodPost, "/sessions/refresh", `{"refresh_token":"`+rrsp.RefreshToken+`"}`)
	if again.Code != http.StatusUnauthorized {
		t.Fatalf("expected revoked token to fail, got %d", again.Code)
	}
}

func TestExpiredAndReplayRejected(t *testing.T) {
	clock := fixedClock{now: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	sender := &captureSender{}
	store := NewMemoryStore()
	server := NewServer(Config{Store: store, Sender: sender, Clock: clock})
	start := doJSON(t, server.Router(), http.MethodPost, "/auth/email/start", `{"email":"user@example.com"}`)
	if start.Code != http.StatusAccepted {
		t.Fatal(start.Code)
	}
	var s StartEmailResponse
	_ = json.NewDecoder(start.Body).Decode(&s)
	ar, _ := store.GetAuthRequest(context.Background(), s.RequestID)
	past := clock.now.Add(-time.Minute)
	ar.ExpiresAt = past
	_ = store.CreateAuthRequest(context.Background(), ar)
	resp := doJSON(t, server.Router(), http.MethodPost, "/auth/email/verify", `{"request_id":"`+s.RequestID+`","code":"`+sender.code+`"}`)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expired verify status = %d", resp.Code)
	}

	start2 := doJSON(t, server.Router(), http.MethodPost, "/auth/email/start", `{"email":"user2@example.com"}`)
	var s2 StartEmailResponse
	_ = json.NewDecoder(start2.Body).Decode(&s2)
	replay1 := doJSON(t, server.Router(), http.MethodPost, "/auth/email/verify", `{"request_id":"`+s2.RequestID+`","code":"`+sender.code+`"}`)
	if replay1.Code != http.StatusOK {
		t.Fatalf("first verify status = %d", replay1.Code)
	}
	replay2 := doJSON(t, server.Router(), http.MethodPost, "/auth/email/verify", `{"request_id":"`+s2.RequestID+`","code":"`+sender.code+`"}`)
	if replay2.Code != http.StatusUnauthorized {
		t.Fatalf("replay status = %d", replay2.Code)
	}
}

func TestCodeAtExactExpiryAndTooManyAttemptsRejected(t *testing.T) {
	clock := fixedClock{now: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	sender := &captureSender{}
	store := NewMemoryStore()
	server := NewServer(Config{Store: store, Sender: sender, Clock: clock, CodePepper: []byte("test-pepper")})

	start := doJSON(t, server.Router(), http.MethodPost, "/auth/email/start", `{"email":"boundary@example.com"}`)
	if start.Code != http.StatusAccepted {
		t.Fatal(start.Code)
	}
	var s StartEmailResponse
	if err := json.NewDecoder(start.Body).Decode(&s); err != nil {
		t.Fatal(err)
	}
	ar, _ := store.GetAuthRequest(context.Background(), s.RequestID)
	ar.ExpiresAt = clock.now
	if err := store.CreateAuthRequest(context.Background(), ar); err != nil {
		t.Fatal(err)
	}
	boundary := doJSON(t, server.Router(), http.MethodPost, "/auth/email/verify", `{"request_id":"`+s.RequestID+`","code":"`+sender.code+`"}`)
	if boundary.Code != http.StatusUnauthorized {
		t.Fatalf("code at exact expiry status = %d", boundary.Code)
	}

	start2 := doJSON(t, server.Router(), http.MethodPost, "/auth/email/start", `{"email":"attempts@example.com"}`)
	if start2.Code != http.StatusAccepted {
		t.Fatal(start2.Code)
	}
	var s2 StartEmailResponse
	if err := json.NewDecoder(start2.Body).Decode(&s2); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		wrong := doJSON(t, server.Router(), http.MethodPost, "/auth/email/verify", `{"request_id":"`+s2.RequestID+`","code":"wrong"}`)
		if wrong.Code != http.StatusUnauthorized {
			t.Fatalf("wrong code status = %d", wrong.Code)
		}
	}
	locked := doJSON(t, server.Router(), http.MethodPost, "/auth/email/verify", `{"request_id":"`+s2.RequestID+`","code":"`+sender.code+`"}`)
	if locked.Code != http.StatusUnauthorized {
		t.Fatalf("valid code after too many attempts status = %d", locked.Code)
	}
}
