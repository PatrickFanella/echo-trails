package apple

import (
	"errors"
	"os"
	"strings"
)

const (
	EnvTeamID     = "ECHOTRAILS_APPLE_TEAM_ID"
	EnvClientID   = "ECHOTRAILS_APPLE_CLIENT_ID"
	EnvKeyID      = "ECHOTRAILS_APPLE_KEY_ID"
	EnvPrivateKey = "ECHOTRAILS_APPLE_PRIVATE_KEY"
)

var ErrMissingProductionConfig = errors.New("missing Sign in with Apple production configuration")

// ProductionConfig names the values required by a future production Apple
// verifier. It intentionally carries no defaults or sample secret material.
type ProductionConfig struct {
	TeamID     string
	ClientID   string
	KeyID      string
	PrivateKey string
}

func LoadProductionConfigFromEnv() (ProductionConfig, error) {
	cfg := ProductionConfig{
		TeamID:     strings.TrimSpace(os.Getenv(EnvTeamID)),
		ClientID:   strings.TrimSpace(os.Getenv(EnvClientID)),
		KeyID:      strings.TrimSpace(os.Getenv(EnvKeyID)),
		PrivateKey: strings.TrimSpace(os.Getenv(EnvPrivateKey)),
	}
	if cfg.TeamID == "" || cfg.ClientID == "" || cfg.KeyID == "" || cfg.PrivateKey == "" {
		return ProductionConfig{}, ErrMissingProductionConfig
	}
	return cfg, nil
}
