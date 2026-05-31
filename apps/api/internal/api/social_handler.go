package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

func (h *Handler) handleEchoReactions(w http.ResponseWriter, r *http.Request) {
	echoID, ok := echoSubresourceID(r.URL.Path, "reactions")
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodPut:
		var req ReactionRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_reaction", "invalid reaction request")
			return
		}
		summary, err := h.social.PutReaction(echoID, req, time.Now().UTC())
		if err != nil {
			writeSocialError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, summary)
	case http.MethodGet:
		if !h.social.EchoKnown(echoID) {
			writeSocialError(w, ErrSocialNotFound)
			return
		}
		writeJSON(w, http.StatusOK, h.social.ReactionSummary(echoID))
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) handleEchoReplies(w http.ResponseWriter, r *http.Request) {
	echoID, ok := echoSubresourceID(r.URL.Path, "replies")
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodPost:
		var req ReplyRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_reply", "invalid reply request")
			return
		}
		reply, err := h.social.AddReply(echoID, req, time.Now().UTC())
		if err != nil {
			writeSocialError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, reply)
	case http.MethodGet:
		if !h.social.EchoKnown(echoID) {
			writeSocialError(w, ErrSocialNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"replies": h.social.ListReplies(echoID, strings.TrimSpace(r.URL.Query().Get("viewer_id")))})
	default:
		http.NotFound(w, r)
	}
}

func echoSubresourceID(path, subresource string) (string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 3 || parts[0] != "echoes" || parts[2] != subresource || !validSocialID(parts[1]) {
		return "", false
	}
	return parts[1], true
}

func (h *Handler) handleEchoStitch(w http.ResponseWriter, r *http.Request) {
	echoID, ok := echoSubresourceID(r.URL.Path, "stitches")
	if !ok || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var req StitchRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_stitch", "invalid stitch request")
		return
	}
	req.SourceEchoID = echoID
	stitch, err := h.social.AddStitch(req, time.Now().UTC())
	if err != nil {
		writeSocialError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, stitch)
}

func (h *Handler) handleCreateReport(w http.ResponseWriter, r *http.Request) {
	var req ReportRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_report", "invalid report request")
		return
	}
	report, err := h.social.AddReport(req, time.Now().UTC())
	if err != nil {
		writeSocialError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, report)
}

func (h *Handler) handleHide(w http.ResponseWriter, r *http.Request) {
	var req VisibilityRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil || h.social.Hide(req) != nil {
		writeError(w, http.StatusBadRequest, "invalid_hide", "invalid hide request")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleBlock(w http.ResponseWriter, r *http.Request) {
	var req VisibilityRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil || h.social.Block(req) != nil {
		writeError(w, http.StatusBadRequest, "invalid_block", "invalid block request")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleNotifications(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.URL.Query().Get("user_id"))
	if userID == "" {
		writeError(w, http.StatusBadRequest, "invalid_notifications", "user_id is required")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": h.social.Notifications(userID)})
}

func writeSocialError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrSocialInvalid):
		writeError(w, http.StatusBadRequest, "invalid_social_request", "invalid social request")
	case errors.Is(err, ErrSocialHidden):
		writeError(w, http.StatusForbidden, "social_target_hidden", "social target hidden")
	case errors.Is(err, ErrSocialNotFound):
		writeError(w, http.StatusNotFound, "social_target_not_found", "social target not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
