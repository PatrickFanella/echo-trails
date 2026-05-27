package pagination

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/primitives/clock"
	"git.subcult.tv/subculture-collective/echo-trails/apps/api/internal/primitives/uuidv7"
)

const cursorVersion = 1

var ErrInvalidCursor = errors.New("invalid cursor")

// Cursor is the opaque cursor payload used for feed/search/list pagination.
// It uses the server timestamp plus UUID tie-breaker for deterministic order.
type Cursor struct {
	After time.Time
	ID    uuidv7.UUID
}

type wireCursor struct {
	Version int    `json:"v"`
	After   string `json:"after"`
	ID      string `json:"id"`
}

func Encode(c Cursor) (string, error) {
	payload := wireCursor{
		Version: cursorVersion,
		After:   clock.Normalize(c.After).Format(time.RFC3339Nano),
		ID:      c.ID.String(),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func Decode(value string) (Cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return Cursor{}, fmt.Errorf("%w: %v", ErrInvalidCursor, err)
	}

	var payload wireCursor
	if err := json.Unmarshal(raw, &payload); err != nil {
		return Cursor{}, fmt.Errorf("%w: %v", ErrInvalidCursor, err)
	}
	if payload.Version != cursorVersion || payload.After == "" || payload.ID == "" {
		return Cursor{}, ErrInvalidCursor
	}

	after, err := time.Parse(time.RFC3339Nano, payload.After)
	if err != nil {
		return Cursor{}, fmt.Errorf("%w: %v", ErrInvalidCursor, err)
	}
	id, err := uuidv7.Parse(payload.ID)
	if err != nil {
		return Cursor{}, fmt.Errorf("%w: %v", ErrInvalidCursor, err)
	}

	return Cursor{After: clock.Normalize(after), ID: id}, nil
}
