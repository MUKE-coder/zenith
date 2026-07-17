package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/zenith/core/internal/storage"
)

const timeLayout = "2006-01-02T15:04:05Z"

// CreateUser stores a new account.
func (s *Store) CreateUser(ctx context.Context, u storage.User) error {
	if !u.Role.Valid() {
		return fmt.Errorf("sqlite: invalid role %q", u.Role)
	}
	if u.PasswordHash == "" {
		return errors.New("sqlite: refusing to store a user with an empty password hash")
	}

	createdAt := u.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	const q = `INSERT INTO users (id, email, password_hash, role, created_at)
	           VALUES (?, ?, ?, ?, ?)`

	_, err := s.db.ExecContext(ctx, q,
		u.ID, strings.TrimSpace(u.Email), u.PasswordHash, string(u.Role),
		createdAt.Format(timeLayout),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return storage.ErrConflict
		}
		// Never wrap the raw error with the row: password_hash is in it.
		return fmt.Errorf("sqlite: create user: %w", err)
	}
	return nil
}

// UserByEmail looks up an account by email, case-insensitively.
func (s *Store) UserByEmail(ctx context.Context, email string) (storage.User, error) {
	const q = `SELECT id, email, password_hash, role, created_at
	           FROM users WHERE email = ?`

	var u storage.User
	var role, createdAt string

	err := s.db.QueryRowContext(ctx, q, strings.TrimSpace(email)).
		Scan(&u.ID, &u.Email, &u.PasswordHash, &role, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.User{}, storage.ErrNotFound
	}
	if err != nil {
		return storage.User{}, fmt.Errorf("sqlite: user by email: %w", err)
	}

	u.Role = storage.Role(role)
	u.CreatedAt, err = time.Parse(timeLayout, createdAt)
	if err != nil {
		return storage.User{}, fmt.Errorf("sqlite: parse created_at: %w", err)
	}
	return u, nil
}

// CountUsers returns the number of accounts.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil {
		return 0, fmt.Errorf("sqlite: count users: %w", err)
	}
	return n, nil
}

// isUniqueViolation reports whether err is a UNIQUE constraint failure, so a
// duplicate email surfaces as ErrConflict rather than an opaque driver error.
func isUniqueViolation(err error) bool {
	var serr *sqlite.Error
	if errors.As(err, &serr) {
		code := serr.Code()
		return code == sqlite3.SQLITE_CONSTRAINT_UNIQUE ||
			code == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY
	}
	return false
}
