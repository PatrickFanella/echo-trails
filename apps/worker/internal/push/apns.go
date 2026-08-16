package push

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

var ErrInvalidDeviceToken = errors.New("invalid APNs device token")

var coordinatePairPattern = regexp.MustCompile(`[-+]?\d{1,3}\.\d{4,}\s*[,;]\s*[-+]?\d{1,3}\.\d{4,}`)

type Notification struct {
	NotificationID string
	DeviceID       string
	Token          string
	Environment    string
	Kind           string
	Preview        string
	EchoID         string
	ActionID       string
	CollapseKey    string
}

type Provider interface {
	Send(context.Context, Notification) (string, error)
}

type APNsConfig struct {
	TeamID             string
	KeyID              string
	BundleID           string
	PrivateKeyPEM      []byte
	SandboxEndpoint    string
	ProductionEndpoint string
	HTTPClient         *http.Client
}

type APNsProvider struct {
	config APNsConfig
	key    *ecdsa.PrivateKey
	mu     sync.Mutex
	jwt    string
	jwtAt  time.Time
}

func NewAPNsProvider(config APNsConfig) (*APNsProvider, error) {
	if strings.TrimSpace(config.TeamID) == "" || strings.TrimSpace(config.KeyID) == "" || strings.TrimSpace(config.BundleID) == "" {
		return nil, errors.New("APNs team, key, and bundle identifiers are required")
	}
	block, _ := pem.Decode(config.PrivateKeyPEM)
	if block == nil {
		return nil, errors.New("invalid APNs private key PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse APNs private key: %w", err)
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("APNs private key must be ECDSA")
	}
	if config.SandboxEndpoint == "" {
		config.SandboxEndpoint = "https://api.sandbox.push.apple.com"
	}
	if config.ProductionEndpoint == "" {
		config.ProductionEndpoint = "https://api.push.apple.com"
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &APNsProvider{config: config, key: key}, nil
}

func (p *APNsProvider) Send(ctx context.Context, notification Notification) (string, error) {
	endpoint := p.config.ProductionEndpoint
	if notification.Environment == "sandbox" {
		endpoint = p.config.SandboxEndpoint
	}
	deepLink := "/echoes/" + notification.EchoID
	if notification.ActionID != "" {
		deepLink = "/moderation/" + notification.ActionID
	}
	payload := map[string]any{
		"aps": map[string]any{
			"alert": map[string]string{"title": "EchoTrails", "body": safePreview(notification.Preview)},
			"sound": "default", "category": "ECHOTRAILS_ACTIVITY", "thread-id": notification.EchoID,
		},
		"deep_link":       deepLink,
		"notification_id": notification.NotificationID,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(endpoint, "/")+"/3/device/"+notification.Token, strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	token, err := p.providerToken(time.Now().UTC())
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "bearer "+token)
	request.Header.Set("apns-topic", p.config.BundleID)
	request.Header.Set("apns-push-type", "alert")
	request.Header.Set("apns-priority", "10")
	if notification.CollapseKey != "" {
		request.Header.Set("apns-collapse-id", notification.CollapseKey)
	}
	response, err := p.config.HTTPClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	apnsID := response.Header.Get("apns-id")
	if response.StatusCode == http.StatusOK {
		return apnsID, nil
	}
	data, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	var failure struct {
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(data, &failure)
	if response.StatusCode == http.StatusGone || failure.Reason == "BadDeviceToken" || failure.Reason == "Unregistered" || failure.Reason == "DeviceTokenNotForTopic" {
		return apnsID, ErrInvalidDeviceToken
	}
	return apnsID, fmt.Errorf("APNs status %d reason %s", response.StatusCode, failure.Reason)
}

func (p *APNsProvider) providerToken(now time.Time) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.jwt != "" && now.Sub(p.jwtAt) < 50*time.Minute {
		return p.jwt, nil
	}
	header, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": p.config.KeyID})
	claims, _ := json.Marshal(map[string]any{"iss": p.config.TeamID, "iat": now.Unix()})
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(unsigned))
	r, s, err := ecdsa.Sign(rand.Reader, p.key, digest[:])
	if err != nil {
		return "", err
	}
	signature := append(paddedBigInt(r, 32), paddedBigInt(s, 32)...)
	p.jwt = unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
	p.jwtAt = now
	return p.jwt, nil
}

func paddedBigInt(value *big.Int, size int) []byte {
	bytes := value.Bytes()
	result := make([]byte, size)
	copy(result[size-len(bytes):], bytes)
	return result
}

func safePreview(preview string) string {
	preview = strings.TrimSpace(preview)
	if preview == "" || len([]rune(preview)) > 160 || coordinatePairPattern.MatchString(preview) {
		return "You have new activity on EchoTrails."
	}
	return preview
}
