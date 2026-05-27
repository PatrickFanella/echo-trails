package locationproof

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	DefaultMaxProofAge = 2 * time.Minute
	DefaultMaxAccuracy = 100.0
	defaultTokenTTL    = 10 * time.Minute
)

var (
	ErrStaleProof             = errors.New("location proof is stale")
	ErrPoorAccuracy           = errors.New("location proof accuracy is too poor")
	ErrOutsideBoundary        = errors.New("location proof is outside place boundary")
	ErrRestrictedPlace        = errors.New("place is restricted")
	ErrAmbiguousEligiblePlace = errors.New("location proof matches multiple eligible places")
	ErrTrustGate              = errors.New("trust gate rejected creation token")
	ErrRateLimited            = errors.New("rate gate rejected creation token")
	ErrInvalidRequest         = errors.New("invalid location proof request")
	ErrMissingSigningKey      = errors.New("missing creation token signing key")
)

type PlaceStatus string

const (
	PlaceActive     PlaceStatus = "active"
	PlaceRestricted PlaceStatus = "restricted"
)

type Coordinate struct {
	Latitude  float64
	Longitude float64
}

type LocationProof struct {
	Coordinate     Coordinate
	AccuracyMeters float64
	CapturedAt     time.Time
}

type Place struct {
	ID           string
	Status       PlaceStatus
	Center       Coordinate
	RadiusMeters float64
}

type GateDecision struct {
	Allowed bool
	Reason  string
}

type Request struct {
	UserID        string
	SelectedPlace string
	Proof         LocationProof
	Places        []Place
	Trust         GateDecision
	Rate          GateDecision
}

type Config struct {
	Now               time.Time
	MaxProofAge       time.Duration
	MaxAccuracyMeters float64
	TokenTTL          time.Duration
	SigningKey        []byte
}

type CreationToken struct {
	Token     string
	PlaceID   string
	UserID    string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

type tokenClaims struct {
	Version   int    `json:"v"`
	UserID    string `json:"user_id"`
	PlaceID   string `json:"place_id"`
	ProofTime int64  `json:"proof_captured_at_unix_ms"`
	IssuedAt  int64  `json:"issued_at_unix_ms"`
	ExpiresAt int64  `json:"expires_at_unix_ms"`
}

func ValidateCreationTokenRequest(req Request, cfg Config) (CreationToken, error) {
	cfg = normalizeConfig(cfg)
	if err := validateRequestShape(req); err != nil {
		return CreationToken{}, err
	}
	if !req.Trust.Allowed {
		return CreationToken{}, ErrTrustGate
	}
	if !req.Rate.Allowed {
		return CreationToken{}, ErrRateLimited
	}
	if cfg.Now.Sub(req.Proof.CapturedAt.UTC()) > cfg.MaxProofAge || req.Proof.CapturedAt.After(cfg.Now.Add(time.Second)) {
		return CreationToken{}, ErrStaleProof
	}
	if req.Proof.AccuracyMeters <= 0 || req.Proof.AccuracyMeters > cfg.MaxAccuracyMeters {
		return CreationToken{}, ErrPoorAccuracy
	}
	if math.IsNaN(req.Proof.AccuracyMeters) || math.IsInf(req.Proof.AccuracyMeters, 0) {
		return CreationToken{}, ErrPoorAccuracy
	}

	selected, found := findPlace(req.Places, req.SelectedPlace)
	if !found {
		return CreationToken{}, ErrInvalidRequest
	}
	if selected.Status == PlaceRestricted {
		return CreationToken{}, ErrRestrictedPlace
	}
	if selected.Status != PlaceActive {
		return CreationToken{}, ErrOutsideBoundary
	}
	if !containsProof(selected, req.Proof.Coordinate) {
		return CreationToken{}, ErrOutsideBoundary
	}
	if eligibleMatches(req.Places, req.Proof.Coordinate) > 1 {
		return CreationToken{}, ErrAmbiguousEligiblePlace
	}

	issuedAt := cfg.Now.UTC().Round(0)
	expiresAt := issuedAt.Add(cfg.TokenTTL)
	token, err := signToken(tokenClaims{
		Version: 1, UserID: strings.TrimSpace(req.UserID), PlaceID: selected.ID,
		ProofTime: req.Proof.CapturedAt.UTC().UnixMilli(), IssuedAt: issuedAt.UnixMilli(), ExpiresAt: expiresAt.UnixMilli(),
	}, cfg.SigningKey)
	if err != nil {
		return CreationToken{}, err
	}
	return CreationToken{Token: token, PlaceID: selected.ID, UserID: strings.TrimSpace(req.UserID), IssuedAt: issuedAt, ExpiresAt: expiresAt}, nil
}

func normalizeConfig(cfg Config) Config {
	if cfg.Now.IsZero() {
		cfg.Now = time.Now().UTC()
	} else {
		cfg.Now = cfg.Now.UTC().Round(0)
	}
	if cfg.MaxProofAge == 0 {
		cfg.MaxProofAge = DefaultMaxProofAge
	}
	if cfg.MaxAccuracyMeters == 0 {
		cfg.MaxAccuracyMeters = DefaultMaxAccuracy
	}
	if cfg.TokenTTL == 0 {
		cfg.TokenTTL = defaultTokenTTL
	}
	return cfg
}

func validateRequestShape(req Request) error {
	if strings.TrimSpace(req.UserID) == "" || strings.TrimSpace(req.SelectedPlace) == "" || len(req.Places) == 0 {
		return ErrInvalidRequest
	}
	if req.Proof.CapturedAt.IsZero() || !validCoordinate(req.Proof.Coordinate) {
		return ErrInvalidRequest
	}
	if len(req.Places) > 0 {
		for _, place := range req.Places {
			if place.ID == "" || place.RadiusMeters <= 0 || math.IsNaN(place.RadiusMeters) || math.IsInf(place.RadiusMeters, 0) {
				return ErrInvalidRequest
			}
		}
	}
	return nil
}

func findPlace(places []Place, id string) (Place, bool) {
	for _, place := range places {
		if place.ID == id {
			return place, true
		}
	}
	return Place{}, false
}

func eligibleMatches(places []Place, point Coordinate) int {
	matches := 0
	for _, place := range places {
		if place.Status == PlaceActive && containsProof(place, point) {
			matches++
		}
	}
	return matches
}

func containsProof(place Place, point Coordinate) bool {
	return place.RadiusMeters > 0 && validCoordinate(place.Center) && validCoordinate(point) && DistanceMeters(place.Center, point) <= place.RadiusMeters
}

func validCoordinate(c Coordinate) bool {
	return c.Latitude >= -90 && c.Latitude <= 90 && c.Longitude >= -180 && c.Longitude <= 180
}

func DistanceMeters(a, b Coordinate) float64 {
	const earthRadiusMeters = 6371000.0
	lat1 := degreesToRadians(a.Latitude)
	lat2 := degreesToRadians(b.Latitude)
	deltaLat := degreesToRadians(b.Latitude - a.Latitude)
	deltaLon := degreesToRadians(b.Longitude - a.Longitude)

	sinLat := math.Sin(deltaLat / 2)
	sinLon := math.Sin(deltaLon / 2)
	h := sinLat*sinLat + math.Cos(lat1)*math.Cos(lat2)*sinLon*sinLon
	return earthRadiusMeters * 2 * math.Atan2(math.Sqrt(h), math.Sqrt(1-h))
}

func degreesToRadians(degrees float64) float64 { return degrees * math.Pi / 180 }

func signToken(claims tokenClaims, key []byte) (string, error) {
	if len(key) == 0 {
		return "", ErrMissingSigningKey
	}
	body, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encodedBody := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(encodedBody))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("crt_%s.%s", encodedBody, signature), nil
}
