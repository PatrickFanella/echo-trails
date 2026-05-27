package uuidv7

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const encodedLength = 36

var (
	ErrInvalidUUID = errors.New("invalid uuid")
	ErrInvalidTime = errors.New("uuidv7 timestamp is out of range")
)

// UUID is an RFC 9562 UUID value. New IDs should be UUIDv7 so lexicographic
// byte/string ordering follows creation time at millisecond precision.
type UUID [16]byte

// New returns a UUIDv7 using the current server time and crypto randomness.
func New() (UUID, error) {
	return NewAt(time.Now(), rand.Reader)
}

// NewAt returns a UUIDv7 for t using random bytes from r. The timestamp is
// encoded as Unix milliseconds in the first 48 bits.
func NewAt(t time.Time, r io.Reader) (UUID, error) {
	ms := t.UTC().UnixMilli()
	if ms < 0 || ms > 0xffffffffffff {
		return UUID{}, ErrInvalidTime
	}

	var id UUID
	if _, err := io.ReadFull(r, id[:]); err != nil {
		return UUID{}, err
	}

	id[0] = byte(ms >> 40)
	id[1] = byte(ms >> 32)
	id[2] = byte(ms >> 24)
	id[3] = byte(ms >> 16)
	id[4] = byte(ms >> 8)
	id[5] = byte(ms)
	id[6] = (id[6] & 0x0f) | 0x70 // version 7
	id[8] = (id[8] & 0x3f) | 0x80 // RFC 4122/RFC 9562 variant

	return id, nil
}

// Parse validates and parses canonical UUID text.
func Parse(s string) (UUID, error) {
	if len(s) != encodedLength || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return UUID{}, ErrInvalidUUID
	}

	compact := strings.ReplaceAll(s, "-", "")
	if len(compact) != 32 {
		return UUID{}, ErrInvalidUUID
	}

	var id UUID
	if _, err := hex.Decode(id[:], []byte(compact)); err != nil {
		return UUID{}, fmt.Errorf("%w: %v", ErrInvalidUUID, err)
	}
	if id.Version() != 7 || id.Variant() != 2 {
		return UUID{}, ErrInvalidUUID
	}
	return id, nil
}

func (u UUID) String() string {
	var buf [encodedLength]byte
	hex.Encode(buf[0:8], u[0:4])
	buf[8] = '-'
	hex.Encode(buf[9:13], u[4:6])
	buf[13] = '-'
	hex.Encode(buf[14:18], u[6:8])
	buf[18] = '-'
	hex.Encode(buf[19:23], u[8:10])
	buf[23] = '-'
	hex.Encode(buf[24:36], u[10:16])
	return string(buf[:])
}

// Time returns the UUIDv7 timestamp in UTC at millisecond precision.
func (u UUID) Time() time.Time {
	ms := int64(u[0])<<40 | int64(u[1])<<32 | int64(u[2])<<24 | int64(u[3])<<16 | int64(u[4])<<8 | int64(u[5])
	return time.UnixMilli(ms).UTC()
}

func (u UUID) Version() byte { return u[6] >> 4 }

// Variant returns 2 for the RFC 4122/RFC 9562 variant.
func (u UUID) Variant() byte { return u[8] >> 6 }

func (u UUID) Before(other UUID) bool { return strings.Compare(u.String(), other.String()) < 0 }
