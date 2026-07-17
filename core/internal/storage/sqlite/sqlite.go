// Package sqlite implements storage.AppStore on SQLite.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	_ "modernc.org/sqlite"

	"github.com/zenith/core/internal/storage"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Store is a SQLite-backed application store.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path.
func Open(ctx context.Context, path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("sqlite: data dir: %w", err)
		}
	}

	// WAL keeps reads from blocking the writer; busy_timeout absorbs the brief
	// contention that remains instead of surfacing SQLITE_BUSY to a request.
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: ping: %w", err)
	}
	return &Store{db: db}, nil
}

// Migrate applies every migration that has not yet run, in version order.
//
// Each migration is recorded in schema_migrations inside the same transaction
// that applies it, so a migration either lands and is recorded or does neither.
// Re-running is therefore a no-op.
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`); err != nil {
		return fmt.Errorf("sqlite: migration table: %w", err)
	}

	applied, err := s.appliedVersions(ctx)
	if err != nil {
		return err
	}

	names, err := migrationNames()
	if err != nil {
		return err
	}

	for _, name := range names {
		if applied[name] {
			continue
		}
		body, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("sqlite: read migration %s: %w", name, err)
		}
		if err := s.applyOne(ctx, name, string(body)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) applyOne(ctx context.Context, name, body string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin %s: %w", name, err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, body); err != nil {
		return fmt.Errorf("sqlite: apply %s: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version) VALUES (?)`, name); err != nil {
		return fmt.Errorf("sqlite: record %s: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit %s: %w", name, err)
	}
	return nil
}

func (s *Store) appliedVersions(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: read migrations: %w", err)
	}
	defer rows.Close()

	applied := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("sqlite: scan migration: %w", err)
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

func migrationNames() ([]string, error) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("sqlite: list migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// Ping verifies the store is reachable.
func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Close releases the underlying handle.
func (s *Store) Close() error {
	return s.db.Close()
}

var _ storage.AppStore = (*Store)(nil)
