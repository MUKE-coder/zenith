// Package config loads Zenith core configuration from the environment.
package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// MinJWTSecretLength is the shortest signing secret Zenith will accept.
//
// HS256 keys shorter than the hash output weaken the signature, and a short
// secret is a guessable one: forging a token means forging any identity.
const MinJWTSecretLength = 32

// Config is the resolved runtime configuration for the core service.
type Config struct {
	// Port is the HTTP listen port.
	Port int

	// DataDir holds the DuckDB and SQLite files. In Docker this is a volume.
	DataDir string

	// EventsDBPath is the DuckDB file (analytics events).
	EventsDBPath string

	// AppDBPath is the SQLite file (users, sites, settings, jobs).
	AppDBPath string

	// JWTSecret signs session tokens.
	JWTSecret string

	// TokenTTL is how long a session token stays valid.
	TokenTTL time.Duration

	// Development relaxes production guards. Never set in a real deployment.
	Development bool

	// AdminEmail and AdminPassword auto-provision the first developer account
	// on boot. Both empty means no auto-provisioning.
	AdminEmail    string
	AdminPassword string

	// GeoIPDBPath points at a MaxMind-format country database. Empty means no
	// country resolution: geo is a nice-to-have, not a prerequisite for
	// running Zenith, and the database cannot be redistributed with it.
	GeoIPDBPath string

	// DashboardDir holds the built SPA. Empty, or missing, means the console
	// is not served and only the API is.
	DashboardDir string

	// ResendEndpoint overrides Resend's URL. Empty means Resend itself. It
	// exists so a deployment's email can be verified against a mock rather
	// than by mailing a real client and hoping.
	ResendEndpoint string

	// EphemeralSecret reports that JWTSecret was generated for this process
	// rather than configured, so every restart invalidates all sessions.
	EphemeralSecret bool
}

// Load reads configuration for the core server, applying defaults.
//
// It fails rather than guessing on anything that would be unsafe to get wrong,
// including a missing or too-short signing secret.
func Load() (Config, error) {
	cfg, err := loadBase()
	if err != nil {
		return Config{}, err
	}

	cfg.JWTSecret, cfg.EphemeralSecret, err = loadJWTSecret(cfg.Development)
	if err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// LoadWithoutSecret reads configuration for maintenance commands, which need
// data paths and admin credentials but never issue tokens.
//
// It exists so that seeding an account does not require a signing secret it
// would not use -- a requirement that would only teach operators to invent a
// throwaway value for it.
func LoadWithoutSecret() (Config, error) {
	return loadBase()
}

func loadBase() (Config, error) {
	dataDir := env("ZENITH_DATA_DIR", "./data")
	development := env("ZENITH_ENV", "production") == "development"

	port, err := strconv.Atoi(env("ZENITH_PORT", "8080"))
	if err != nil {
		return Config{}, fmt.Errorf("config: ZENITH_PORT must be a number: %w", err)
	}
	if port < 1 || port > 65535 {
		return Config{}, fmt.Errorf("config: ZENITH_PORT out of range: %d", port)
	}

	ttl, err := time.ParseDuration(env("ZENITH_TOKEN_TTL", "24h"))
	if err != nil {
		return Config{}, fmt.Errorf("config: ZENITH_TOKEN_TTL must be a duration like 24h: %w", err)
	}
	if ttl <= 0 {
		return Config{}, fmt.Errorf("config: ZENITH_TOKEN_TTL must be positive, got %s", ttl)
	}

	return Config{
		Port:           port,
		DataDir:        dataDir,
		EventsDBPath:   env("ZENITH_EVENTS_DB", filepath.Join(dataDir, "events.duckdb")),
		AppDBPath:      env("ZENITH_APP_DB", filepath.Join(dataDir, "zenith.sqlite")),
		TokenTTL:       ttl,
		Development:    development,
		AdminEmail:     os.Getenv("ZENITH_ADMIN_EMAIL"),
		AdminPassword:  os.Getenv("ZENITH_ADMIN_PASSWORD"),
		GeoIPDBPath:    os.Getenv("ZENITH_GEOIP_DB"),
		DashboardDir:   env("ZENITH_DASHBOARD_DIR", "./dashboard"),
		ResendEndpoint: os.Getenv("ZENITH_RESEND_ENDPOINT"),
	}, nil
}

// loadJWTSecret resolves the signing secret.
//
// There is deliberately no default. A hardcoded fallback would ship to
// production as the key that signs every token, and generating one silently
// would log every user out on each restart. In production a missing secret is
// a boot failure; in development it is generated, loudly.
func loadJWTSecret(development bool) (secret string, ephemeral bool, err error) {
	secret = os.Getenv("ZENITH_JWT_SECRET")

	if secret == "" {
		if !development {
			return "", false, fmt.Errorf(
				"config: ZENITH_JWT_SECRET is not set. Generate one with "+
					"`openssl rand -base64 %d` and set it in the environment",
				MinJWTSecretLength)
		}

		generated, err := randomSecret()
		if err != nil {
			return "", false, err
		}
		return generated, true, nil
	}

	if len(secret) < MinJWTSecretLength {
		return "", false, fmt.Errorf(
			"config: ZENITH_JWT_SECRET is %d characters; it must be at least %d. "+
				"Generate one with `openssl rand -base64 %d`",
			len(secret), MinJWTSecretLength, MinJWTSecretLength)
	}

	return secret, false, nil
}

func randomSecret() (string, error) {
	b := make([]byte, MinJWTSecretLength)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("config: generate secret: %w", err)
	}
	return base64.RawStdEncoding.EncodeToString(b), nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
