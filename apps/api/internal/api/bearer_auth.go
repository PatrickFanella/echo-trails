package api

import (
	"log/slog"
	"net/http"
	"strings"
)

// BearerAuthMiddleware makes the authenticated session the only source of
// request identity. Compatibility headers are populated internally for the
// existing handlers and all client-supplied values are discarded.
func BearerAuthMiddleware(store Store, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Del("X-EchoTrails-User-ID")
		r.Header.Del("X-EchoTrails-User-Email")
		r.Header.Del("X-EchoTrails-User-Role")
		r.Header.Del("X-EchoTrails-User-Status")

		authorization := strings.TrimSpace(r.Header.Get("Authorization"))
		if authorization != "" {
			scheme, token, found := strings.Cut(authorization, " ")
			if !found || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
				writeError(w, http.StatusUnauthorized, "invalid_access_token", "invalid access token")
				return
			}
			user, ok, err := store.AuthenticateAccessToken(r.Context(), codeHash(strings.TrimSpace(token)), realClock{}.Now())
			if err != nil {
				slog.Error("access token lookup failed", "error", "authenticate_access_token_failed")
				writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
				return
			}
			if !ok {
				writeError(w, http.StatusUnauthorized, "invalid_access_token", "invalid access token")
				return
			}
			r.Header.Set("X-EchoTrails-User-ID", user.ID)
			r.Header.Set("X-EchoTrails-User-Email", user.Email)
			r.Header.Set("X-EchoTrails-User-Role", user.Role)
			r.Header.Set("X-EchoTrails-User-Status", user.Status)
		}
		next.ServeHTTP(w, r)
	})
}
