package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

type Handler struct{ store *Store }

func NewHandler(store *Store) http.Handler { return &Handler{store: store} }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/places/search":
		h.handleSearch(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/places/requests":
		h.handleRequest(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/places/"):
		h.handleDetail(w, r)
	default:
		http.NotFound(w, r)
	}
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
