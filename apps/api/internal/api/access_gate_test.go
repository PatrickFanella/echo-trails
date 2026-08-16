package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type accessGateRepository struct {
	*MemoryAccountRepository
	state AccountAccessState
}

func (r accessGateRepository) GetAccountAccessState(context.Context, string) (AccountAccessState, error) {
	return r.state, nil
}

func TestReleaseAccessMiddlewareEnforcesInviteAndCurrentConsent(t *testing.T) {
	tests := []struct {
		name, method, path string
		state              AccountAccessState
		want               int
	}{
		{name: "invite required", path: "/echoes", want: http.StatusForbidden},
		{name: "consent required", path: "/echoes", state: AccountAccessState{BetaAccessAccepted: true}, want: http.StatusPreconditionRequired},
		{name: "invite claim exempt", path: "/beta/access", want: http.StatusNoContent},
		{name: "consent allowed after invite", path: "/consent/acceptances", state: AccountAccessState{BetaAccessAccepted: true}, want: http.StatusNoContent},
		{name: "profile setup allowed after consent", method: http.MethodPatch, path: "/me", state: AccountAccessState{BetaAccessAccepted: true, ConsentAccepted: true}, want: http.StatusNoContent},
		{name: "profile required for product mutation", path: "/echoes", state: AccountAccessState{BetaAccessAccepted: true, ConsentAccepted: true}, want: http.StatusPreconditionRequired},
		{name: "ready account may mutate", path: "/echoes", state: AccountAccessState{BetaAccessAccepted: true, ConsentAccepted: true, ProfileConfigured: true}, want: http.StatusNoContent},
		{name: "privacy always available", path: "/privacy/deletions", want: http.StatusNoContent},
		{name: "session management always available", path: "/account/sessions/device", want: http.StatusNoContent},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := accessGateRepository{MemoryAccountRepository: NewMemoryAccountRepository(), state: test.state}
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
			method := test.method
			if method == "" {
				method = http.MethodPost
			}
			request := httptest.NewRequest(method, test.path, strings.NewReader(`{}`))
			request.Header.Set("X-EchoTrails-User-ID", "019f4999-0001-7000-8000-000000000001")
			response := httptest.NewRecorder()
			ReleaseAccessMiddleware(repository, next).ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status=%d body=%s want=%d", response.Code, response.Body.String(), test.want)
			}
		})
	}
}

func TestAccountBootstrapReturnsServerAuthoritativeState(t *testing.T) {
	repository := accessGateRepository{
		MemoryAccountRepository: NewMemoryAccountRepository(),
		state:                   AccountAccessState{BetaAccessAccepted: true, ConsentAccepted: true, ProfileConfigured: false},
	}
	handler := NewHandlerWithConfig(NewStore(DefaultPlaces()), HandlerConfig{Account: repository})
	request := httptest.NewRequest(http.MethodGet, "/account/bootstrap", nil)
	request.Header.Set("X-EchoTrails-User-ID", "019f4999-0001-7000-8000-000000000001")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var state AccountAccessState
	if err := json.NewDecoder(response.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	if !state.BetaAccessAccepted || !state.ConsentAccepted || state.ProfileConfigured {
		t.Fatalf("unexpected state: %+v", state)
	}
}
