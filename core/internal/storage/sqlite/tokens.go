package sqlite

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// RevokeToken adds a token id to the denylist.
func (s *Store) RevokeToken(ctx context.Context, jti, userID string, expiresAt time.Time) error {
	if jti == "" {
		return errors.New("sqlite: cannot revoke a token with no id")
	}

	// A token can be presented to logout twice -- two tabs, a retried request.
	// The second is not an error: the token is already revoked, which is what
	// the caller wanted.
	const q = `INSERT INTO revoked_tokens (jti, user_id, expires_at)
	           VALUES (?, ?, ?)
	           ON CONFLICT (jti) DO NOTHING`

	// An owner token's subject is a site, not a user row, so user_id is NULL
	// rather than a dangling reference the foreign key would reject.
	var user any
	if userID != "" {
		user = userID
	}

	if _, err := s.db.ExecContext(ctx, q, jti, user, expiresAt.UTC().Format(timeLayout)); err != nil {
		return fmt.Errorf("sqlite: revoke token: %w", err)
	}
	return nil
}

// IsTokenRevoked reports whether a token id has been revoked.
func (s *Store) IsTokenRevoked(ctx context.Context, jti string) (bool, error) {
	if jti == "" {
		// A token with no id could never have been revoked, and could never be.
		// Treat it as revoked rather than let it through.
		return true, nil
	}

	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM revoked_tokens WHERE jti = ?)`, jti).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("sqlite: check revocation: %w", err)
	}
	return exists, nil
}

// DeleteExpiredRevokedTokens drops denylist rows past their expiry.
func (s *Store) DeleteExpiredRevokedTokens(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM revoked_tokens WHERE expires_at < ?`,
		time.Now().UTC().Format(timeLayout))
	if err != nil {
		return 0, fmt.Errorf("sqlite: sweep revoked tokens: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return 0, nil // The sweep worked; the count is not worth an error.
	}
	return n, nil
}
