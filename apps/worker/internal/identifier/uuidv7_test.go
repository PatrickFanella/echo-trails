package identifier

import (
	"bytes"
	"testing"
	"time"
)

func TestUUIDv7EncodesVersionVariantAndTimeOrder(t *testing.T) {
	randomness := bytes.NewReader(make([]byte, 32))
	first, err := newUUIDv7At(time.UnixMilli(1_700_000_000_000), randomness)
	if err != nil {
		t.Fatal(err)
	}
	second, err := newUUIDv7At(time.UnixMilli(1_700_000_000_001), randomness)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 36 || first[14] != '7' || (first[19] != '8' && first[19] != '9' && first[19] != 'a' && first[19] != 'b') {
		t.Fatalf("invalid UUIDv7 %q", first)
	}
	if first >= second {
		t.Fatalf("UUIDv7 time order is not monotonic: %s >= %s", first, second)
	}
}
