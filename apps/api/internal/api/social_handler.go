package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/locationproof"
)

func (h *Handler) handleEchoDetailView(w http.ResponseWriter, r *http.Request) {
	echoID, ok := echoSubresourceID(r.URL.Path, "views")
	if !ok {
		http.NotFound(w, r)
		return
	}
	userID, _, authenticated := authenticatedUser(r)
	if !authenticated {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if err := h.social.RecordDetailView(r.Context(), echoID, userID, h.clock.Now()); err != nil {
		writeSocialError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleEchoReactions(w http.ResponseWriter, r *http.Request) {
	echoID, ok := echoSubresourceID(r.URL.Path, "reactions")
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodPut:
		userID, _, authenticated := authenticatedUser(r)
		if !authenticated {
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		var req ReactionRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_reaction", "invalid reaction request")
			return
		}
		req.UserID = userID
		summary, err := h.social.PutReaction(r.Context(), echoID, req, time.Now().UTC())
		if err != nil {
			writeSocialError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, summary)
	case http.MethodGet:
		known, err := h.social.EchoKnown(r.Context(), echoID)
		if err != nil {
			writeSocialError(w, err)
			return
		}
		if !known {
			writeSocialError(w, ErrSocialNotFound)
			return
		}
		summary, err := h.social.ReactionSummary(r.Context(), echoID)
		if err != nil {
			writeSocialError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, summary)
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
		userID, _, authenticated := authenticatedUser(r)
		if !authenticated {
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		var req ReplyRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_reply", "invalid reply request")
			return
		}
		req.UserID = userID
		reply, err := h.social.AddReply(r.Context(), echoID, req, time.Now().UTC())
		if err != nil {
			writeSocialError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, reply)
	case http.MethodGet:
		known, err := h.social.EchoKnown(r.Context(), echoID)
		if err != nil {
			writeSocialError(w, err)
			return
		}
		if !known {
			writeSocialError(w, ErrSocialNotFound)
			return
		}
		viewerID, _, _ := authenticatedUser(r)
		page, err := h.social.ListReplyPage(r.Context(), echoID, viewerID, r.URL.Query().Get("cursor"), parseInt(r.URL.Query().Get("limit"), 20))
		if err != nil {
			writePaginationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, page)
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

func (h *Handler) handleReplyResource(w http.ResponseWriter, r *http.Request) {
	userID, _, authenticated := authenticatedUser(r)
	if !authenticated {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 2 || len(parts) > 3 || parts[0] != "replies" || !validSocialID(parts[1]) {
		http.NotFound(w, r)
		return
	}
	replyID := parts[1]
	if len(parts) == 3 {
		if parts[2] != "history" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		history, err := h.social.ReplyHistory(r.Context(), replyID, userID)
		if err != nil {
			writeSocialError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"revisions": history})
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var request ReplyRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&request); err != nil {
			writeSocialError(w, ErrSocialInvalid)
			return
		}
		reply, err := h.social.UpdateReply(r.Context(), replyID, userID, request.Body, time.Now().UTC())
		if err != nil {
			writeSocialError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, reply)
	case http.MethodDelete:
		if err := h.social.DeleteReply(r.Context(), replyID, userID, time.Now().UTC()); err != nil {
			writeSocialError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) handleEchoStitch(w http.ResponseWriter, r *http.Request) {
	echoID, ok := echoSubresourceID(r.URL.Path, "stitches")
	if !ok || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	userID, _, authenticated := authenticatedUser(r)
	if !authenticated {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var req StitchRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_stitch", "invalid stitch request")
		return
	}
	req.UserID = userID
	req.SourceEchoID = echoID
	if strings.TrimSpace(req.CreationToken) != "" {
		h.handleCreateFullStitch(w, r, req)
		return
	}
	stitch, err := h.social.AddStitch(r.Context(), req, time.Now().UTC())
	if err != nil {
		writeSocialError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, stitch)
}

func (h *Handler) handleCreateFullStitch(w http.ResponseWriter, r *http.Request, request StitchRequest) {
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" || len(idempotencyKey) > 255 {
		writeError(w, http.StatusBadRequest, "idempotency_key_required", "Idempotency-Key is required")
		return
	}
	createRequest := CreateEchoRequest{
		PlaceID: strings.TrimSpace(request.PlaceID), ContentMode: request.ContentMode, Title: strings.TrimSpace(request.Title),
		BodyText: strings.TrimSpace(request.Body), CaptionText: strings.TrimSpace(request.CaptionText), CreationToken: strings.TrimSpace(request.CreationToken),
	}
	if err := ValidateCreateEchoRequest(createRequest); err != nil {
		writeEchoError(w, err)
		return
	}
	if err := h.social.ValidateStitchSource(r.Context(), request.UserID, request.SourceEchoID, strings.TrimSpace(request.QuotedReplyID), request.QuoteWarningAcknowledged); err != nil {
		writeSocialError(w, err)
		return
	}
	place, found, err := h.store.GetPlace(r.Context(), createRequest.PlaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if !found || place.State != PlaceStateActive {
		writeError(w, http.StatusBadRequest, "place_unavailable", "Place is unavailable for creation")
		return
	}
	now := h.clock.Now()
	if _, err = locationproof.VerifyCreationToken(createRequest.CreationToken, request.UserID, createRequest.PlaceID, now, h.creationSigningKey); err != nil {
		writeEchoError(w, ErrCreationProofMissing)
		return
	}
	authorization, err := h.authorizations.Consume(r.Context(), createRequest.CreationToken, request.UserID, createRequest.PlaceID, idempotencyKey, now)
	if err != nil {
		writeEchoError(w, err)
		return
	}
	profile, found, err := h.profiles.GetProfileByUserID(r.Context(), request.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if !found {
		writeError(w, http.StatusConflict, "profile_required", "Create a public profile before posting")
		return
	}
	publicLocation, err := h.publicLocationForPlace(r.Context(), place, hashCreationToken(createRequest.CreationToken))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	record, err := h.echoes.CreateDraft(r.Context(), EchoCreateInput{
		Request: createRequest, AuthorUserID: request.UserID, AuthorHandle: profile.Handle,
		PrivateLocation: authorization.Coordinate, PublicLocation: publicLocation,
		CreationAuthorizationID: authorization.ID, IdempotencyKey: idempotencyKey, Now: now,
	})
	if err != nil {
		writeEchoError(w, err)
		return
	}
	if err = h.social.LinkStitch(r.Context(), request.SourceEchoID, record.ID, request.UserID, strings.TrimSpace(request.QuotedReplyID), now); err != nil {
		writeSocialError(w, err)
		return
	}
	if err = h.social.RegisterEcho(r.Context(), record.ID, record.AuthorUserID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	writeJSON(w, http.StatusCreated, Stitch{
		ID: record.ID, EchoID: record.ID, UserID: record.AuthorUserID, SourceEchoID: request.SourceEchoID,
		Title: record.Title, Body: record.BodyText, CreatedAt: record.CreatedAt, Status: record.Status, QuotedReplyID: strings.TrimSpace(request.QuotedReplyID),
	})
}

func (h *Handler) handleCreateReport(w http.ResponseWriter, r *http.Request) {
	userID, _, authenticated := authenticatedUser(r)
	if !authenticated {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var req ReportRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_report", "invalid report request")
		return
	}
	if req.TargetType == ReportTargetUser && strings.TrimSpace(req.TargetID) == "" && strings.TrimSpace(req.TargetHandle) != "" && h.profiles != nil {
		profile, found, err := h.profiles.GetProfileByHandle(r.Context(), req.TargetHandle)
		if err != nil || !found {
			writeError(w, http.StatusBadRequest, "invalid_report", "invalid report request")
			return
		}
		req.TargetID = profile.UserID
	}
	req.ReporterID = userID
	report, err := h.social.AddReport(r.Context(), req, time.Now().UTC())
	if err != nil {
		writeSocialError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, report)
}

func (h *Handler) handleHide(w http.ResponseWriter, r *http.Request) {
	userID, _, authenticated := authenticatedUser(r)
	if !authenticated {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var req VisibilityRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_hide", "invalid hide request")
		return
	}
	req.UserID = userID
	if h.social.Hide(r.Context(), req) != nil {
		writeError(w, http.StatusBadRequest, "invalid_hide", "invalid hide request")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleBlock(w http.ResponseWriter, r *http.Request) {
	userID, _, authenticated := authenticatedUser(r)
	if !authenticated {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var request BlockRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_block", "invalid block request")
		return
	}
	targetID := strings.TrimSpace(request.TargetID)
	if targetID == "" && strings.TrimSpace(request.TargetHandle) != "" && h.profiles != nil {
		profile, found, err := h.profiles.GetProfileByHandle(r.Context(), request.TargetHandle)
		if err != nil || !found {
			writeError(w, http.StatusBadRequest, "invalid_block", "invalid block request")
			return
		}
		targetID = profile.UserID
	}
	req := VisibilityRequest{UserID: userID, TargetID: targetID}
	if h.social.Block(r.Context(), req) != nil {
		writeError(w, http.StatusBadRequest, "invalid_block", "invalid block request")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleNotifications(w http.ResponseWriter, r *http.Request) {
	userID, _, authenticated := authenticatedUser(r)
	if !authenticated {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.Method == http.MethodPatch {
		var request struct {
			NotificationIDs []string `json:"notification_ids,omitempty"`
			All             bool     `json:"all,omitempty"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&request); err != nil {
			writeEngagementError(w, ErrEngagementInvalid)
			return
		}
		if err := h.engagement.MarkNotificationsRead(r.Context(), userID, request.NotificationIDs, request.All, h.clock.Now()); err != nil {
			writeEngagementError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	page, err := h.social.NotificationPage(r.Context(), userID, r.URL.Query().Get("cursor"), parseInt(r.URL.Query().Get("limit"), 20))
	if err != nil {
		writeSocialError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func writeSocialError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrSocialInvalid):
		writeError(w, http.StatusBadRequest, "invalid_social_request", "invalid social request")
	case errors.Is(err, ErrSocialHidden):
		writeError(w, http.StatusForbidden, "social_target_hidden", "social target hidden")
	case errors.Is(err, ErrSocialForbidden):
		writeError(w, http.StatusForbidden, "social_forbidden", "social action forbidden")
	case errors.Is(err, ErrSocialNotFound):
		writeError(w, http.StatusNotFound, "social_target_not_found", "social target not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
