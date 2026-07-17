package config_test

import (
	"strings"
	"testing"

	"github.com/zenith/core/internal/config"
)

const validSecret = "a-signing-secret-that-is-long-enough-ok"

// A deployment without a signing secret must not start. Inventing one silently
// would either sign everyone out on restart or, worse, ship a default key.
func TestProductionRefusesToBootWithoutSecret(t *testing.T) {
	t.Setenv("ZENITH_JWT_SECRET", "")
	t.Setenv("ZENITH_ENV", "production")

	_, err := config.Load()
	if err == nil {
		t.Fatal("loaded production config with no signing secret, want an error")
	}
	// The error must say how to fix it, not just what is wrong.
	if !strings.Contains(err.Error(), "ZENITH_JWT_SECRET") {
		t.Errorf("error does not name the variable: %v", err)
	}
	if !strings.Contains(err.Error(), "openssl") {
		t.Errorf("error does not say how to generate one: %v", err)
	}
}

// Production is the default: forgetting to set ZENITH_ENV must not silently
// hand out a generated secret.
func TestMissingEnvDefaultsToProduction(t *testing.T) {
	t.Setenv("ZENITH_JWT_SECRET", "")
	t.Setenv("ZENITH_ENV", "")

	if _, err := config.Load(); err == nil {
		t.Fatal("loaded with no ZENITH_ENV and no secret, want an error")
	}
}

func TestShortSecretIsRejected(t *testing.T) {
	t.Setenv("ZENITH_JWT_SECRET", "too-short")
	t.Setenv("ZENITH_ENV", "production")

	_, err := config.Load()
	if err == nil {
		t.Fatal("accepted a short signing secret, want an error")
	}
	if !strings.Contains(err.Error(), "32") {
		t.Errorf("error does not state the minimum length: %v", err)
	}
}

func TestValidSecretLoads(t *testing.T) {
	t.Setenv("ZENITH_JWT_SECRET", validSecret)
	t.Setenv("ZENITH_ENV", "production")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.JWTSecret != validSecret {
		t.Error("secret was not carried through")
	}
	if cfg.EphemeralSecret {
		t.Error("a configured secret was reported as ephemeral")
	}
}

// Development generates a secret so `go run ./cmd/core` works, but must flag it
// so nobody mistakes the behavior for production-safe.
func TestDevelopmentGeneratesEphemeralSecret(t *testing.T) {
	t.Setenv("ZENITH_JWT_SECRET", "")
	t.Setenv("ZENITH_ENV", "development")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.JWTSecret) < config.MinJWTSecretLength {
		t.Errorf("generated secret is %d chars, want >= %d",
			len(cfg.JWTSecret), config.MinJWTSecretLength)
	}
	if !cfg.EphemeralSecret {
		t.Error("generated secret was not flagged ephemeral")
	}
}

// Two dev boots must not share a secret, or the "ephemeral" one is a constant.
func TestGeneratedSecretsDiffer(t *testing.T) {
	t.Setenv("ZENITH_JWT_SECRET", "")
	t.Setenv("ZENITH_ENV", "development")

	first, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	second, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if first.JWTSecret == second.JWTSecret {
		t.Error("two loads generated the same secret")
	}
}

// Maintenance commands never issue tokens, so they must not demand a secret.
func TestLoadWithoutSecretNeedsNoSecret(t *testing.T) {
	t.Setenv("ZENITH_JWT_SECRET", "")
	t.Setenv("ZENITH_ENV", "production")

	cfg, err := config.LoadWithoutSecret()
	if err != nil {
		t.Fatalf("load without secret: %v", err)
	}
	if cfg.AppDBPath == "" {
		t.Error("app db path was not resolved")
	}
}

func TestDefaults(t *testing.T) {
	t.Setenv("ZENITH_JWT_SECRET", validSecret)
	t.Setenv("ZENITH_ENV", "production")
	t.Setenv("ZENITH_PORT", "")
	t.Setenv("ZENITH_TOKEN_TTL", "")
	t.Setenv("ZENITH_DATA_DIR", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Port != 8080 {
		t.Errorf("port = %d, want 8080", cfg.Port)
	}
	if cfg.TokenTTL.Hours() != 24 {
		t.Errorf("token ttl = %s, want 24h", cfg.TokenTTL)
	}
}

func TestInvalidPortIsRejected(t *testing.T) {
	t.Setenv("ZENITH_JWT_SECRET", validSecret)

	for _, port := range []string{"not-a-number", "0", "70000", "-1"} {
		t.Setenv("ZENITH_PORT", port)
		if _, err := config.Load(); err == nil {
			t.Errorf("accepted port %q, want an error", port)
		}
	}
}

// A zero or negative TTL would mint tokens that are already expired.
func TestInvalidTokenTTLIsRejected(t *testing.T) {
	t.Setenv("ZENITH_JWT_SECRET", validSecret)
	t.Setenv("ZENITH_PORT", "")

	for _, ttl := range []string{"not-a-duration", "0", "-1h"} {
		t.Setenv("ZENITH_TOKEN_TTL", ttl)
		if _, err := config.Load(); err == nil {
			t.Errorf("accepted ttl %q, want an error", ttl)
		}
	}
}
