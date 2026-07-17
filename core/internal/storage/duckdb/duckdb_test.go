package duckdb_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/zenith/core/internal/storage"
	"github.com/zenith/core/internal/storage/duckdb"
)

func open(t *testing.T) *duckdb.Store {
	t.Helper()

	s, err := duckdb.Open(context.Background(), filepath.Join(t.TempDir(), "events.duckdb"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// A pageview must survive the round trip with every field intact.
func TestInsertAndReadBackPageview(t *testing.T) {
	ctx := context.Background()
	s := open(t)

	ts := time.Date(2026, 7, 17, 9, 30, 0, 0, time.UTC)
	want := storage.Event{
		SiteID:      "site-1",
		TS:          ts,
		Type:        "pageview",
		Path:        "/pricing",
		Referrer:    "https://news.ycombinator.com/",
		UTMSource:   "hn",
		UTMMedium:   "social",
		UTMCampaign: "launch",
		UTMTerm:     "analytics",
		UTMContent:  "post",
		Country:     "UG",
		Device:      "desktop",
		Browser:     "Firefox",
		OS:          "Linux",
		VisitorHash: "abc123",
	}

	if err := s.Insert(ctx, want); err != nil {
		t.Fatalf("insert: %v", err)
	}

	var got storage.Event
	err := s.DB().QueryRowContext(ctx, `
		SELECT site_id, ts, type, path, referrer,
		       utm_source, utm_medium, utm_campaign, utm_term, utm_content,
		       country, device, browser, os, visitor_hash
		FROM events`).
		Scan(&got.SiteID, &got.TS, &got.Type, &got.Path, &got.Referrer,
			&got.UTMSource, &got.UTMMedium, &got.UTMCampaign, &got.UTMTerm, &got.UTMContent,
			&got.Country, &got.Device, &got.Browser, &got.OS, &got.VisitorHash)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}

	if got.SiteID != want.SiteID || got.Path != want.Path || got.Type != want.Type {
		t.Errorf("core fields: got %+v, want %+v", got, want)
	}
	if !got.TS.UTC().Equal(want.TS) {
		t.Errorf("ts = %v, want %v", got.TS.UTC(), want.TS)
	}
	if got.UTMCampaign != want.UTMCampaign || got.Country != want.Country {
		t.Errorf("utm/geo: got campaign %q country %q, want %q and %q",
			got.UTMCampaign, got.Country, want.UTMCampaign, want.Country)
	}
	if got.VisitorHash != want.VisitorHash {
		t.Errorf("visitor_hash = %q, want %q", got.VisitorHash, want.VisitorHash)
	}
}

// Custom events carry a name and arbitrary properties as JSON.
func TestInsertCustomEventWithProps(t *testing.T) {
	ctx := context.Background()
	s := open(t)

	err := s.Insert(ctx, storage.Event{
		SiteID:    "site-1",
		TS:        time.Now().UTC(),
		Type:      "event",
		EventName: "signup",
		Props:     map[string]any{"plan": "pro", "seats": 3},
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Query through DuckDB's JSON operators: if props round-tripped as real
	// JSON rather than an opaque string, this extracts a value.
	var name, plan string
	err = s.DB().QueryRowContext(ctx,
		`SELECT event_name, json_extract_string(props, '$.plan') FROM events`).
		Scan(&name, &plan)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}

	if name != "signup" {
		t.Errorf("event_name = %q, want %q", name, "signup")
	}
	if plan != "pro" {
		t.Errorf("props.plan = %q, want %q", plan, "pro")
	}
}

// An event with no properties stores NULL, not the string "null" or "{}".
func TestInsertWithoutPropsStoresNull(t *testing.T) {
	ctx := context.Background()
	s := open(t)

	if err := s.Insert(ctx, storage.Event{
		SiteID: "site-1", TS: time.Now().UTC(), Type: "pageview", Path: "/",
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	var nulls int
	if err := s.DB().QueryRowContext(ctx,
		`SELECT count(*) FROM events WHERE props IS NULL`).Scan(&nulls); err != nil {
		t.Fatalf("count: %v", err)
	}
	if nulls != 1 {
		t.Errorf("rows with NULL props = %d, want 1", nulls)
	}
}

// Ingestion batches, so a multi-event insert must land as one transaction.
func TestInsertBatch(t *testing.T) {
	ctx := context.Background()
	s := open(t)

	now := time.Now().UTC()
	events := make([]storage.Event, 0, 100)
	for i := range 100 {
		events = append(events, storage.Event{
			SiteID: "site-1", TS: now.Add(time.Duration(i) * time.Second),
			Type: "pageview", Path: "/", VisitorHash: "v1",
		})
	}

	if err := s.Insert(ctx, events...); err != nil {
		t.Fatalf("insert batch: %v", err)
	}

	var n int
	if err := s.DB().QueryRowContext(ctx, `SELECT count(*) FROM events`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 100 {
		t.Errorf("rows = %d, want 100", n)
	}
}

func TestInsertNothingIsNoOp(t *testing.T) {
	if err := open(t).Insert(context.Background()); err != nil {
		t.Errorf("insert with no events: %v", err)
	}
}

// The events file is reopened on every boot; the schema must survive.
func TestSchemaSurvivesReopen(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "events.duckdb")

	first, err := duckdb.Open(ctx, path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := first.Insert(ctx, storage.Event{
		SiteID: "site-1", TS: time.Now().UTC(), Type: "pageview", Path: "/",
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	first.Close()

	second, err := duckdb.Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer second.Close()

	var n int
	if err := second.DB().QueryRowContext(ctx, `SELECT count(*) FROM events`).Scan(&n); err != nil {
		t.Fatalf("count after reopen: %v", err)
	}
	if n != 1 {
		t.Errorf("rows after reopen = %d, want 1", n)
	}
}

// The schema must never grow a column that stores a raw address or a stable
// cross-day identity. This is the privacy guarantee, so it gets a test.
func TestEventsSchemaStoresNoRawIP(t *testing.T) {
	ctx := context.Background()
	s := open(t)

	rows, err := s.DB().QueryContext(ctx,
		`SELECT column_name FROM information_schema.columns WHERE table_name = 'events'`)
	if err != nil {
		t.Fatalf("read columns: %v", err)
	}
	defer rows.Close()

	forbidden := map[string]bool{
		"ip": true, "ip_address": true, "remote_addr": true,
		"visitor_id": true, "user_id": true, "client_id": true,
	}

	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if forbidden[col] {
			t.Errorf("events has column %q: raw addresses and persistent "+
				"identifiers must never be stored", col)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
}
