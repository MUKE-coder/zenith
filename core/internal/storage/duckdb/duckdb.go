// Package duckdb implements storage.EventStore on DuckDB.
package duckdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"

	_ "github.com/marcboeker/go-duckdb/v2"

	"github.com/zenith/core/internal/storage"
)

// Store is a DuckDB-backed event store.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the DuckDB database at path and ensures the event
// schema exists.
func Open(ctx context.Context, path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := ensureDir(dir); err != nil {
			return nil, fmt.Errorf("duckdb: data dir: %w", err)
		}
	}

	db, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, fmt.Errorf("duckdb: open: %w", err)
	}

	// DuckDB is single-writer; more connections buy contention, not throughput.
	db.SetMaxOpenConns(1)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("duckdb: ping: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// migrate creates the events schema.
//
// One wide, denormalized table is the point: DuckDB is columnar, so a query
// that reads three columns pays for three columns, and every dashboard number
// is a GROUP BY over exactly this shape. Joins would buy normalization that
// nothing here needs.
//
// Note what is absent. There is no IP column and no persistent visitor id --
// visitor_hash is derived from the IP and rotates daily, and the raw address is
// never written down. The schema itself is the privacy guarantee: data that is
// never stored cannot leak, be subpoenaed, or require a consent banner.
func (s *Store) migrate(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS events (
	site_id      VARCHAR NOT NULL,
	ts           TIMESTAMP NOT NULL,
	-- 'pageview' | 'event'
	type         VARCHAR NOT NULL,
	path         VARCHAR,
	referrer     VARCHAR,
	utm_source   VARCHAR,
	utm_medium   VARCHAR,
	utm_campaign VARCHAR,
	utm_term     VARCHAR,
	utm_content  VARCHAR,
	-- ISO-3166-1 alpha-2. Country is as fine as geo ever gets.
	country      VARCHAR,
	device       VARCHAR,
	browser      VARCHAR,
	os           VARCHAR,
	-- hash(date + site + ip + ua + daily_salt). Same visitor, same day, same
	-- hash; tomorrow it is a different value and yesterday's is unlinkable.
	visitor_hash VARCHAR,
	-- Set when type = 'event'.
	event_name   VARCHAR,
	props        JSON
);`
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("duckdb: migrate: %w", err)
	}
	return nil
}

// Insert appends events to the store.
func (s *Store) Insert(ctx context.Context, events ...storage.Event) error {
	if len(events) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("duckdb: begin: %w", err)
	}
	defer tx.Rollback()

	const q = `INSERT INTO events (
		site_id, ts, type, path, referrer,
		utm_source, utm_medium, utm_campaign, utm_term, utm_content,
		country, device, browser, os, visitor_hash, event_name, props
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	stmt, err := tx.PrepareContext(ctx, q)
	if err != nil {
		return fmt.Errorf("duckdb: prepare: %w", err)
	}
	defer stmt.Close()

	for _, e := range events {
		props, err := marshalProps(e.Props)
		if err != nil {
			return err
		}
		if _, err := stmt.ExecContext(ctx,
			e.SiteID, e.TS, e.Type, e.Path, e.Referrer,
			e.UTMSource, e.UTMMedium, e.UTMCampaign, e.UTMTerm, e.UTMContent,
			e.Country, e.Device, e.Browser, e.OS, e.VisitorHash, e.EventName, props,
		); err != nil {
			return fmt.Errorf("duckdb: insert: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("duckdb: commit: %w", err)
	}
	return nil
}

// Ping verifies the store is reachable.
func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Close releases the underlying handle.
func (s *Store) Close() error {
	return s.db.Close()
}

func marshalProps(props map[string]any) (any, error) {
	if len(props) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(props)
	if err != nil {
		return nil, fmt.Errorf("duckdb: marshal props: %w", err)
	}
	return string(b), nil
}

var _ storage.EventStore = (*Store)(nil)
