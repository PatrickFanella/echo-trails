package locationproof

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestValidateCreationTokenRequestRejectsStaleProof(t *testing.T) {
	req, cfg := validRequest()
	req.Proof.CapturedAt = cfg.Now.Add(-DefaultMaxProofAge - time.Second)

	_, err := ValidateCreationTokenRequest(req, cfg)
	if !errors.Is(err, ErrStaleProof) {
		t.Fatalf("error = %v, want ErrStaleProof", err)
	}
}

func TestValidateCreationTokenRequestRejectsPoorAccuracy(t *testing.T) {
	req, cfg := validRequest()
	req.Proof.AccuracyMeters = DefaultMaxAccuracy + 0.1

	_, err := ValidateCreationTokenRequest(req, cfg)
	if !errors.Is(err, ErrPoorAccuracy) {
		t.Fatalf("error = %v, want ErrPoorAccuracy", err)
	}
}

func TestValidateCreationTokenRequestRejectsOutsideBoundary(t *testing.T) {
	req, cfg := validRequest()
	req.Proof.Coordinate = Coordinate{Latitude: 45.518, Longitude: -122.681}

	_, err := ValidateCreationTokenRequest(req, cfg)
	if !errors.Is(err, ErrOutsideBoundary) {
		t.Fatalf("error = %v, want ErrOutsideBoundary", err)
	}
}

func TestValidateCreationTokenRequestRejectsRestrictedPlace(t *testing.T) {
	req, cfg := validRequest()
	req.Places[0].Status = PlaceRestricted

	_, err := ValidateCreationTokenRequest(req, cfg)
	if !errors.Is(err, ErrRestrictedPlace) {
		t.Fatalf("error = %v, want ErrRestrictedPlace", err)
	}
}

func TestValidateCreationTokenRequestUsesExplicitSelectionWhenPlacesOverlap(t *testing.T) {
	req, cfg := validRequest()
	req.Places = append(req.Places, Place{
		ID: "place_neighbor", Status: PlaceActive,
		Center: Coordinate{Latitude: 45.51525, Longitude: -122.67835}, RadiusMeters: 50,
	})

	token, err := ValidateCreationTokenRequest(req, cfg)
	if err != nil || token.PlaceID != req.SelectedPlace {
		t.Fatalf("token = %+v error = %v", token, err)
	}
}

func TestValidateCreationTokenRequestReturnsSuccessToken(t *testing.T) {
	req, cfg := validRequest()

	token, err := ValidateCreationTokenRequest(req, cfg)
	if err != nil {
		t.Fatalf("ValidateCreationTokenRequest: %v", err)
	}
	if token.PlaceID != req.SelectedPlace || token.UserID != req.UserID {
		t.Fatalf("token binding mismatch: %+v", token)
	}
	if !strings.HasPrefix(token.Token, "crt_") {
		t.Fatalf("token prefix = %q, want crt_", token.Token)
	}
	if strings.Contains(token.Token, "45.515") || strings.Contains(token.Token, "122.678") {
		t.Fatalf("token must not expose exact private coordinates: %s", token.Token)
	}
	if !token.ExpiresAt.After(token.IssuedAt) {
		t.Fatalf("token expiration must be after issue time: %+v", token)
	}
}

func TestVerifyCreationTokenChecksSignatureBindingAndExpiration(t *testing.T) {
	req, cfg := validRequest()
	token, err := ValidateCreationTokenRequest(req, cfg)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := VerifyCreationToken(token.Token, req.UserID, req.SelectedPlace, cfg.Now, cfg.SigningKey)
	if err != nil || verified.PlaceID != req.SelectedPlace {
		t.Fatalf("verified = %+v error = %v", verified, err)
	}
	for name, candidate := range map[string]string{
		"tampered": token.Token + "x",
		"bad user": token.Token,
	} {
		userID := req.UserID
		if name == "bad user" {
			userID = "usr_other"
		}
		if _, err := VerifyCreationToken(candidate, userID, req.SelectedPlace, cfg.Now, cfg.SigningKey); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("%s error = %v, want ErrInvalidToken", name, err)
		}
	}
	if _, err := VerifyCreationToken(token.Token, req.UserID, req.SelectedPlace, token.ExpiresAt, cfg.SigningKey); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expired error = %v, want ErrInvalidToken", err)
	}
}

func TestValidateCreationTokenRequestRejectsTrustAndRateGates(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Request)
		want error
	}{
		{name: "trust", edit: func(req *Request) { req.Trust.Allowed = false }, want: ErrTrustGate},
		{name: "rate", edit: func(req *Request) { req.Rate.Allowed = false }, want: ErrRateLimited},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, cfg := validRequest()
			tt.edit(&req)
			_, err := ValidateCreationTokenRequest(req, cfg)
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func validRequest() (Request, Config) {
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	proofPoint := Coordinate{Latitude: 45.5152, Longitude: -122.6784}
	return Request{
		UserID: "usr_test_1", SelectedPlace: "place_library",
		Proof:  LocationProof{Coordinate: proofPoint, AccuracyMeters: 25, CapturedAt: now.Add(-30 * time.Second)},
		Places: []Place{{ID: "place_library", Status: PlaceActive, Center: proofPoint, RadiusMeters: 75}},
		Trust:  GateDecision{Allowed: true}, Rate: GateDecision{Allowed: true},
	}, Config{Now: now, SigningKey: []byte("test-signing-key")}
}
