package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

const adminCookieName = "echotrails_admin"

func (h *Handler) handleAdmin(w http.ResponseWriter, r *http.Request) {
	if h.admin == nil {
		writeError(w, http.StatusServiceUnavailable, "admin_unavailable", "admin service unavailable")
		return
	}
	if r.URL.Path == "/admin/session" {
		h.handleAdminSession(w, r)
		return
	}
	mutating := r.Method != http.MethodGet && r.Method != http.MethodHead
	session, ok := h.authenticateAdminRequest(w, r, mutating)
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		if strings.HasPrefix(r.URL.Path, "/admin/media/") && strings.HasSuffix(r.URL.Path, "/preview") {
			reviewID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/admin/media/"), "/preview")
			key, err := h.admin.GetAdminMediaObject(r.Context(), session, reviewID)
			if err != nil {
				writeAdminError(w, err)
				return
			}
			privateObjects, configured := h.media.(PrivateObjectURLStore)
			if !configured {
				writeError(w, http.StatusServiceUnavailable, "preview_unavailable", "private preview unavailable")
				return
			}
			url, err := privateObjects.GetPrivateObjectURL(r.Context(), key)
			if err != nil {
				writeAdminError(w, err)
				return
			}
			http.Redirect(w, r, url, http.StatusTemporaryRedirect)
			return
		}
		queue := strings.TrimPrefix(r.URL.Path, "/admin/")
		if queue == "reports" || queue == "media" || queue == "places" || queue == "jobs" || queue == "appeals" || queue == "audit" {
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			page, err := h.admin.ListAdminQueue(r.Context(), queue, r.URL.Query().Get("cursor"), limit)
			if err != nil {
				writeAdminError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, page)
			return
		}
	}
	if r.Method == http.MethodPost && r.URL.Path == "/admin/invites" {
		var request AdminInviteRequest
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&request) != nil {
			writeAdminError(w, ErrAdminInvalid)
			return
		}
		if err := h.admin.CreateAdminInvite(r.Context(), session, request, r.Header.Get("Idempotency-Key"), h.clock.Now()); err != nil {
			writeAdminError(w, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
		return
	}
	resource := ""
	var request AdminMutationRequest
	if r.Method == http.MethodPost && r.URL.Path == "/admin/moderation-actions" {
		resource = "moderation"
	} else if r.Method == http.MethodPatch {
		for prefix, kind := range map[string]string{"/admin/places/": "place", "/admin/media/": "review", "/admin/appeals/": "appeal", "/admin/jobs/": "job"} {
			if strings.HasPrefix(r.URL.Path, prefix) {
				resource = kind
				request.TargetID = strings.TrimPrefix(r.URL.Path, prefix)
				break
			}
		}
	}
	if resource != "" {
		targetID := request.TargetID
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&request) != nil {
			writeAdminError(w, ErrAdminInvalid)
			return
		}
		if targetID != "" {
			request.TargetID = targetID
		}
		if err := h.admin.ApplyAdminMutation(r.Context(), session, resource, request, r.Header.Get("Idempotency-Key"), h.clock.Now()); err != nil {
			writeAdminError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.NotFound(w, r)
}

func (h *Handler) handleAdminSession(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		userID, _, authenticated := authenticatedUser(r)
		role := r.Header.Get("X-EchoTrails-User-Role")
		if !authenticated || (role != "moderator" && role != "admin") {
			writeAdminError(w, ErrAdminUnauthorized)
			return
		}
		token, err := randToken("adm_", 32)
		if err != nil {
			writeAdminError(w, err)
			return
		}
		csrf, err := randToken("csrf_", 32)
		if err != nil {
			writeAdminError(w, err)
			return
		}
		session, err := h.admin.CreateAdminSession(r.Context(), userID, codeHash(token), codeHash(csrf), h.clock.Now())
		if err != nil {
			writeAdminError(w, err)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: adminCookieName, Value: token, Path: "/admin", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode, Expires: session.ExpiresAt})
		writeJSON(w, http.StatusCreated, map[string]any{"csrf_token": csrf, "role": session.Role, "expires_at": session.ExpiresAt})
	case http.MethodDelete:
		cookie, err := r.Cookie(adminCookieName)
		if err != nil {
			writeAdminError(w, ErrAdminUnauthorized)
			return
		}
		if _, ok := h.authenticateAdminRequest(w, r, true); !ok {
			return
		}
		if err = h.admin.RevokeAdminSession(r.Context(), codeHash(cookie.Value), h.clock.Now()); err != nil {
			writeAdminError(w, err)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: adminCookieName, Value: "", Path: "/admin", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) authenticateAdminRequest(w http.ResponseWriter, r *http.Request, requireCSRF bool) (AdminSessionRecord, bool) {
	cookie, err := r.Cookie(adminCookieName)
	if err != nil {
		writeAdminError(w, ErrAdminUnauthorized)
		return AdminSessionRecord{}, false
	}
	csrf := ""
	if requireCSRF {
		csrf = strings.TrimSpace(r.Header.Get("X-CSRF-Token"))
		if csrf == "" {
			writeAdminError(w, ErrAdminUnauthorized)
			return AdminSessionRecord{}, false
		}
	}
	session, err := h.admin.AuthenticateAdminSession(r.Context(), codeHash(cookie.Value), codeHash(csrf), h.clock.Now(), requireCSRF)
	if err != nil {
		writeAdminError(w, err)
		return AdminSessionRecord{}, false
	}
	return session, true
}

func writeAdminError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrAdminUnauthorized):
		writeError(w, http.StatusForbidden, "admin_forbidden", "administrator authorization required")
	case errors.Is(err, ErrAdminInvalid):
		writeError(w, http.StatusBadRequest, "invalid_admin_request", "confirmation, reason code, CSRF token, and idempotency key are required")
	case errors.Is(err, ErrAdminNotFound):
		writeError(w, http.StatusNotFound, "admin_target_not_found", "admin target not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
