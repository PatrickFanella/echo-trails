package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBearerAuthMiddlewareRejectsSpoofedIdentityAndUsesSession(t *testing.T) {
	store := NewMemoryStore()
	user, err := store.FindOrCreateUserByEmail(context.Background(), "real@example.com")
	if err != nil {
		t.Fatal(err)
	}
	token := "act_test"
	if err := store.CreateSession(context.Background(), Session{
		ID: "session", UserID: user.ID, AccessTokenHash: codeHash(token),
		AccessExpiresAt: time.Now().Add(time.Hour), RefreshTokenHash: "refresh", ExpiresAt: time.Now().Add(24 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, email, ok := authenticatedUser(r)
		if !ok || userID != user.ID || email != "real@example.com" {
			t.Fatalf("unexpected authenticated identity: %q %q %v", userID, email, ok)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodGet, "/me", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-EchoTrails-User-ID", "attacker")
	response := httptest.NewRecorder()
	BearerAuthMiddleware(store, next).ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", response.Code)
	}
}

func TestBearerAuthMiddlewareStripsIdentityWithoutToken(t *testing.T) {
	store := NewMemoryStore()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authenticatedUser(r); ok {
			t.Fatal("client-supplied identity must be stripped")
		}
		w.WriteHeader(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.Header.Set("X-EchoTrails-User-ID", "attacker")
	response := httptest.NewRecorder()
	BearerAuthMiddleware(store, next).ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", response.Code)
	}
}
