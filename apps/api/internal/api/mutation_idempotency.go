package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/primitives/uuidv7"
)

var errMutationConflict = errors.New("idempotency key conflicts with an earlier request")
var errMutationInProgress = errors.New("idempotent mutation is still in progress")

type mutationReplay struct {
	Status int
	Body   []byte
}

type mutationIdempotencyStore interface {
	BeginMutation(context.Context, string, string, []byte, time.Time) (*mutationReplay, error)
	CompleteMutation(context.Context, string, string, int, []byte, time.Time) error
	AbandonMutation(context.Context, string, string) error
}

func (s *PostgresStore) BeginMutation(ctx context.Context, userID, key string, fingerprint []byte, now time.Time) (*mutationReplay, error) {
	id, err := uuidv7.New()
	if err != nil {
		return nil, err
	}
	key = "http:" + key
	command, err := s.pool.Exec(ctx, `INSERT INTO idempotency_keys
		(id, user_id, key, request_hash, locked_until, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (user_id, key) DO NOTHING`,
		id, userID, key, fingerprint, now.Add(30*time.Second), now.Add(24*time.Hour))
	if err != nil {
		return nil, err
	}
	if command.RowsAffected() == 1 {
		return nil, nil
	}
	var storedHash, body []byte
	var status *int
	var lockedUntil *time.Time
	err = s.pool.QueryRow(ctx, `SELECT request_hash, response_status, coalesce(response_body, '{}'::jsonb)::text::bytea, locked_until
		FROM idempotency_keys WHERE user_id = $1 AND key = $2`, userID, key).Scan(&storedHash, &status, &body, &lockedUntil)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(storedHash, fingerprint) {
		return nil, errMutationConflict
	}
	if status != nil {
		return &mutationReplay{Status: *status, Body: body}, nil
	}
	if lockedUntil != nil && lockedUntil.After(now) {
		return nil, errMutationInProgress
	}
	command, err = s.pool.Exec(ctx, `UPDATE idempotency_keys SET locked_until = $3
		WHERE user_id = $1 AND key = $2 AND response_status IS NULL AND (locked_until IS NULL OR locked_until <= $4)`, userID, key, now.Add(30*time.Second), now)
	if err != nil {
		return nil, err
	}
	if command.RowsAffected() != 1 {
		return nil, errMutationInProgress
	}
	return nil, nil
}

func (s *PostgresStore) CompleteMutation(ctx context.Context, userID, key string, status int, body []byte, now time.Time) error {
	if len(bytes.TrimSpace(body)) == 0 {
		body = []byte("{}")
	}
	if !json.Valid(body) {
		return errors.New("idempotent response is not JSON")
	}
	_, err := s.pool.Exec(ctx, `UPDATE idempotency_keys SET response_status = $3, response_body = $4::jsonb,
		locked_until = NULL, expires_at = $5 WHERE user_id = $1 AND key = $2`, userID, "http:"+key, status, body, now.Add(24*time.Hour))
	return err
}

func (s *PostgresStore) AbandonMutation(ctx context.Context, userID, key string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM idempotency_keys WHERE user_id = $1 AND key = $2 AND response_status IS NULL`, userID, "http:"+key)
	return err
}

type bufferedResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (w *bufferedResponse) Header() http.Header { return w.header }
func (w *bufferedResponse) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *bufferedResponse) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(body)
}

func (h *Handler) enforceMutationIdempotency(w http.ResponseWriter, r *http.Request, next func(http.ResponseWriter, *http.Request)) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return false
	}
	if strings.HasPrefix(r.URL.Path, "/admin/") {
		return false
	}
	userID, _, ok := authenticatedUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return true
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" || len(key) > 200 {
		writeError(w, http.StatusBadRequest, "idempotency_key_required", "Idempotency-Key is required")
		return true
	}
	store, production := h.store.(mutationIdempotencyStore)
	if !production {
		next(w, r)
		return true
	}
	executeIdempotentMutation(w, r, userID, key, store, h.clock.Now(), next)
	return true
}

func executeIdempotentMutation(w http.ResponseWriter, r *http.Request, userID, key string, store mutationIdempotencyStore, now time.Time, next func(http.ResponseWriter, *http.Request)) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", "request body is too large")
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	digest := sha256.Sum256(append([]byte(r.Method+"\n"+r.URL.RequestURI()+"\n"), body...))
	replay, err := store.BeginMutation(r.Context(), userID, key, digest[:], now)
	if err != nil {
		if errors.Is(err, errMutationConflict) {
			writeError(w, http.StatusConflict, "idempotency_conflict", err.Error())
		} else if errors.Is(err, errMutationInProgress) {
			writeError(w, http.StatusConflict, "idempotency_in_progress", err.Error())
		} else {
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		}
		return
	}
	if replay != nil {
		if replay.Status != http.StatusNoContent {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(replay.Status)
		if replay.Status != http.StatusNoContent {
			_, _ = w.Write(replay.Body)
		}
		return
	}
	recorder := &bufferedResponse{header: make(http.Header)}
	next(recorder, r)
	status := recorder.status
	if status == 0 {
		status = http.StatusOK
	}
	if status >= 200 && status < 300 {
		if err = store.CompleteMutation(r.Context(), userID, key, status, recorder.body.Bytes(), now); err != nil {
			_ = store.AbandonMutation(r.Context(), userID, key)
			writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}
	} else {
		_ = store.AbandonMutation(r.Context(), userID, key)
	}
	for name, values := range recorder.header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}
	w.WriteHeader(status)
	_, _ = w.Write(recorder.body.Bytes())
}
