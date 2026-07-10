package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

func (h *Handler) handleAppeals(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := authenticatedUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if r.URL.Path == "/appeals" {
		switch r.Method {
		case http.MethodPost:
			var request AppealRequest
			if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&request) != nil {
				writeAppealError(w, ErrAppealInvalid)
				return
			}
			appeal, err := h.appeals.CreateAppeal(r.Context(), userID, request, idempotencyKey, h.clock.Now())
			if err != nil {
				writeAppealError(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, appeal)
		case http.MethodGet:
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			page, err := h.appeals.ListAppeals(r.Context(), userID, r.URL.Query().Get("cursor"), limit)
			if err != nil {
				writeAppealError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, page)
		default:
			http.NotFound(w, r)
		}
		return
	}
	if r.Method != http.MethodPatch {
		http.NotFound(w, r)
		return
	}
	appealID := strings.TrimPrefix(r.URL.Path, "/appeals/")
	var request struct {
		Body string `json:"body"`
	}
	if appealID == "" || strings.Contains(appealID, "/") || json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&request) != nil {
		writeAppealError(w, ErrAppealInvalid)
		return
	}
	appeal, err := h.appeals.UpdateAppeal(r.Context(), userID, appealID, request.Body, idempotencyKey, h.clock.Now())
	if err != nil {
		writeAppealError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, appeal)
}

func writeAppealError(w http.ResponseWriter, err error) {
	if err == ErrAppealInvalid {
		writeError(w, http.StatusBadRequest, "invalid_appeal", "valid moderation action, body, and idempotency key required")
		return
	}
	if err == ErrSocialNotFound {
		writeError(w, http.StatusNotFound, "appeal_not_found", "appeal not found")
		return
	}
	writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
}
