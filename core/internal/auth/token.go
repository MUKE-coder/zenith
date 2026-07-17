package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/zenith/core/internal/id"
	"github.com/zenith/core/internal/storage"
)

// Issuer is the token issuer name in every Zenith token.
const tokenIssuer = "zenith"

// signingMethod is the only algorithm Zenith signs or accepts.
//
// Pinning it is the defense against algorithm confusion: a parser that trusts
// the token's own `alg` header will happily accept "none" (no signature at all)
// or verify an HMAC using a public key as the secret. The algorithm is our
// decision, never the token's.
var signingMethod = jwt.SigningMethodHS256

// ErrInvalidToken is returned for any token that fails validation, for any
// reason. The reason is deliberately not exposed to callers.
var ErrInvalidToken = errors.New("invalid or expired token")

// Claims is a Zenith session token's payload.
type Claims struct {
	// Role decides what the holder may see: every site, or one.
	Role storage.Role `json:"role"`

	// SiteID scopes an owner token to exactly one site. Empty for developers,
	// who are scoped to all of them.
	SiteID string `json:"site_id,omitempty"`

	jwt.RegisteredClaims
}

// TokenID returns the token's unique id, used to revoke it.
func (c *Claims) TokenID() string { return c.ID }

// Issuer mints and validates session tokens.
type Issuer struct {
	secret []byte
	ttl    time.Duration

	// now is overridable so tests can exercise expiry without sleeping.
	now func() time.Time
}

// NewIssuer returns an Issuer signing with secret.
func NewIssuer(secret string, ttl time.Duration) (*Issuer, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("auth: signing secret must be at least 32 characters, got %d", len(secret))
	}
	if ttl <= 0 {
		return nil, fmt.Errorf("auth: token ttl must be positive, got %s", ttl)
	}
	return &Issuer{secret: []byte(secret), ttl: ttl, now: time.Now}, nil
}

// IssueDeveloper mints a token that can see every site.
func (i *Issuer) IssueDeveloper(userID string) (string, time.Time, error) {
	return i.issue(userID, storage.RoleDeveloper, "")
}

// IssueOwner mints a token scoped to exactly one site.
//
// This is what the domain-native dashboard runs on: the holder proved they know
// one site's password, so the token must not be able to read any other site.
// The site id is baked into the token rather than passed alongside it, so a
// query cannot ask for a site the token does not name.
func (i *Issuer) IssueOwner(siteID string) (string, time.Time, error) {
	if siteID == "" {
		return "", time.Time{}, errors.New("auth: an owner token must name a site")
	}
	return i.issue("site:"+siteID, storage.RoleOwner, siteID)
}

func (i *Issuer) issue(subject string, role storage.Role, siteID string) (string, time.Time, error) {
	if subject == "" {
		return "", time.Time{}, errors.New("auth: token subject is required")
	}
	if !role.Valid() {
		return "", time.Time{}, fmt.Errorf("auth: invalid role %q", role)
	}

	jti, err := id.New()
	if err != nil {
		return "", time.Time{}, err
	}

	now := i.now().UTC()
	expiresAt := now.Add(i.ttl)

	claims := Claims{
		Role:   role,
		SiteID: siteID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tokenIssuer,
			Subject:   subject,
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			// Every token expires. A token that never expires is a password
			// that can never be changed.
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	signed, err := jwt.NewWithClaims(signingMethod, claims).SignedString(i.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: sign token: %w", err)
	}
	return signed, expiresAt, nil
}

// Parse validates a token and returns its claims.
//
// It returns ErrInvalidToken for every failure -- bad signature, wrong
// algorithm, expired, malformed -- so a caller cannot accidentally tell an
// attacker which part of their forgery to fix.
func (i *Issuer) Parse(token string) (*Claims, error) {
	claims := &Claims{}

	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		return i.secret, nil
	},
		jwt.WithValidMethods([]string{signingMethod.Alg()}),
		jwt.WithIssuer(tokenIssuer),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(func() time.Time { return i.now() }),
	)
	if err != nil || !parsed.Valid {
		return nil, ErrInvalidToken
	}

	// Structural checks the library cannot know about.
	if !claims.Role.Valid() {
		return nil, ErrInvalidToken
	}
	if claims.ID == "" {
		// Without a jti the token could never be revoked.
		return nil, ErrInvalidToken
	}
	if claims.Role == storage.RoleOwner && claims.SiteID == "" {
		// An owner token that names no site would be scoped to nothing --
		// or, read wrongly, to everything.
		return nil, ErrInvalidToken
	}
	if claims.Role == storage.RoleDeveloper && claims.SiteID != "" {
		// A developer token is scoped to all sites; naming one is a
		// contradiction that suggests a forged or confused token.
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// Expiry returns when the token stops being valid.
//
// Named Expiry rather than ExpiresAt so it does not shadow the embedded
// RegisteredClaims field of that name.
func (c *Claims) Expiry() time.Time {
	if c.RegisteredClaims.ExpiresAt == nil {
		return time.Time{}
	}
	return c.RegisteredClaims.ExpiresAt.Time
}
