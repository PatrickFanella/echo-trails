package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

func (h *Handler) handleAccountBootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	userID, _, ok := authenticatedUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	access, configured := h.account.(accountAccessRepository)
	if !configured {
		writeError(w, http.StatusServiceUnavailable, "access_state_unavailable", "account access state unavailable")
		return
	}
	state, err := access.GetAccountAccessState(r.Context(), userID)
	if err != nil {
		writeAccountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func ReleaseAccessMiddleware(account AccountRepository, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if enforceReleaseAccess(account, w, r) {
			return
		}
		next.ServeHTTP(w, r)
	})
}

func enforceReleaseAccess(account AccountRepository, w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions || strings.HasPrefix(r.URL.Path, "/admin/") {
		return false
	}
	// These authenticated account-safety operations must remain available even
	// before invitation or after a consent version changes.
	if strings.HasPrefix(r.URL.Path, "/privacy/") || strings.HasPrefix(r.URL.Path, "/account/sessions") || r.URL.Path == "/analytics/opt-out" {
		return false
	}
	userID, _, authenticated := authenticatedUser(r)
	if !authenticated {
		return false // The route's normal authentication check returns 401.
	}
	access, production := account.(accountAccessRepository)
	if !production {
		return false
	}
	state, err := access.GetAccountAccessState(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return true
	}
	if r.URL.Path == "/beta/access" {
		return false
	}
	if !state.BetaAccessAccepted {
		writeError(w, http.StatusForbidden, "invite_required", "Valid beta invite required")
		return true
	}
	if r.URL.Path == "/consent/acceptances" {
		return false
	}
	if !state.ConsentAccepted {
		writeError(w, http.StatusPreconditionRequired, "consent_required", "Current terms and privacy consent required")
		return true
	}
	// Profile creation is the final onboarding mutation. Every other product
	// mutation requires a server-confirmed public identity so authorship and
	// safety controls never operate on an unconfigured account.
	if r.URL.Path == "/me" && r.Method == http.MethodPatch {
		return false
	}
	if !state.ProfileConfigured {
		writeError(w, http.StatusPreconditionRequired, "profile_required", "Public profile setup required")
		return true
	}
	return false
}

func (h *Handler) handleBetaAccess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	userID, _, ok := authenticatedUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var request InviteCodeRequest
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&request) != nil {
		writeAccountError(w, ErrInviteInvalid)
		return
	}
	response, err := h.account.ClaimInvite(r.Context(), userID, request.Code, request.City, h.clock.Now())
	if err != nil {
		writeAccountError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) handleConsent(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/consent/current" && r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, CurrentConsentVersion())
		return
	}
	if r.URL.Path != "/consent/acceptances" || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	userID, _, ok := authenticatedUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var request ConsentAcceptanceRequest
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&request) != nil {
		writeAccountError(w, ErrAccountInvalid)
		return
	}
	if err := h.account.AcceptConsent(r.Context(), userID, request, h.clock.Now()); err != nil {
		writeAccountError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) handlePrivacy(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := authenticatedUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.Method == http.MethodPost && (r.URL.Path == "/privacy/exports" || r.URL.Path == "/privacy/deletions") {
		kind := "export"
		if r.URL.Path == "/privacy/deletions" {
			kind = "delete"
		}
		job, err := h.account.QueuePrivacyJob(r.Context(), userID, kind, h.clock.Now())
		if err != nil {
			writeAccountError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, job)
		return
	}
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/privacy/exports/") {
		remainder := strings.TrimPrefix(r.URL.Path, "/privacy/exports/")
		jobID, download := strings.CutSuffix(remainder, "/download")
		if strings.Contains(jobID, "/") || jobID == "" {
			http.NotFound(w, r)
			return
		}
		job, err := h.account.GetPrivacyJob(r.Context(), userID, jobID)
		if err != nil {
			writeAccountError(w, err)
			return
		}
		if download {
			privateObjects, configured := h.media.(PrivateObjectURLStore)
			if !configured || job.Status != "ready" || job.ObjectKey == "" {
				writeError(w, http.StatusConflict, "privacy_export_not_ready", "privacy export is not ready")
				return
			}
			url, urlErr := privateObjects.GetPrivateObjectURL(r.Context(), job.ObjectKey)
			if urlErr != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
				return
			}
			http.Redirect(w, r, url, http.StatusTemporaryRedirect)
			return
		}
		writeJSON(w, http.StatusOK, job)
		return
	}
	http.NotFound(w, r)
}

func (h *Handler) handleAnalytics(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := authenticatedUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.URL.Path == "/analytics/opt-out" && r.Method == http.MethodGet {
		optedOut, err := h.account.GetAnalyticsOptOut(r.Context(), userID)
		if err != nil {
			writeAccountError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, AnalyticsOptOutRequest{OptOut: optedOut})
		return
	}
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	if r.URL.Path == "/analytics/opt-out" {
		var request AnalyticsOptOutRequest
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&request) != nil {
			writeAccountError(w, ErrAccountInvalid)
			return
		}
		if err := h.account.SetAnalyticsOptOut(r.Context(), userID, request.OptOut, h.clock.Now()); err != nil {
			writeAccountError(w, err)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		return
	}
	var event AnalyticsEventEnvelope
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&event) != nil {
		writeAccountError(w, ErrAccountInvalid)
		return
	}
	event.UserID = userID
	if err := h.account.RecordAnalyticsEvent(r.Context(), userID, event, h.clock.Now()); err != nil {
		writeAccountError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) handleAccountSessions(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := authenticatedUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.URL.Path == "/account/sessions" && r.Method == http.MethodGet {
		page, err := h.account.ListAccountSessionPage(r.Context(), userID, r.URL.Query().Get("cursor"), parseInt(r.URL.Query().Get("limit"), 20))
		if err != nil {
			writePaginationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, page)
		return
	}
	if r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/account/sessions/") {
		sessionID := strings.TrimPrefix(r.URL.Path, "/account/sessions/")
		if sessionID == "" || strings.Contains(sessionID, "/") || strings.TrimSpace(r.Header.Get("Idempotency-Key")) == "" {
			writeAccountError(w, ErrAccountInvalid)
			return
		}
		if err := h.account.RevokeAccountSession(r.Context(), userID, sessionID, h.clock.Now()); err != nil {
			writeAccountError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.NotFound(w, r)
}

func writeAccountError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInviteInvalid), errors.Is(err, ErrInviteRequired):
		writeError(w, http.StatusForbidden, "invite_required", "Valid beta invite required")
	case errors.Is(err, ErrAccountInvalid):
		writeError(w, http.StatusBadRequest, "invalid_account_request", "invalid account request")
	case errors.Is(err, ErrEngagementNotFound):
		writeError(w, http.StatusNotFound, "privacy_job_not_found", "privacy job not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
