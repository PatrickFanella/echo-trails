package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

func (h *Handler) handlePlaceSubscription(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := authenticatedUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	placeID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/places/"), "/subscription")
	if !validSocialID(placeID) || strings.Contains(placeID, "/") {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodPut:
		// Only curated public Places can be newly subscribed to. DELETE remains
		// idempotent even if a Place was later removed, allowing clients to
		// clean up stale subscriptions.
		place, found, err := h.store.GetPlace(r.Context(), placeID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
		if !found || place.State != PlaceStateActive {
			writeError(w, http.StatusNotFound, "place_not_found", "Place not found")
			return
		}
		request := struct {
			NotificationsEnabled *bool `json:"notifications_enabled,omitempty"`
		}{}
		if r.ContentLength > 0 && json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&request) != nil {
			writeEngagementError(w, ErrEngagementInvalid)
			return
		}
		enabled := true
		if request.NotificationsEnabled != nil {
			enabled = *request.NotificationsEnabled
		}
		subscription, err := h.engagement.PutSubscription(r.Context(), userID, placeID, enabled, h.clock.Now())
		if err != nil {
			writeEngagementError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, subscription)
	case http.MethodDelete:
		if err := h.engagement.DeleteSubscription(r.Context(), userID, placeID); err != nil {
			writeEngagementError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) handleSubscriptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	userID, _, ok := authenticatedUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	page, err := h.engagement.ListSubscriptionPage(r.Context(), userID, r.URL.Query().Get("cursor"), parseInt(r.URL.Query().Get("limit"), 20))
	if err != nil {
		writePaginationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) handleDevices(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := authenticatedUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.URL.Path == "/devices" && r.Method == http.MethodPost {
		var request DeviceRegistrationRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&request); err != nil {
			writeEngagementError(w, ErrEngagementInvalid)
			return
		}
		device, err := h.engagement.RegisterDevice(r.Context(), userID, request, h.clock.Now())
		if err != nil {
			writeEngagementError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, device)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/devices/") && r.Method == http.MethodDelete {
		deviceID := strings.TrimPrefix(r.URL.Path, "/devices/")
		if err := h.engagement.DeleteDevice(r.Context(), userID, deviceID); err != nil {
			writeEngagementError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.NotFound(w, r)
}

func (h *Handler) handleNotificationPreferences(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := authenticatedUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if r.Method == http.MethodGet {
		preferences, err := h.engagement.GetNotificationPreferences(r.Context(), userID, h.clock.Now())
		if err != nil {
			writeEngagementError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, preferences)
		return
	}
	if r.Method != http.MethodPatch {
		http.NotFound(w, r)
		return
	}
	var patch NotificationPreferencesPatch
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&patch); err != nil {
		writeEngagementError(w, ErrEngagementInvalid)
		return
	}
	preferences, err := h.engagement.PatchNotificationPreferences(r.Context(), userID, patch, h.clock.Now())
	if err != nil {
		writeEngagementError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, preferences)
}

func (h *Handler) handleMute(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := authenticatedUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/mutes/"), "/")
	if len(parts) != 2 || (parts[0] != "places" && parts[0] != "echoes") || !validSocialID(parts[1]) {
		http.NotFound(w, r)
		return
	}
	targetType := strings.TrimSuffix(parts[0], "s")
	switch r.Method {
	case http.MethodPut:
		var request struct {
			MutedUntil *time.Time `json:"muted_until,omitempty"`
		}
		if r.ContentLength > 0 && json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&request) != nil {
			writeEngagementError(w, ErrEngagementInvalid)
			return
		}
		if err := h.engagement.PutMute(r.Context(), userID, targetType, parts[1], request.MutedUntil, h.clock.Now()); err != nil {
			writeEngagementError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		if err := h.engagement.DeleteMute(r.Context(), userID, targetType, parts[1]); err != nil {
			writeEngagementError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

func writeEngagementError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrEngagementInvalid):
		writeError(w, http.StatusBadRequest, "invalid_engagement_request", "invalid engagement request")
	case errors.Is(err, ErrEngagementNotFound):
		writeError(w, http.StatusNotFound, "engagement_target_not_found", "engagement target not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
