package idempotency

import (
	"context"
	"errors"
	"testing"
)

func TestMemoryStoreReplaysCompletedResult(t *testing.T) {
	store := NewMemoryStore()
	calls := 0
	fn := func(context.Context) (Result, error) {
		calls++
		return Result{StatusCode: 201, Headers: map[string]string{"content-type": "application/json"}, Body: []byte(`{"id":"echo_1"}`)}, nil
	}

	first, replayed, err := store.Do(context.Background(), "key-1", "POST:/echoes:hash", fn)
	if err != nil {
		t.Fatalf("first Do: %v", err)
	}
	if replayed {
		t.Fatal("first call should not be replay")
	}

	second, replayed, err := store.Do(context.Background(), "key-1", "POST:/echoes:hash", fn)
	if err != nil {
		t.Fatalf("second Do: %v", err)
	}
	if !replayed {
		t.Fatal("second call should replay")
	}
	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1", calls)
	}
	if first.StatusCode != second.StatusCode || string(first.Body) != string(second.Body) || first.Headers["content-type"] != second.Headers["content-type"] {
		t.Fatalf("replayed result mismatch: first=%+v second=%+v", first, second)
	}
}

func TestMemoryStoreRejectsMismatchedFingerprint(t *testing.T) {
	store := NewMemoryStore()
	_, _, err := store.Do(context.Background(), "key-1", "fingerprint-a", func(context.Context) (Result, error) {
		return Result{StatusCode: 200}, nil
	})
	if err != nil {
		t.Fatalf("seed Do: %v", err)
	}

	_, _, err = store.Do(context.Background(), "key-1", "fingerprint-b", func(context.Context) (Result, error) {
		return Result{StatusCode: 200}, nil
	})
	if !errors.Is(err, ErrFingerprintMismatch) {
		t.Fatalf("got %v, want ErrFingerprintMismatch", err)
	}
}
