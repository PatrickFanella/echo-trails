package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type clock interface{ Now() time.Time }
type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

type Config struct {
	Profiles ProfileStore
	Clock    clock
}

type Server struct {
	profiles ProfileStore
	clock    clock
	mux      *http.ServeMux
}

func NewServer(cfg Config) *Server {
	if cfg.Profiles == nil {
		cfg.Profiles = NewMemoryProfileStore()
	}
	if cfg.Clock == nil {
		cfg.Clock = realClock{}
	}
	s := &Server{profiles: cfg.Profiles, clock: cfg.Clock, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) Router() http.Handler { return s.mux }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /me", s.handleGetMe)
	s.mux.HandleFunc("PATCH /me", s.handlePatchMe)
	s.mux.HandleFunc("GET /users/{handle}", s.handleGetPublicProfile)
}

func (s *Server) handleGetMe(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := authenticatedUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Unauthorized")
		return
	}
	profile, found, err := s.profiles.GetProfileByUserID(r.Context(), userID)
	if err != nil {
		slog.Error("profile lookup failed", "error", "get_me_failed")
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "profile_not_found", "Profile not found")
		return
	}
	writeJSON(w, http.StatusOK, toMeProfile(profile, s.clock.Now()))
}

func (s *Server) handlePatchMe(w http.ResponseWriter, r *http.Request) {
	userID, email, ok := authenticatedUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Unauthorized")
		return
	}
	var req UpsertProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Bad request")
		return
	}
	profile, err := s.profiles.UpsertProfile(r.Context(), userID, email, req, s.clock.Now())
	if err != nil {
		status, code, title := profileError(err)
		writeError(w, status, code, title)
		return
	}
	slog.Info("profile upserted", "user_id", userID, "handle", profile.Handle)
	writeJSON(w, http.StatusOK, toMeProfile(profile, s.clock.Now()))
}

func (s *Server) handleGetPublicProfile(w http.ResponseWriter, r *http.Request) {
	handle, err := ValidateHandle(r.PathValue("handle"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_handle", "Invalid handle")
		return
	}
	profile, found, err := s.profiles.GetProfileByHandle(r.Context(), handle)
	if err != nil {
		slog.Error("profile lookup failed", "error", "get_public_profile_failed")
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "profile_not_found", "Profile not found")
		return
	}
	writeJSON(w, http.StatusOK, toPublicProfile(profile))
}

func authenticatedUser(r *http.Request) (userID, email string, ok bool) {
	userID = strings.TrimSpace(r.Header.Get("X-EchoTrails-User-ID"))
	if userID == "" {
		return "", "", false
	}
	return userID, strings.TrimSpace(r.Header.Get("X-EchoTrails-User-Email")), true
}

func profileError(err error) (int, string, string) {
	switch {
	case errors.Is(err, ErrInvalidHandle):
		return http.StatusBadRequest, "invalid_handle", "Invalid handle"
	case errors.Is(err, ErrHandleTaken):
		return http.StatusConflict, "handle_taken", "Handle is already taken"
	case errors.Is(err, ErrHandleChangeTooSoon):
		return http.StatusConflict, "handle_change_cooldown", "Handle change cooldown is active"
	default:
		return http.StatusInternalServerError, "internal_error", "Internal server error"
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, title string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "title": title}})
}
