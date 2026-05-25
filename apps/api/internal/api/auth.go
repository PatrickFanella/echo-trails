package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/mail"
	"strings"
	"time"
)

type EmailSender interface {
	SendAuthCode(ctx context.Context, email, requestID, code string, expiresAt time.Time) error
}

type DevEmailSender struct{ Log func(string, ...any) }

func (d DevEmailSender) SendAuthCode(_ context.Context, email, requestID, code string, expiresAt time.Time) error {
	if d.Log != nil {
		d.Log("dev auth code email=%s request_id=%s code=%s expires_at=%s", email, requestID, code, expiresAt.UTC().Format(time.RFC3339))
	}
	return nil
}

type AuthRequest struct {
	ID        string
	Email     string
	CodeHash  string
	Attempts  int
	ExpiresAt time.Time
	UsedAt    *time.Time
}

type Session struct {
	ID               string
	UserID           string
	DeviceName       string
	RefreshTokenHash string
	ExpiresAt        time.Time
	RevokedAt        *time.Time
}

type User struct{ ID, Email string }

type Store interface {
	CreateAuthRequest(ctx context.Context, req AuthRequest) error
	GetAuthRequest(ctx context.Context, id string) (AuthRequest, bool)
	ConsumeAuthRequest(ctx context.Context, id string, submittedHash string, now time.Time) (AuthRequest, bool, error)
	FindOrCreateUserByEmail(ctx context.Context, email string) (User, error)
	CreateSession(ctx context.Context, s Session) error
	RotateSessionRefreshToken(ctx context.Context, currentHash string, newHash string, newExpiresAt time.Time, now time.Time) (Session, bool, error)
	RevokeSessionByRefreshTokenHash(ctx context.Context, hash string, revokedAt time.Time) (bool, error)
}

type clock interface{ Now() time.Time }
type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

func normalizeEmail(input string) (string, error) {
	addr, err := mail.ParseAddress(strings.TrimSpace(input))
	if err != nil {
		return "", err
	}
	email := strings.ToLower(strings.TrimSpace(addr.Address))
	if email == "" || !strings.Contains(email, "@") {
		return "", errors.New("invalid email")
	}
	return email, nil
}

func randToken(prefix string, n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(b), nil
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

func codeHash(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

func authCodeHash(pepper []byte, code string) string {
	mac := hmac.New(sha256.New, pepper)
	_, _ = mac.Write([]byte(code))
	return hex.EncodeToString(mac.Sum(nil))
}

func constantTimeEqualHex(a, b string) bool { return subtleConstantTimeCompare([]byte(a), []byte(b)) }

func subtleConstantTimeCompare(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

func newCode() (string, error)               { return randToken("", 3) }
func newID(prefix string) (string, error)    { return randToken(prefix, 16) }
func newToken(prefix string) (string, error) { return randToken(prefix, 32) }

func verifyCode(expectedHash, code string) bool {
	return constantTimeEqualHex(expectedHash, codeHash(code))
}

func deviceNameOrDefault(name string) string {
	if strings.TrimSpace(name) == "" {
		return "unknown device"
	}
	return strings.TrimSpace(name)
}
