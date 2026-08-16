package apple

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

const AppleIssuer = "https://appleid.apple.com"
const AppleJWKSURL = "https://appleid.apple.com/auth/keys"

type ProductionVerifier struct {
	client   *http.Client
	clientID string
	keysURL  string
	mu       sync.Mutex
	keys     map[string]*rsa.PublicKey
	keysAt   time.Time
}

func NewProductionVerifier(clientID string, client *http.Client) (*ProductionVerifier, error) {
	if strings.TrimSpace(clientID) == "" {
		return nil, errors.New("Apple client ID is required")
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &ProductionVerifier{client: client, clientID: strings.TrimSpace(clientID), keysURL: AppleJWKSURL}, nil
}

func (v *ProductionVerifier) VerifyIdentityToken(ctx context.Context, identityToken string) (Claims, error) {
	parts := strings.Split(strings.TrimSpace(identityToken), ".")
	if len(parts) != 3 {
		return Claims{}, ErrInvalidIdentityToken
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Claims{}, ErrInvalidIdentityToken
	}
	var header struct{ Algorithm, KeyID string }
	var rawHeader map[string]any
	if json.Unmarshal(headerBytes, &rawHeader) != nil {
		return Claims{}, ErrInvalidIdentityToken
	}
	header.Algorithm, _ = rawHeader["alg"].(string)
	header.KeyID, _ = rawHeader["kid"].(string)
	if header.Algorithm != "RS256" || header.KeyID == "" {
		return Claims{}, ErrInvalidIdentityToken
	}
	key, err := v.key(ctx, header.KeyID)
	if err != nil {
		return Claims{}, ErrInvalidIdentityToken
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Claims{}, ErrInvalidIdentityToken
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature) != nil {
		return Claims{}, ErrInvalidIdentityToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, ErrInvalidIdentityToken
	}
	var tokenClaims struct {
		Issuer        string          `json:"iss"`
		Audience      json.RawMessage `json:"aud"`
		Subject       string          `json:"sub"`
		Email         string          `json:"email"`
		EmailVerified any             `json:"email_verified"`
		ExpiresAt     int64           `json:"exp"`
		IssuedAt      int64           `json:"iat"`
	}
	if json.Unmarshal(payload, &tokenClaims) != nil || tokenClaims.Issuer != AppleIssuer || tokenClaims.Subject == "" || !audienceContains(tokenClaims.Audience, v.clientID) {
		return Claims{}, ErrInvalidIdentityToken
	}
	now := time.Now().UTC().Unix()
	if tokenClaims.ExpiresAt <= now || tokenClaims.IssuedAt > now+60 {
		return Claims{}, ErrInvalidIdentityToken
	}
	verified := tokenClaims.EmailVerified == true || tokenClaims.EmailVerified == "true"
	return Claims{Subject: tokenClaims.Subject, Email: tokenClaims.Email, EmailVerified: verified}, nil
}

func (v *ProductionVerifier) key(ctx context.Context, keyID string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if key := v.keys[keyID]; key != nil && time.Since(v.keysAt) < 24*time.Hour {
		return key, nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, v.keysURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := v.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, ErrInvalidIdentityToken
	}
	var document struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			Alg string `json:"alg"`
			Use string `json:"use"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if json.NewDecoder(response.Body).Decode(&document) != nil {
		return nil, ErrInvalidIdentityToken
	}
	keys := map[string]*rsa.PublicKey{}
	for _, item := range document.Keys {
		key, parseErr := parseRSAKey(item.N, item.E)
		if parseErr == nil {
			keys[item.Kid] = key
		}
	}
	v.keys, v.keysAt = keys, time.Now()
	key := keys[keyID]
	if key == nil {
		return nil, ErrInvalidIdentityToken
	}
	return key, nil
}

func parseRSAKey(modulus, exponent string) (*rsa.PublicKey, error) {
	n, err := base64.RawURLEncoding.DecodeString(modulus)
	if err != nil {
		return nil, err
	}
	e, err := base64.RawURLEncoding.DecodeString(exponent)
	if err != nil || len(e) == 0 || len(e) > 4 {
		return nil, ErrInvalidIdentityToken
	}
	var padded [4]byte
	copy(padded[4-len(e):], e)
	return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(binary.BigEndian.Uint32(padded[:]))}, nil
}

func audienceContains(raw json.RawMessage, clientID string) bool {
	var single string
	if json.Unmarshal(raw, &single) == nil {
		return single == clientID
	}
	var multiple []string
	if json.Unmarshal(raw, &multiple) != nil {
		return false
	}
	for _, audience := range multiple {
		if audience == clientID {
			return true
		}
	}
	return false
}
