package clock

import (
	"testing"
	"time"
)

func TestNormalizeReturnsUTCServerTimestamp(t *testing.T) {
	local := time.Date(2026, 5, 25, 9, 15, 30, 123, time.FixedZone("test", -7*60*60))
	normalized := Normalize(local)

	if normalized.Location() != time.UTC {
		t.Fatalf("location = %v, want UTC", normalized.Location())
	}
	if !normalized.Equal(local) {
		t.Fatalf("instant changed: got %s want %s", normalized, local)
	}
}
