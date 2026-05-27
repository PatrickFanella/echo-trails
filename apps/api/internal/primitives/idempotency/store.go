package idempotency

import (
	"context"
	"errors"
	"sync"
	"time"

	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/primitives/clock"
)

var (
	ErrMissingKey          = errors.New("missing idempotency key")
	ErrFingerprintMismatch = errors.New("idempotency key reused with different request")
	ErrRequestInProgress   = errors.New("idempotent request is already in progress")
)

type Result struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
}

type record struct {
	fingerprint string
	completed   bool
	result      Result
	createdAt   time.Time
	updatedAt   time.Time
}

// MemoryStore is a deterministic in-memory implementation for API tests and
// local wiring. A later DB-backed store should preserve these semantics.
type MemoryStore struct {
	mu      sync.Mutex
	records map[string]record
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{records: make(map[string]record)}
}

// Do executes fn once for a key/fingerprint pair. Completed results are
// returned unchanged on replay; mismatched fingerprints are rejected.
func (s *MemoryStore) Do(ctx context.Context, key, fingerprint string, fn func(context.Context) (Result, error)) (Result, bool, error) {
	if key == "" {
		return Result{}, false, ErrMissingKey
	}

	now := clock.Now()
	s.mu.Lock()
	if existing, ok := s.records[key]; ok {
		if existing.fingerprint != fingerprint {
			s.mu.Unlock()
			return Result{}, false, ErrFingerprintMismatch
		}
		if existing.completed {
			result := cloneResult(existing.result)
			s.mu.Unlock()
			return result, true, nil
		}
		s.mu.Unlock()
		return Result{}, false, ErrRequestInProgress
	}
	s.records[key] = record{fingerprint: fingerprint, createdAt: now, updatedAt: now}
	s.mu.Unlock()

	result, err := fn(ctx)
	if err != nil {
		s.mu.Lock()
		delete(s.records, key)
		s.mu.Unlock()
		return Result{}, false, err
	}

	s.mu.Lock()
	s.records[key] = record{fingerprint: fingerprint, completed: true, result: cloneResult(result), createdAt: now, updatedAt: clock.Now()}
	s.mu.Unlock()

	return cloneResult(result), false, nil
}

func cloneResult(in Result) Result {
	out := Result{StatusCode: in.StatusCode}
	if in.Headers != nil {
		out.Headers = make(map[string]string, len(in.Headers))
		for k, v := range in.Headers {
			out.Headers[k] = v
		}
	}
	if in.Body != nil {
		out.Body = append([]byte(nil), in.Body...)
	}
	return out
}
