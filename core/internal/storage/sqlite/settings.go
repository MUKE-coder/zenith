package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zenith/core/internal/storage"
)

// Settings returns the global settings.
//
// The row is created by migration 0004 and constrained to id = 1, so this is a
// plain SELECT with no "missing row" branch to get wrong.
func (s *Store) Settings(ctx context.Context) (storage.Settings, error) {
	const q = `SELECT resend_api_key, mail_from, updated_at FROM settings WHERE id = 1`

	var (
		out       storage.Settings
		apiKey    sql.NullString
		mailFrom  sql.NullString
		updatedAt string
	)

	err := s.db.QueryRowContext(ctx, q).Scan(&apiKey, &mailFrom, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		// Only reachable if someone deleted the singleton by hand.
		return storage.Settings{}, nil
	}
	if err != nil {
		return storage.Settings{}, fmt.Errorf("sqlite: settings: %w", err)
	}

	out.ResendAPIKey = apiKey.String
	out.MailFrom = mailFrom.String

	if parsed, err := time.Parse(timeLayout, updatedAt); err == nil {
		out.UpdatedAt = parsed
	}
	return out, nil
}

// UpdateSettings replaces the global settings.
func (s *Store) UpdateSettings(ctx context.Context, in storage.Settings) error {
	const q = `UPDATE settings
	           SET resend_api_key = ?, mail_from = ?, updated_at = ?
	           WHERE id = 1`

	_, err := s.db.ExecContext(ctx, q,
		nullable(strings.TrimSpace(in.ResendAPIKey)),
		nullable(strings.TrimSpace(in.MailFrom)),
		time.Now().UTC().Format(timeLayout),
	)
	if err != nil {
		// Never wrap the raw error with the row: the api key is in it.
		return errors.New("sqlite: could not update settings")
	}
	return nil
}

// nullable stores an empty string as NULL, so "unset" and "set to nothing" do
// not become two different states meaning the same thing.
func nullable(v string) any {
	if v == "" {
		return nil
	}
	return v
}
