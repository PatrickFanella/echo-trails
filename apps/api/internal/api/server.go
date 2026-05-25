package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

type Config struct {
	Store      Store
	Sender     EmailSender
	Clock      clock
	CodePepper []byte
}

type Server struct {
	store  Store
	sender EmailSender
	clock  clock
	pepper []byte
	mux    *http.ServeMux
}

func NewServer(cfg Config) *Server {
	if cfg.Store == nil {
		cfg.Store = NewMemoryStore()
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
	s := &Server{store: cfg.Store, sender: cfg.Sender, clock: cfg.Clock, pepper: cfg.CodePepper, mux: http.NewServeMux()}
	s.routes()
	return s
}
func (s *Server) Router() http.Handler { return s.mux }

func (s *Server) routes() {
	s.mux.HandleFunc("POST /auth/email/start", s.handleStart)
	s.mux.HandleFunc("POST /auth/email/verify", s.handleVerify)
	s.mux.HandleFunc("POST /sessions/refresh", s.handleRefresh)
	s.mux.HandleFunc("POST /sessions/logout", s.handleLogout)
}

func (s *Server) writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
func decodeJSON(r *http.Request, v any) error { return json.NewDecoder(r.Body).Decode(v) }

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
