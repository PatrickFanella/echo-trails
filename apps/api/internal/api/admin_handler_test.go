package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeAdminRepository struct {
	session AdminSessionRecord
	mutated bool
}

func (f *fakeAdminRepository) CreateAdminSession(context.Context, string, string, string, time.Time) (AdminSessionRecord, error) {
	return f.session, nil
}
func (f *fakeAdminRepository) AuthenticateAdminSession(_ context.Context, _, csrf string, _ time.Time, required bool) (AdminSessionRecord, error) {
	if required && csrf != codeHash("csrf-value") {
		return AdminSessionRecord{}, ErrAdminUnauthorized
	}
	return f.session, nil
}
func (f *fakeAdminRepository) RevokeAdminSession(context.Context, string, time.Time) error {
	return nil
}
func (f *fakeAdminRepository) ListAdminQueue(context.Context, string, string, int) (AdminQueuePage, error) {
	return AdminQueuePage{Items: []AdminQueueItem{}}, nil
}
func (f *fakeAdminRepository) ApplyAdminMutation(_ context.Context, _ AdminSessionRecord, _ string, _ AdminMutationRequest, key string, _ time.Time) error {
	if key == "" {
		return ErrAdminInvalid
	}
	f.mutated = true
	return nil
}
func (f *fakeAdminRepository) CreateAdminInvite(context.Context, AdminSessionRecord, AdminInviteRequest, string, time.Time) error {
	return nil
}
func (f *fakeAdminRepository) GetAdminMediaObject(context.Context, AdminSessionRecord, string) (string, error) {
	return "private/original", nil
}

func TestAdminSessionRequiresServerAuthenticatedRole(t *testing.T) {
	repository := &fakeAdminRepository{session: AdminSessionRecord{UserID: "user", Role: "admin", ExpiresAt: time.Now().Add(time.Hour)}}
	handler := NewHandlerWithConfig(NewStore(DefaultPlaces()), HandlerConfig{Admin: repository})
	request := httptest.NewRequest(http.MethodPost, "/admin/session", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "/admin/session", nil)
	request.Header.Set("X-EchoTrails-User-ID", "user")
	request.Header.Set("X-EchoTrails-User-Role", "admin")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", response.Code)
	}
	if cookie := response.Result().Cookies()[0]; !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("admin cookie does not have release security flags: %#v", cookie)
	}
}

func TestAdminMutationRequiresCSRFAndIdempotency(t *testing.T) {
	repository := &fakeAdminRepository{session: AdminSessionRecord{UserID: "user", Role: "admin", ExpiresAt: time.Now().Add(time.Hour)}}
	handler := NewHandlerWithConfig(NewStore(DefaultPlaces()), HandlerConfig{Admin: repository})
	body := `{"action":"retry","reason_code":"ops_retry","confirmed":true}`
	request := httptest.NewRequest(http.MethodPatch, "/admin/jobs/11111111-1111-4111-8111-111111111111", strings.NewReader(body))
	request.AddCookie(&http.Cookie{Name: adminCookieName, Value: "session"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF status = %d, want 403", response.Code)
	}

	request = httptest.NewRequest(http.MethodPatch, "/admin/jobs/11111111-1111-4111-8111-111111111111", strings.NewReader(body))
	request.AddCookie(&http.Cookie{Name: adminCookieName, Value: "session"})
	request.Header.Set("X-CSRF-Token", "csrf-value")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("missing idempotency status = %d, want 400", response.Code)
	}

	request = httptest.NewRequest(http.MethodPatch, "/admin/jobs/11111111-1111-4111-8111-111111111111", strings.NewReader(body))
	request.AddCookie(&http.Cookie{Name: adminCookieName, Value: "session"})
	request.Header.Set("X-CSRF-Token", "csrf-value")
	request.Header.Set("Idempotency-Key", "request-1")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || !repository.mutated {
		t.Fatalf("valid mutation status=%d mutated=%v", response.Code, repository.mutated)
	}
}
