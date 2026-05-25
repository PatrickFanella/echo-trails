package clock

import "time"

// Now returns the authoritative server timestamp shape used by API primitives.
// It is always UTC and strips the monotonic clock reading so it is stable for
// serialization and persistence boundaries.
func Now() time.Time {
	return Normalize(time.Now())
}

// Normalize converts any timestamp to the canonical server timestamp form.
func Normalize(t time.Time) time.Time {
	return t.UTC().Round(0)
}
