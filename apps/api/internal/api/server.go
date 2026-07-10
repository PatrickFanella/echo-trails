package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	appleauth "git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/auth/apple"
	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/primitives/pagination"
	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/primitives/uuidv7"
)

type Config struct {
	Store      Store
	Profiles   ProfileStore
	Sender     EmailSender
	Clock      clock
	CodePepper []byte
	Apple      *appleauth.Service
}

type Server struct {
	store    Store
	profiles ProfileStore
	sender   EmailSender
	clock    clock
	pepper   []byte
	apple    *appleauth.Service
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
	s := &Server{store: cfg.Store, profiles: cfg.Profiles, sender: cfg.Sender, clock: cfg.Clock, pepper: cfg.CodePepper, apple: cfg.Apple, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) Router() http.Handler { return s.mux }

func (s *Server) routes() {
	s.mux.HandleFunc("POST /auth/email/start", s.handleStart)
	s.mux.HandleFunc("POST /auth/email/verify", s.handleVerify)
	s.mux.HandleFunc("POST /auth/apple", s.handleApple)
	s.mux.HandleFunc("POST /sessions/refresh", s.handleRefresh)
	s.mux.HandleFunc("POST /sessions/logout", s.handleLogout)
	s.mux.HandleFunc("GET /me", s.handleGetMe)
	s.mux.HandleFunc("PATCH /me", s.handlePatchMe)
	s.mux.HandleFunc("GET /users/{handle}", s.handleGetPublicProfile)
	s.mux.HandleFunc("GET /users/{handle}/echoes", s.handleGetPublicProfileEchoes)
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
	userID, _, ok := authenticatedUser(r)
	if !ok {
		s.writeError(w, http.StatusUnauthorized, "unauthorized", "Unauthorized")
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" || len(idempotencyKey) > 200 {
		s.writeError(w, http.StatusBadRequest, "idempotency_key_required", "Idempotency-Key is required")
		return
	}
	if store, production := s.store.(mutationIdempotencyStore); production {
		executeIdempotentMutation(w, r, userID, idempotencyKey, store, s.clock.Now(), s.handlePatchMeCore)
		return
	}
	s.handlePatchMeCore(w, r)
}

func (s *Server) handlePatchMeCore(w http.ResponseWriter, r *http.Request) {
	userID, email, _ := authenticatedUser(r)
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

type publicProfileEchoRepository interface {
	ListVisiblePublishedByAuthorHandlePage(context.Context, string, string, string, int) (EchoRecordPage, error)
}

func (s *Server) handleGetPublicProfileEchoes(w http.ResponseWriter, r *http.Request) {
	viewerID, _, _ := authenticatedUser(r)
	handle, err := ValidateHandle(r.PathValue("handle"))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_handle", "Invalid handle")
		return
	}
	if _, found, profileErr := s.profiles.GetProfileByHandle(r.Context(), handle); profileErr != nil {
		s.writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	} else if !found {
		s.writeError(w, http.StatusNotFound, "profile_not_found", "Profile not found")
		return
	}
	repository, ok := s.store.(publicProfileEchoRepository)
	if !ok {
		s.writeJSON(w, http.StatusOK, map[string]any{"echoes": []EchoResponse{}, "next_cursor": ""})
		return
	}
	page, err := repository.ListVisiblePublishedByAuthorHandlePage(r.Context(), handle, viewerID, r.URL.Query().Get("cursor"), parseInt(r.URL.Query().Get("limit"), 20))
	if err != nil {
		if errors.Is(err, pagination.ErrInvalidCursor) {
			s.writeError(w, http.StatusBadRequest, "invalid_cursor", "Invalid pagination cursor")
		} else {
			s.writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		}
		return
	}
	echoes := make([]EchoResponse, 0, len(page.Records))
	for _, record := range page.Records {
		echoes = append(echoes, toEchoResponse(record, ""))
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"echoes": echoes, "next_cursor": page.NextCursor})
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
		s.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid email authentication request")
		return
	}
	email, err := normalizeEmail(req.Email)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_email", "Invalid email address")
		return
	}
	requestUUID, err := uuidv7.New()
	if err != nil {
		slog.Error("auth email start failed", "error", "request_id_generation_failed")
		s.writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	requestID := requestUUID.String()
	code, err := newCode()
	if err != nil {
		slog.Error("auth email start failed", "error", "code_generation_failed")
		s.writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	expiresAt := s.clock.Now().Add(15 * time.Minute)
	if err := s.store.CreateAuthRequest(r.Context(), AuthRequest{ID: requestID, Email: email, CodeHash: authCodeHash(s.pepper, code), ExpiresAt: expiresAt}); err != nil {
		slog.Error("auth email start failed", "error", "store_create_auth_request_failed")
		s.writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	if err := s.sender.SendAuthCode(r.Context(), email, requestID, code, expiresAt); err != nil {
		slog.Error("auth email start failed", "error", "email_send_failed")
		s.writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	slog.Info("auth email code queued", "request_id", requestID)
	s.writeJSON(w, http.StatusAccepted, StartEmailResponse{RequestID: requestID, ExpiresAt: expiresAt})
}

func (s *Server) handleVerify(w http.ResponseWriter, r *http.Request) {
	var req VerifyEmailRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid email verification request")
		return
	}
	now := s.clock.Now()
	ar, ok, err := s.store.ConsumeAuthRequest(r.Context(), req.RequestID, authCodeHash(s.pepper, req.Code), now)
	if err != nil {
		slog.Error("auth email verify failed", "error", "consume_request_failed")
		s.writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	if !ok {
		s.writeError(w, http.StatusUnauthorized, "invalid_email_code", "Email code is invalid or expired")
		return
	}
	user, err := s.store.FindOrCreateUserByEmail(r.Context(), ar.Email)
	if err != nil {
		slog.Error("auth email verify failed", "error", "find_or_create_user_failed")
		s.writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	response, err := s.createSessionResponse(r.Context(), user.ID, req.DeviceName)
	if err != nil {
		slog.Error("auth email verify failed", "error", "create_session_failed")
		s.writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	slog.Info("auth email verified", "request_id", ar.ID, "user_id", user.ID, "session_id", response.SessionID)
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleApple(w http.ResponseWriter, r *http.Request) {
	if s.apple == nil {
		s.writeError(w, http.StatusServiceUnavailable, "apple_auth_unavailable", "Sign in with Apple is unavailable")
		return
	}
	var request struct {
		IdentityToken string `json:"identity_token"`
		DeviceName    string `json:"device_name,omitempty"`
	}
	if decodeJSON(r, &request) != nil || strings.TrimSpace(request.IdentityToken) == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_apple_token", "Invalid Apple identity token")
		return
	}
	result, err := s.apple.SignIn(r.Context(), request.IdentityToken)
	if err != nil {
		s.writeError(w, http.StatusUnauthorized, "invalid_apple_token", "Invalid Apple identity token")
		return
	}
	response, err := s.createSessionResponse(r.Context(), result.User.ID, request.DeviceName)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) createSessionResponse(ctx context.Context, userID, deviceName string) (VerifyEmailResponse, error) {
	sessionUUID, err := uuidv7.New()
	if err != nil {
		return VerifyEmailResponse{}, err
	}
	refreshToken, err := newToken("rfr_")
	if err != nil {
		return VerifyEmailResponse{}, err
	}
	accessToken, err := newToken("act_")
	if err != nil {
		return VerifyEmailResponse{}, err
	}
	now := s.clock.Now()
	refreshExpires := now.Add(30 * 24 * time.Hour)
	accessExpires := now.Add(15 * time.Minute)
	sessionID := sessionUUID.String()
	if err = s.store.CreateSession(ctx, Session{ID: sessionID, UserID: userID, DeviceName: deviceNameOrDefault(deviceName), AccessTokenHash: codeHash(accessToken), AccessExpiresAt: accessExpires, RefreshTokenHash: codeHash(refreshToken), ExpiresAt: refreshExpires}); err != nil {
		return VerifyEmailResponse{}, err
	}
	return VerifyEmailResponse{UserID: userID, SessionID: sessionID, AccessToken: accessToken, RefreshToken: refreshToken, AccessTokenExpiresAt: accessExpires, RefreshTokenExpiresAt: refreshExpires}, nil
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" || len(idempotencyKey) > 200 {
		s.writeError(w, http.StatusBadRequest, "idempotency_key_required", "Idempotency-Key is required")
		return
	}
	var req RefreshRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid session refresh request")
		return
	}
	material := codeHash(req.RefreshToken) + ":" + idempotencyKey
	newRefresh := deriveSessionToken("rfr_", s.pepper, "refresh:"+material)
	newAccess := deriveSessionToken("act_", s.pepper, "access:"+material)
	refreshExpires := s.clock.Now().Add(30 * 24 * time.Hour)
	accessExpires := s.clock.Now().Add(15 * time.Minute)
	refreshStore, supportsReplay := s.store.(IdempotentRefreshStore)
	var sess Session
	var ok bool
	var err error
	if supportsReplay {
		sess, ok, err = refreshStore.RotateSessionRefreshTokenIdempotent(r.Context(), codeHash(req.RefreshToken), idempotencyKey, codeHash(newRefresh), codeHash(newAccess), refreshExpires, accessExpires, s.clock.Now())
	} else {
		sess, ok, err = s.store.RotateSessionRefreshToken(r.Context(), codeHash(req.RefreshToken), codeHash(newRefresh), codeHash(newAccess), refreshExpires, accessExpires, s.clock.Now())
	}
	if err != nil {
		slog.Error("session refresh failed", "error", "rotate_refresh_token_failed", "session_id", sess.ID)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	if !ok {
		s.writeError(w, http.StatusUnauthorized, "invalid_refresh_token", "Refresh token is invalid or expired")
		return
	}
	slog.Info("session refreshed", "session_id", sess.ID, "user_id", sess.UserID)
	s.writeJSON(w, http.StatusOK, VerifyEmailResponse{UserID: sess.UserID, SessionID: sess.ID, AccessToken: newAccess, RefreshToken: newRefresh, AccessTokenExpiresAt: accessExpires, RefreshTokenExpiresAt: refreshExpires})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.Header.Get("Idempotency-Key")) == "" {
		s.writeError(w, http.StatusBadRequest, "idempotency_key_required", "Idempotency-Key is required")
		return
	}
	var req LogoutRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid session logout request")
		return
	}
	revoked, err := s.store.RevokeSessionByRefreshTokenHash(r.Context(), codeHash(req.RefreshToken), s.clock.Now())
	if err != nil {
		slog.Error("session logout failed", "error", "revoke_session_failed")
		s.writeError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
		return
	}
	slog.Info("session logout processed", "revoked", revoked)
	w.WriteHeader(http.StatusNoContent)
}

func deriveSessionToken(prefix string, key []byte, material string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(material))
	return prefix + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
