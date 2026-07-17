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

// siteColumns is the column list every site query selects, in struct order.
const siteColumns = `id, name, domain, site_key, api_key, owner_email, created_at`

// CreateSite stores a new site.
func (s *Store) CreateSite(ctx context.Context, site storage.Site) error {
	if site.ID == "" || site.Name == "" || site.Domain == "" {
		return errors.New("sqlite: site id, name, and domain are required")
	}
	// A site with no keys could never receive an event or be read.
	if site.SiteKey == "" || site.APIKey == "" {
		return errors.New("sqlite: a site needs both a site key and an api key")
	}

	createdAt := site.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	var ownerEmail any
	if site.OwnerEmail != "" {
		ownerEmail = site.OwnerEmail
	}

	const q = `INSERT INTO sites (id, name, domain, site_key, api_key, owner_email, created_at)
	           VALUES (?, ?, ?, ?, ?, ?, ?)`

	_, err := s.db.ExecContext(ctx, q,
		site.ID, site.Name, site.Domain, site.SiteKey, site.APIKey,
		ownerEmail, createdAt.Format(timeLayout),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return storage.ErrConflict
		}
		return fmt.Errorf("sqlite: create site: %w", err)
	}
	return nil
}

// UpdateSite changes a site's name, domain, and owner email.
//
// The keys are deliberately absent: rotating one breaks every installed
// snippet or every dashboard, and that must not be reachable from an edit
// form.
func (s *Store) UpdateSite(ctx context.Context, site storage.Site) error {
	if site.ID == "" {
		return errors.New("sqlite: site id is required")
	}
	if site.Name == "" || site.Domain == "" {
		return errors.New("sqlite: site name and domain are required")
	}

	const q = `UPDATE sites SET name = ?, domain = ?, owner_email = ? WHERE id = ?`

	res, err := s.db.ExecContext(ctx, q,
		site.Name, site.Domain, nullable(site.OwnerEmail), site.ID)
	if err != nil {
		return fmt.Errorf("sqlite: update site: %w", err)
	}

	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return storage.ErrNotFound
	}
	return nil
}

// DeleteSite removes a site.
//
// The foreign keys on audit_jobs, audit_results, and report_history are ON
// DELETE CASCADE, so deleting the site row takes its audits and report history
// with it. The revoked_tokens table references users, not sites, so an owner's
// dead sessions are unaffected -- and harmless, since the site they named is
// gone.
func (s *Store) DeleteSite(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("sqlite: site id is required")
	}

	res, err := s.db.ExecContext(ctx, `DELETE FROM sites WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("sqlite: delete site: %w", err)
	}

	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return storage.ErrNotFound
	}
	return nil
}

// ListSites returns every site, oldest first.
func (s *Store) ListSites(ctx context.Context) ([]storage.Site, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+siteColumns+` FROM sites ORDER BY created_at ASC, name ASC`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list sites: %w", err)
	}
	defer rows.Close()

	// Never nil: an empty list must encode as [] rather than null.
	sites := []storage.Site{}

	for rows.Next() {
		site, err := scanSite(rows)
		if err != nil {
			return nil, err
		}
		sites = append(sites, site)
	}
	return sites, rows.Err()
}

// SiteByID looks up a site.
func (s *Store) SiteByID(ctx context.Context, id string) (storage.Site, error) {
	if strings.TrimSpace(id) == "" {
		return storage.Site{}, storage.ErrNotFound
	}
	return s.siteWhere(ctx, `id = ?`, id)
}

// SiteBySiteKey resolves a public site key to its site.
func (s *Store) SiteBySiteKey(ctx context.Context, siteKey string) (storage.Site, error) {
	if strings.TrimSpace(siteKey) == "" {
		return storage.Site{}, storage.ErrNotFound
	}
	return s.siteWhere(ctx, `site_key = ?`, siteKey)
}

// SiteByAPIKey resolves a secret api key to its site.
func (s *Store) SiteByAPIKey(ctx context.Context, apiKey string) (storage.Site, error) {
	if strings.TrimSpace(apiKey) == "" {
		return storage.Site{}, storage.ErrNotFound
	}
	// api_key is nullable, and a NULL must never be matched by an empty
	// string -- the guard above is what stops a blank key opening a site.
	return s.siteWhere(ctx, `api_key = ?`, apiKey)
}

func (s *Store) siteWhere(ctx context.Context, where string, args ...any) (storage.Site, error) {
	q := `SELECT ` + siteColumns + ` FROM sites WHERE ` + where

	site, err := scanSite(s.db.QueryRowContext(ctx, q, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return storage.Site{}, storage.ErrNotFound
	}
	if err != nil {
		return storage.Site{}, err
	}
	return site, nil
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows, so a single-row
// lookup and a list scan a site the same way and cannot drift apart.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanSite reads one row selected with siteColumns.
func scanSite(row rowScanner) (storage.Site, error) {
	var (
		site       storage.Site
		apiKey     sql.NullString
		ownerEmail sql.NullString
		createdAt  string
	)

	err := row.Scan(
		&site.ID, &site.Name, &site.Domain, &site.SiteKey,
		&apiKey, &ownerEmail, &createdAt,
	)
	if err != nil {
		// Passed through unwrapped: siteWhere needs to recognize ErrNoRows.
		return storage.Site{}, err
	}

	// Both are nullable in the schema; empty string is the honest zero value.
	site.APIKey = apiKey.String
	site.OwnerEmail = ownerEmail.String

	site.CreatedAt, err = time.Parse(timeLayout, createdAt)
	if err != nil {
		return storage.Site{}, fmt.Errorf("sqlite: parse created_at: %w", err)
	}
	return site, nil
}
