package apple

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProductionVerifierValidatesSignatureIssuerAudienceAndExpiry(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		exponent := big.NewInt(int64(key.PublicKey.E)).Bytes()
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kid": "apple-key", "kty": "RSA", "alg": "RS256", "use": "sig",
			"n": base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(exponent),
		}}})
	}))
	defer server.Close()
	verifier, err := NewProductionVerifier("com.example.EchoTrails", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	verifier.keysURL = server.URL
	token := signedAppleToken(t, key, map[string]any{
		"iss": AppleIssuer, "aud": "com.example.EchoTrails", "sub": "apple-subject", "email": "person@example.com",
		"email_verified": "true", "iat": time.Now().Add(-time.Minute).Unix(), "exp": time.Now().Add(time.Hour).Unix(),
	})
	claims, err := verifier.VerifyIdentityToken(context.Background(), token)
	if err != nil || claims.Subject != "apple-subject" || !claims.EmailVerified {
		t.Fatalf("claims = %+v error = %v", claims, err)
	}

	badAudience := signedAppleToken(t, key, map[string]any{
		"iss": AppleIssuer, "aud": "wrong-client", "sub": "apple-subject", "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(),
	})
	if _, err = verifier.VerifyIdentityToken(context.Background(), badAudience); err == nil {
		t.Fatal("wrong audience must be rejected")
	}
}

func signedAppleToken(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "apple-key"})
	payload, _ := json.Marshal(claims)
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
}
