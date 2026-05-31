package api

import "testing"

func TestSafePathRedactsSensitivePathSegments(t *testing.T) {
	for _, path := range []string{"/auth/token/abc", "/places/near/lat/lon", "/secret/code"} {
		if got := safePath(path); got != "[redacted]" {
			t.Fatalf("safePath(%q) = %q", path, got)
		}
	}
	if got := safePath("/places/search"); got != "/places/search" {
		t.Fatalf("safe public path redacted: %q", got)
	}
}
