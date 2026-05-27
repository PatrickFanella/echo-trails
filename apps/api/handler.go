package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

type Handler struct{ store *Store }

func NewHandler(store *Store) http.Handler { return &Handler{store: store} }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/echoes" && r.Method == http.MethodPost {
		h.createEcho(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/echoes/") {
		suffix := strings.TrimPrefix(r.URL.Path, "/echoes/")
		if strings.HasSuffix(suffix, "/publish") && r.Method == http.MethodPost {
			h.publishEcho(w, r, strings.TrimSuffix(suffix, "/publish"))
			return
		}
		if r.Method == http.MethodGet {
			h.getEcho(w, r, suffix)
			return
		}
	}
	http.NotFound(w, r)
}

func (h *Handler) createEcho(w http.ResponseWriter, r *http.Request) {
	authorID := r.Header.Get("X-User-ID")
	if authorID == "" {
		http.Error(w, "missing X-User-ID", http.StatusUnauthorized)
		return
	}
	var req createEchoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := validateCreate(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	e, _, err := h.store.CreateDraft(authorID, r.Header.Get("Idempotency-Key"), req)
	if err == errConflict {
		http.Error(w, "idempotency conflict", http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusCreated, toResponse(e, true))
}

func (h *Handler) publishEcho(w http.ResponseWriter, r *http.Request, id string) {
	authorID := r.Header.Get("X-User-ID")
	if authorID == "" {
		http.Error(w, "missing X-User-ID", http.StatusUnauthorized)
		return
	}
	e, err := h.store.Publish(id, authorID)
	if err == errNotFound {
		http.NotFound(w, r)
		return
	}
	if err == errForbidden {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	writeJSON(w, http.StatusOK, toResponse(e, true))
}

func (h *Handler) getEcho(w http.ResponseWriter, r *http.Request, id string) {
	requester := r.Header.Get("X-User-ID")
	e, err := h.store.Get(id, requester)
	if err == errNotFound {
		http.NotFound(w, r)
		return
	}
	if err == errForbidden {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	writeJSON(w, http.StatusOK, toResponse(e, requester == e.AuthorID))
}

func validateCreate(req createEchoRequest) error {
	if strings.TrimSpace(req.Title) == "" {
		return errorString("title is required")
	}
	if len([]rune(req.Title)) > maxTitleLen {
		return errorString("title must be at most 80 characters")
	}
	if strings.TrimSpace(req.Body) == "" {
		return errorString("body is required")
	}
	if len([]rune(req.Body)) > maxBodyLen {
		return errorString("body must be at most 500 characters")
	}
	return nil
}

type errorString string

func (e errorString) Error() string { return string(e) }

func toResponse(e *Echo, includeExact bool) echoResponse {
	r := echoResponse{ID: e.ID, AuthorID: e.AuthorID, Title: e.Title, Body: e.Body, Status: e.Status, PublicLocation: publicLocation(e.Lat, e.Lon), CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt}
	if includeExact {
		loc := location{Lat: e.Lat, Lon: e.Lon}
		r.ExactLocation = &loc
	}
	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
