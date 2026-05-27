package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type Config struct {
	Store      Store
	Profiles   ProfileStore
	Sender     EmailSender
	Clock      clock
	CodePepper []byte
}

type Server struct {
	store    Store
	profiles ProfileStore
	sender   EmailSender
	clock    clock
	pepper   []byte
	mux      *http.ServeMux
}

func NewServer(cfg Config) *Server {
	if cfg.Store == nil {
		cfg.Store = NewMemoryStore()
	}
	if cfg.Profiles == nil {
		cfg.Profiles = NewMemoryProfileStore()
	}
	if cfg.Sender == nil {
		cfg.Sender = DevEmailSender{}
	}
	if cfg.Clock == nil {
		cfg.Clock = realClock{}
	}
	if len(cfg.CodePepper) == 0 {
		pepper, err := randomBytes(32)
		if err != nil {
			panic("generate auth code pepper: " + err.Error())
		}
		cfg.CodePepper = pepper
	}
	s := &Server{store: cfg.Store, profiles: cfg.Profiles, sender: cfg.Sender, clock: cfg.Clock, pepper: cfg.CodePepper, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) Router() http.Handler { return s.mux }

func (s *Server) routes() {
	s.mux.HandleFunc("POST /auth/email/start", s.handleStart)
	s.mux.HandleFunc("POST /auth/email/verify", s.handleVerify)
	s.mux.HandleFunc("POST /sessions/refresh", s.handleRefresh)
	s.mux.HandleFunc("POST /sessions/logout", s.handleLogout)
	s.mux.HandleFunc("GET /me", s.handleGetMe)
	s.mux.HandleFunc("PATCH /me", s.handlePatchMe)
	s.mux.HandleFunc("GET /users/{handle}", s.handleGetPublicProfile)
}

func (s *Server) writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func decodeJSON(r *http.Request, v any) error { return json.NewDecoder(r.Body).Decode(v) }

func (s *Server) writeError(w http.ResponseWriter, status int, code, title string) {
	s.writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "title": title}})
}

func (s *Server) handleGetMe(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := authenticatedUser(r)
	if !ok {
		s.writeError(w, http.StatusUnauthorized, "unauthorized", "Unauthorized")
		return
	}
	profile, found, err := s.profiles.GetProfileByUserID(r.Context(), userID)
	if err != nil {
		slog.Error("profile lookup failed", "error", "get_me_failed")
		s.writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	if !found {
		s.writeError(w, http.StatusNotFound, "profile_not_found", "Profile not found")
		return
	}
	s.writeJSON(w, http.StatusOK, toMeProfile(profile, s.clock.Now()))
}

func (s *Server) handlePatchMe(w http.ResponseWriter, r *http.Request) {
	userID, email, ok := authenticatedUser(r)
	if !ok {
		s.writeError(w, http.StatusUnauthorized, "unauthorized", "Unauthorized")
		return
	}
	var req UpsertProfileRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", "Bad request")
		return
	}
	profile, err := s.profiles.UpsertProfile(r.Context(), userID, email, req, s.clock.Now())
	if err != nil {
		status, code, title := profileError(err)
		s.writeError(w, status, code, title)
		return
	}
	slog.Info("profile upserted", "user_id", userID, "handle", profile.Handle)
	s.writeJSON(w, http.StatusOK, toMeProfile(profile, s.clock.Now()))
}

func (s *Server) handleGetPublicProfile(w http.ResponseWriter, r *http.Request) {
	handle, err := ValidateHandle(r.PathValue("handle"))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_handle", "Invalid handle")
		return
	}
	profile, found, err := s.profiles.GetProfileByHandle(r.Context(), handle)
	if err != nil {
		slog.Error("profile lookup failed", "error", "get_public_profile_failed")
		s.writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	if !found {
		s.writeError(w, http.StatusNotFound, "profile_not_found", "Profile not found")
		return
	}
	s.writeJSON(w, http.StatusOK, toPublicProfile(profile))
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

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	var req StartEmailRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	email, err := normalizeEmail(req.Email)
	if err != nil {
		http.Error(w, "invalid email", http.StatusBadRequest)
		return
	}
	requestID, err := newID("req_")
	if err != nil {
		slog.Error("auth email start failed", "error", "request_id_generation_failed")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	code, err := newCode()
	if err != nil {
		slog.Error("auth email start failed", "error", "code_generation_failed")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	expiresAt := s.clock.Now().Add(15 * time.Minute)
	if err := s.store.CreateAuthRequest(r.Context(), AuthRequest{ID: requestID, Email: email, CodeHash: authCodeHash(s.pepper, code), ExpiresAt: expiresAt}); err != nil {
		slog.Error("auth email start failed", "error", "store_create_auth_request_failed")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if err := s.sender.SendAuthCode(r.Context(), email, requestID, code, expiresAt); err != nil {
		slog.Error("auth email start failed", "error", "email_send_failed")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	slog.Info("auth email code queued", "request_id", requestID)
	s.writeJSON(w, http.StatusAccepted, StartEmailResponse{RequestID: requestID, ExpiresAt: expiresAt})
}

func (s *Server) handleVerify(w http.ResponseWriter, r *http.Request) {
	var req VerifyEmailRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	now := s.clock.Now()
	ar, ok, err := s.store.ConsumeAuthRequest(r.Context(), req.RequestID, authCodeHash(s.pepper, req.Code), now)
	if err != nil {
		slog.Error("auth email verify failed", "error", "consume_request_failed")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	user, err := s.store.FindOrCreateUserByEmail(r.Context(), ar.Email)
	if err != nil {
		slog.Error("auth email verify failed", "error", "find_or_create_user_failed")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	sessionID, err := newID("ses_")
	if err != nil {
		slog.Error("auth email verify failed", "error", "session_id_generation_failed")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	refreshToken, err := newToken("rfr_")
	if err != nil {
		slog.Error("auth email verify failed", "error", "refresh_token_generation_failed")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	accessToken, err := newToken("act_")
	if err != nil {
		slog.Error("auth email verify failed", "error", "access_token_generation_failed")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	refreshExpires := s.clock.Now().Add(30 * 24 * time.Hour)
	accessExpires := s.clock.Now().Add(15 * time.Minute)
	if err := s.store.CreateSession(r.Context(), Session{ID: sessionID, UserID: user.ID, DeviceName: deviceNameOrDefault(req.DeviceName), RefreshTokenHash: codeHash(refreshToken), ExpiresAt: refreshExpires}); err != nil {
		slog.Error("auth email verify failed", "error", "create_session_failed")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	slog.Info("auth email verified", "request_id", ar.ID, "user_id", user.ID, "session_id", sessionID)
	s.writeJSON(w, http.StatusOK, VerifyEmailResponse{UserID: user.ID, SessionID: sessionID, AccessToken: accessToken, RefreshToken: refreshToken, AccessTokenExpiresAt: accessExpires, RefreshTokenExpiresAt: refreshExpires})
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	newRefresh, err := newToken("rfr_")
	if err != nil {
		slog.Error("session refresh failed", "error", "refresh_token_generation_failed")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	newAccess, err := newToken("act_")
	if err != nil {
		slog.Error("session refresh failed", "error", "access_token_generation_failed")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	refreshExpires := s.clock.Now().Add(30 * 24 * time.Hour)
	accessExpires := s.clock.Now().Add(15 * time.Minute)
	sess, ok, err := s.store.RotateSessionRefreshToken(r.Context(), codeHash(req.RefreshToken), codeHash(newRefresh), refreshExpires, s.clock.Now())
	if err != nil {
		slog.Error("session refresh failed", "error", "rotate_refresh_token_failed", "session_id", sess.ID)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	slog.Info("session refreshed", "session_id", sess.ID, "user_id", sess.UserID)
	s.writeJSON(w, http.StatusOK, VerifyEmailResponse{UserID: sess.UserID, SessionID: sess.ID, AccessToken: newAccess, RefreshToken: newRefresh, AccessTokenExpiresAt: accessExpires, RefreshTokenExpiresAt: refreshExpires})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	var req LogoutRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	revoked, err := s.store.RevokeSessionByRefreshTokenHash(r.Context(), codeHash(req.RefreshToken), s.clock.Now())
	if err != nil {
		slog.Error("session logout failed", "error", "revoke_session_failed")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	slog.Info("session logout processed", "revoked", revoked)
	w.WriteHeader(http.StatusOK)
}
