package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

type Handler struct{ store *PlacesStore }

func NewHandler(store *PlacesStore) http.Handler { return &Handler{store: store} }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/places/search":
		h.handleSearch(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/places/requests":
		h.handleRequest(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/discovery/nearby":
		h.handleNearby(w, r)
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/echoes") && strings.HasPrefix(r.URL.Path, "/places/"):
		h.handlePlaceEchoes(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/places/"):
		h.handleDetail(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) handleNearby(w http.ResponseWriter, r *http.Request) {
	lat, latErr := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lon, lonErr := strconv.ParseFloat(r.URL.Query().Get("lon"), 64)
	if latErr != nil || lonErr != nil || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		writeError(w, http.StatusBadRequest, "invalid_coordinates", "valid lat and lon are required")
		return
	}
	radius, ok := parseOptionalInt(r.URL.Query().Get("radius_meters"), 1000)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_radius", "radius_meters must be an integer")
		return
	}
	if radius < 10 || radius > 10000 {
		writeError(w, http.StatusBadRequest, "invalid_radius", "radius_meters must be between 10 and 10000")
		return
	}
	includeRestricted := r.URL.Query().Get("include_restricted") == "true"
	writeJSON(w, http.StatusOK, map[string]any{"places": h.store.Nearby(lat, lon, radius, includeRestricted)})
}

func (h *Handler) handlePlaceEchoes(w http.ResponseWriter, r *http.Request) {
	placeID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/places/"), "/echoes")
	if p, ok := h.store.Get(placeID); !ok || p.State != PlaceStateActive {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"echoes": h.store.Echoes(placeID, parseInt(r.URL.Query().Get("limit"), 20)), "next_cursor": ""})
}

func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		q = r.URL.Query().Get("query")
	}
	includeRestricted := r.URL.Query().Get("include_restricted") == "true"
	writeJSON(w, http.StatusOK, map[string]any{"places": h.store.Search(q, includeRestricted)})
}

func (h *Handler) handleDetail(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/places/")
	p, ok := h.store.Get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch p.State {
	case PlaceStateActive, PlaceStateRestricted, PlaceStatePendingReview:
		writeJSON(w, http.StatusOK, p)
	case PlaceStateMerged:
		w.Header().Set("Location", "/places/"+p.CanonicalID)
		w.WriteHeader(http.StatusPermanentRedirect)
	case PlaceStateRejected:
		writeJSON(w, http.StatusGone, map[string]any{"id": p.ID, "state": p.State, "replacement_id": p.ReplacementID, "canonical_id": p.CanonicalID, "rejection_reason": p.RejectionReason})
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) handleRequest(w http.ResponseWriter, r *http.Request) {
	var req PlaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	p := h.store.CreateRequest(req)
	writeJSON(w, http.StatusCreated, p)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, title string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "title": title}})
}

func parseInt(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func parseOptionalInt(value string, fallback int) (int, bool) {
	if value == "" {
		return fallback, true
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}
	return parsed, true
}
