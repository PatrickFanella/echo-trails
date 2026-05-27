package pagination

import (
	"strings"
	"testing"
	"time"

	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/primitives/uuidv7"
)

func TestCursorEncodeDecodeRoundTrip(t *testing.T) {
	after := time.Date(2026, 5, 25, 12, 30, 45, 123456789, time.FixedZone("test", -7*60*60))
	id, err := uuidv7.NewAt(after, strings.NewReader(strings.Repeat("z", 16)))
	if err != nil {
		t.Fatalf("NewAt: %v", err)
	}

	encoded, err := Encode(Cursor{After: after, ID: id})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if !decoded.After.Equal(after.UTC()) {
		t.Fatalf("after mismatch: got %s want %s", decoded.After, after.UTC())
	}
	if decoded.ID != id {
		t.Fatalf("id mismatch: got %s want %s", decoded.ID, id)
	}
}

func TestCursorDecodeRejectsMalformedInput(t *testing.T) {
	if _, err := Decode("%%%not-base64%%%"); err == nil {
		t.Fatal("expected malformed cursor error")
	}
}
