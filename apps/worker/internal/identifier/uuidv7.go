package identifier

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"time"
)

// NewUUIDv7 returns an RFC 9562 UUIDv7 using the current UTC millisecond and
// cryptographic randomness. Worker-created rows use the same sortable ID
// invariant as API-created rows.
func NewUUIDv7() (string, error) {
	return newUUIDv7At(time.Now().UTC(), rand.Reader)
}

func newUUIDv7At(now time.Time, randomness io.Reader) (string, error) {
	var value [16]byte
	if _, err := io.ReadFull(randomness, value[:]); err != nil {
		return "", err
	}
	milliseconds := now.UTC().UnixMilli()
	value[0] = byte(milliseconds >> 40)
	value[1] = byte(milliseconds >> 32)
	value[2] = byte(milliseconds >> 24)
	value[3] = byte(milliseconds >> 16)
	value[4] = byte(milliseconds >> 8)
	value[5] = byte(milliseconds)
	value[6] = (value[6] & 0x0f) | 0x70
	value[8] = (value[8] & 0x3f) | 0x80
	var encoded [36]byte
	hex.Encode(encoded[0:8], value[0:4])
	encoded[8] = '-'
	hex.Encode(encoded[9:13], value[4:6])
	encoded[13] = '-'
	hex.Encode(encoded[14:18], value[6:8])
	encoded[18] = '-'
	hex.Encode(encoded[19:23], value[8:10])
	encoded[23] = '-'
	hex.Encode(encoded[24:36], value[10:16])
	return string(encoded[:]), nil
}
