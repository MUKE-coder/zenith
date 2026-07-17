package auth_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/zenith/core/internal/auth"
	"github.com/zenith/core/internal/storage"
)

const testSecret = "a-test-signing-secret-long-enough-to-pass"

func newIssuer(t *testing.T) *auth.Issuer {
	t.Helper()
	i, err := auth.NewIssuer(testSecret, time.Hour)
	if err != nil {
		t.Fatalf("new issuer: %v", err)
	}
	return i
}

func TestIssueAndParseDeveloper(t *testing.T) {
	i := newIssuer(t)

	token, expiresAt, err := i.IssueDeveloper("user-1")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if expiresAt.IsZero() {
		t.Error("issue returned a zero expiry")
	}

	claims, err := i.Parse(token)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.Role != storage.RoleDeveloper {
		t.Errorf("role = %q, want developer", claims.Role)
	}
	if claims.Subject != "user-1" {
		t.Errorf("subject = %q, want user-1", claims.Subject)
	}
	if claims.SiteID != "" {
		t.Errorf("developer token names site %q; it should be scoped to all sites", claims.SiteID)
	}
	if claims.TokenID() == "" {
		t.Error("token has no jti and so could never be revoked")
	}
}

func TestIssueAndParseOwner(t *testing.T) {
	i := newIssuer(t)

	token, _, err := i.IssueOwner("site-42")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	claims, err := i.Parse(token)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.Role != storage.RoleOwner {
		t.Errorf("role = %q, want owner", claims.Role)
	}
	if claims.SiteID != "site-42" {
		t.Errorf("site_id = %q, want site-42", claims.SiteID)
	}
}

// Every token must carry a distinct jti, or revoking one would revoke another.
func TestTokenIDsAreUnique(t *testing.T) {
	i := newIssuer(t)

	seen := make(map[string]bool, 100)
	for range 100 {
		token, _, err := i.IssueDeveloper("user-1")
		if err != nil {
			t.Fatalf("issue: %v", err)
		}
		claims, err := i.Parse(token)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if seen[claims.TokenID()] {
			t.Fatalf("duplicate jti %q: revoking one session would revoke another", claims.TokenID())
		}
		seen[claims.TokenID()] = true
	}
}

func TestOwnerTokenMustNameASite(t *testing.T) {
	i := newIssuer(t)
	if _, _, err := i.IssueOwner(""); err == nil {
		t.Error("issued an owner token with no site, want rejection")
	}
}

// A token signed with a different secret must not be accepted.
func TestParseRejectsForeignSignature(t *testing.T) {
	mint := newIssuer(t)
	token, _, err := mint.IssueDeveloper("user-1")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	other, err := auth.NewIssuer("a-completely-different-secret-of-length", time.Hour)
	if err != nil {
		t.Fatalf("new issuer: %v", err)
	}

	if _, err := other.Parse(token); !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("got %v, want ErrInvalidToken for a token signed elsewhere", err)
	}
}

// Flipping any byte of the signature must invalidate the token.
func TestParseRejectsTamperedToken(t *testing.T) {
	i := newIssuer(t)
	token, _, err := i.IssueDeveloper("user-1")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	tampered := token[:len(token)-1] + flipChar(token[len(token)-1])
	if _, err := i.Parse(tampered); !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("got %v, want ErrInvalidToken for a tampered signature", err)
	}
}

// The classic JWT forgery: alg=none, no signature. If the parser trusts the
// token's own header, anyone can mint any identity.
func TestParseRejectsAlgNone(t *testing.T) {
	i := newIssuer(t)

	forged := jwt.NewWithClaims(jwt.SigningMethodNone, auth.Claims{
		Role: storage.RoleDeveloper,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "zenith",
			Subject:   "attacker",
			ID:        "forged-jti",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	raw, err := forged.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign none: %v", err)
	}

	if _, err := i.Parse(raw); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("got %v, want ErrInvalidToken: an alg=none token was accepted", err)
	}
}

func TestParseRejectsExpiredToken(t *testing.T) {
	i, err := auth.NewIssuer(testSecret, time.Millisecond)
	if err != nil {
		t.Fatalf("new issuer: %v", err)
	}

	token, _, err := i.IssueDeveloper("user-1")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	if _, err := i.Parse(token); !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("got %v, want ErrInvalidToken for an expired token", err)
	}
}

// A token with no exp would never expire. It must not be accepted.
func TestParseRejectsTokenWithoutExpiry(t *testing.T) {
	i := newIssuer(t)

	forged := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss":  "zenith",
		"sub":  "user-1",
		"jti":  "no-expiry",
		"role": "developer",
	})
	raw, err := forged.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	if _, err := i.Parse(raw); !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("got %v, want ErrInvalidToken for a token with no expiry", err)
	}
}

// A validly signed token claiming a role the system does not define.
func TestParseRejectsUnknownRole(t *testing.T) {
	i := newIssuer(t)

	forged := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss":  "zenith",
		"sub":  "user-1",
		"jti":  "x",
		"role": "superuser",
		"exp":  time.Now().Add(time.Hour).Unix(),
	})
	raw, err := forged.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	if _, err := i.Parse(raw); !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("got %v, want ErrInvalidToken for role \"superuser\"", err)
	}
}

// An owner token with no site would be scoped to nothing -- or, misread, to
// everything. Reject it rather than find out which.
func TestParseRejectsOwnerWithoutSite(t *testing.T) {
	i := newIssuer(t)

	forged := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss":  "zenith",
		"sub":  "site:",
		"jti":  "x",
		"role": "owner",
		"exp":  time.Now().Add(time.Hour).Unix(),
	})
	raw, err := forged.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	if _, err := i.Parse(raw); !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("got %v, want ErrInvalidToken for an owner token naming no site", err)
	}
}

// A developer token is scoped to every site; naming one is a contradiction.
func TestParseRejectsDeveloperWithSite(t *testing.T) {
	i := newIssuer(t)

	forged := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss":     "zenith",
		"sub":     "user-1",
		"jti":     "x",
		"role":    "developer",
		"site_id": "site-1",
		"exp":     time.Now().Add(time.Hour).Unix(),
	})
	raw, err := forged.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	if _, err := i.Parse(raw); !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("got %v, want ErrInvalidToken for a developer token naming a site", err)
	}
}

// A token minted by something else that happens to know the secret.
func TestParseRejectsForeignIssuer(t *testing.T) {
	i := newIssuer(t)

	forged := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss":  "not-zenith",
		"sub":  "user-1",
		"jti":  "x",
		"role": "developer",
		"exp":  time.Now().Add(time.Hour).Unix(),
	})
	raw, err := forged.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	if _, err := i.Parse(raw); !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("got %v, want ErrInvalidToken for a foreign issuer", err)
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	i := newIssuer(t)

	for _, token := range []string{"", "not-a-token", "a.b.c", strings.Repeat("x", 500)} {
		if _, err := i.Parse(token); !errors.Is(err, auth.ErrInvalidToken) {
			t.Errorf("Parse(%q) = %v, want ErrInvalidToken", token, err)
		}
	}
}

// A short secret is a guessable one: forging a token forges any identity.
func TestNewIssuerRejectsShortSecret(t *testing.T) {
	if _, err := auth.NewIssuer("too-short", time.Hour); err == nil {
		t.Error("accepted a short signing secret, want rejection")
	}
}

func TestNewIssuerRejectsNonPositiveTTL(t *testing.T) {
	for _, ttl := range []time.Duration{0, -time.Hour} {
		if _, err := auth.NewIssuer(testSecret, ttl); err == nil {
			t.Errorf("accepted ttl %s, want rejection", ttl)
		}
	}
}

func flipChar(c byte) string {
	if c == 'A' {
		return "B"
	}
	return "A"
}
