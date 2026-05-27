package uuidv7

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestUUIDv7OrderingFollowsTimestamp(t *testing.T) {
	base := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	rng := bytes.NewReader(bytes.Repeat([]byte{0x42}, 64))

	first, err := NewAt(base, rng)
	if err != nil {
		t.Fatalf("NewAt first: %v", err)
	}
	second, err := NewAt(base.Add(time.Millisecond), rng)
	if err != nil {
		t.Fatalf("NewAt second: %v", err)
	}

	if !first.Before(second) {
		t.Fatalf("expected %s to sort before %s", first, second)
	}
	if first.Time() != base {
		t.Fatalf("timestamp mismatch: got %s want %s", first.Time(), base)
	}
	if first.Version() != 7 || first.Variant() != 2 {
		t.Fatalf("unexpected version/variant: v=%d variant=%d", first.Version(), first.Variant())
	}
}

func TestParseValidatesUUIDv7Shape(t *testing.T) {
	id, err := NewAt(time.UnixMilli(1_779_719_200_123), strings.NewReader(strings.Repeat("a", 16)))
	if err != nil {
		t.Fatalf("NewAt: %v", err)
	}

	parsed, err := Parse(id.String())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if parsed != id {
		t.Fatalf("parse mismatch: got %s want %s", parsed, id)
	}
	if _, err := Parse("not-a-uuid"); err == nil {
		t.Fatal("expected invalid UUID error")
	}
}
