package duckdb

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/zenith/core/internal/storage"
)

// defaultLimit caps a breakdown when the caller does not.
const defaultLimit = 50

// maxLimit is the most rows any breakdown will return. A dashboard panel shows
// a handful; nobody needs ten thousand referrer rows, and refusing to build
// them is what keeps one URL from pinning the server.
const maxLimit = 500

// sessionizedPageviews marks each pageview with the session it belongs to.
//
// Sessions are derived, not stored. A session is a run of one visitor's events
// with no gap longer than SessionGap: the lag() marks each gap as a new
// session start, and the running sum turns those marks into a session number.
// Deriving costs a window function per query but keeps ingestion a plain
// append -- and, more importantly, means a session concept can change without
// a migration or a backfill.
const sessionizedPageviews = `
WITH scoped AS (
	SELECT visitor_hash, ts, path
	FROM events
	WHERE site_id = ? AND ts >= ? AND ts < ? AND type = 'pageview'
),
marked AS (
	SELECT visitor_hash, ts, path,
		CASE
			WHEN lag(ts) OVER w IS NULL THEN 1
			WHEN ts - lag(ts) OVER w > INTERVAL %d MINUTE THEN 1
			ELSE 0
		END AS starts
	FROM scoped
	WINDOW w AS (PARTITION BY visitor_hash ORDER BY ts)
),
sessionized AS (
	SELECT visitor_hash, ts, path,
		sum(starts) OVER (
			PARTITION BY visitor_hash ORDER BY ts ROWS UNBOUNDED PRECEDING
		) AS session_num
	FROM marked
)`

func sessionGapMinutes() int {
	return int(storage.SessionGap / time.Minute)
}

// Summary returns pageviews, unique visitors, and sessions for a period.
func (s *Store) Summary(ctx context.Context, q storage.Query) (storage.Summary, error) {
	if err := validate(q); err != nil {
		return storage.Summary{}, err
	}

	query := fmt.Sprintf(`
WITH scoped AS (
	SELECT visitor_hash, ts, type
	FROM events
	WHERE site_id = ? AND ts >= ? AND ts < ?
),
marked AS (
	SELECT visitor_hash,
		CASE
			WHEN lag(ts) OVER w IS NULL THEN 1
			WHEN ts - lag(ts) OVER w > INTERVAL %d MINUTE THEN 1
			ELSE 0
		END AS starts
	FROM scoped
	WINDOW w AS (PARTITION BY visitor_hash ORDER BY ts)
)
SELECT
	(SELECT count(*) FILTER (WHERE type = 'pageview') FROM scoped),
	(SELECT count(DISTINCT visitor_hash) FROM scoped),
	(SELECT COALESCE(sum(starts), 0) FROM marked)`, sessionGapMinutes())

	var out storage.Summary
	err := s.db.QueryRowContext(ctx, query, q.SiteID, q.From, q.To).
		Scan(&out.Pageviews, &out.Visitors, &out.Sessions)
	if err != nil {
		return storage.Summary{}, fmt.Errorf("duckdb: summary: %w", err)
	}
	return out, nil
}

// Timeseries returns traffic bucketed by g.
//
// Empty buckets are filled with zeroes. Without that a quiet Tuesday would not
// appear at all and the chart would draw a line straight from Monday to
// Wednesday, silently hiding the gap it exists to show.
func (s *Store) Timeseries(ctx context.Context, q storage.Query, g storage.Granularity) ([]storage.Bucket, error) {
	if err := validate(q); err != nil {
		return nil, err
	}
	if !g.Valid() {
		return nil, fmt.Errorf("duckdb: unsupported granularity %q", g)
	}

	// g is interpolated, never bound: date_trunc takes a literal unit. Valid()
	// above is what makes that safe.
	query := fmt.Sprintf(`
SELECT date_trunc('%s', ts) AS bucket,
       count(*) FILTER (WHERE type = 'pageview') AS pageviews,
       count(DISTINCT visitor_hash) AS visitors
FROM events
WHERE site_id = ? AND ts >= ? AND ts < ?
GROUP BY bucket
ORDER BY bucket`, g)

	rows, err := s.db.QueryContext(ctx, query, q.SiteID, q.From, q.To)
	if err != nil {
		return nil, fmt.Errorf("duckdb: timeseries: %w", err)
	}
	defer rows.Close()

	found := map[time.Time]storage.Bucket{}
	for rows.Next() {
		var b storage.Bucket
		if err := rows.Scan(&b.TS, &b.Pageviews, &b.Visitors); err != nil {
			return nil, fmt.Errorf("duckdb: scan bucket: %w", err)
		}
		found[b.TS.UTC()] = b
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("duckdb: timeseries rows: %w", err)
	}

	return fillBuckets(q.From, q.To, g, found), nil
}

// fillBuckets returns one bucket per interval in [from, to), zero where no
// events landed.
func fillBuckets(from, to time.Time, g storage.Granularity, found map[time.Time]storage.Bucket) []storage.Bucket {
	out := []storage.Bucket{}

	for cursor := truncate(from.UTC(), g); cursor.Before(to.UTC()); cursor = advance(cursor, g) {
		if b, ok := found[cursor]; ok {
			out = append(out, b)
			continue
		}
		out = append(out, storage.Bucket{TS: cursor})
	}
	return out
}

// truncate rounds t down to the start of its bucket, matching date_trunc.
func truncate(t time.Time, g storage.Granularity) time.Time {
	switch g {
	case storage.GranularityHour:
		return t.Truncate(time.Hour)
	case storage.GranularityDay:
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	case storage.GranularityWeek:
		// DuckDB's date_trunc('week') starts weeks on Monday; Go's Weekday
		// starts them on Sunday. Align to Monday or the fill would miss every
		// bucket the query returned.
		day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		offset := (int(day.Weekday()) + 6) % 7
		return day.AddDate(0, 0, -offset)
	case storage.GranularityMonth:
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
	return t
}

func advance(t time.Time, g storage.Granularity) time.Time {
	switch g {
	case storage.GranularityHour:
		return t.Add(time.Hour)
	case storage.GranularityDay:
		return t.AddDate(0, 0, 1)
	case storage.GranularityWeek:
		return t.AddDate(0, 0, 7)
	case storage.GranularityMonth:
		return t.AddDate(0, 1, 0)
	}
	return t.AddDate(0, 0, 1)
}

// Breakdown groups traffic by one dimension, busiest first.
func (s *Store) Breakdown(ctx context.Context, q storage.Query, d storage.Dimension) ([]storage.Count, error) {
	if err := validate(q); err != nil {
		return nil, err
	}
	if !d.Valid() {
		return nil, fmt.Errorf("duckdb: unsupported dimension %q", d)
	}

	args := []any{q.SiteID, q.From, q.To}
	filters := ""

	// A page is something that was viewed. Custom events carry the path they
	// fired on, and counting those here would list pages nobody ever loaded --
	// a "/signup" row with zero pageviews, sitting in a table of top pages.
	// Which pages an event fired on is the events panel's question, not this
	// one's.
	if d == storage.DimPath {
		filters += " AND type = 'pageview'"
	}

	// A site's own domain is its biggest "referrer" because internal
	// navigation sets one. Excluding it is what makes the panel answer the
	// question it is asking: where did people come from?
	if d == storage.DimReferrer && q.ExcludeReferrer != "" {
		filters += " AND referrer != ?"
		args = append(args, q.ExcludeReferrer)
	}

	// Rows where the dimension is empty are not a category, they are an
	// absence: "no referrer" is direct traffic, and a blank row would just be
	// noise at the top of the table.
	//
	// d is interpolated, never bound: it is a column name. Valid() is what
	// makes that safe.
	query := fmt.Sprintf(`
SELECT %[1]s AS label,
       count(DISTINCT visitor_hash) AS visitors,
       count(*) FILTER (WHERE type = 'pageview') AS pageviews
FROM events
WHERE site_id = ? AND ts >= ? AND ts < ?
  AND %[1]s IS NOT NULL AND %[1]s != ''%[2]s
GROUP BY label
ORDER BY %[3]s
LIMIT ?`, string(d), filters, orderFor(d))

	args = append(args, limitOf(q))

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("duckdb: breakdown %s: %w", d, err)
	}
	defer rows.Close()

	return scanCounts(rows)
}

// orderFor returns the ORDER BY for a breakdown, busiest first.
//
// Which number is "busiest" depends on the question. A top-pages table is
// ranked by views -- that is what makes a page top -- while a referrer or a
// country is ranked by how many people came. Getting this wrong is not just a
// display quirk: the ranking decides which rows survive the LIMIT, so a panel
// sorted here by one number and shown by another can drop the very rows it
// should lead with.
//
// The second term breaks ties on the other number, and the label breaks the
// rest, so the order is total and a redraw never shuffles equal rows.
func orderFor(d storage.Dimension) string {
	if d == storage.DimPath {
		return "pageviews DESC, visitors DESC, label ASC"
	}
	return "visitors DESC, pageviews DESC, label ASC"
}

// EntryPages returns the pages sessions started on.
func (s *Store) EntryPages(ctx context.Context, q storage.Query) ([]storage.Count, error) {
	return s.edgePages(ctx, q, "ASC")
}

// ExitPages returns the pages sessions ended on.
func (s *Store) ExitPages(ctx context.Context, q storage.Query) ([]storage.Count, error) {
	return s.edgePages(ctx, q, "DESC")
}

// edgePages returns the first (ASC) or last (DESC) page of each session.
func (s *Store) edgePages(ctx context.Context, q storage.Query, direction string) ([]storage.Count, error) {
	if err := validate(q); err != nil {
		return nil, err
	}
	// Not caller-controlled -- the two exported methods pass a literal -- but
	// asserted anyway so a future caller cannot make it so.
	if direction != "ASC" && direction != "DESC" {
		return nil, fmt.Errorf("duckdb: bad direction %q", direction)
	}

	query := fmt.Sprintf(sessionizedPageviews+`,
ranked AS (
	SELECT path, visitor_hash,
		row_number() OVER (PARTITION BY visitor_hash, session_num ORDER BY ts %s) AS rn
	FROM sessionized
)
SELECT path AS label,
       count(DISTINCT visitor_hash) AS visitors,
       count(*) AS pageviews
FROM ranked
WHERE rn = 1 AND path IS NOT NULL AND path != ''
GROUP BY label
ORDER BY pageviews DESC, label ASC
LIMIT ?`, sessionGapMinutes(), direction)

	rows, err := s.db.QueryContext(ctx, query, q.SiteID, q.From, q.To, limitOf(q))
	if err != nil {
		return nil, fmt.Errorf("duckdb: edge pages: %w", err)
	}
	defer rows.Close()

	return scanCounts(rows)
}

// Events returns custom event counts.
func (s *Store) Events(ctx context.Context, q storage.Query) ([]storage.EventStat, error) {
	if err := validate(q); err != nil {
		return nil, err
	}

	const query = `
SELECT event_name,
       count(*) AS count,
       count(DISTINCT visitor_hash) AS visitors
FROM events
WHERE site_id = ? AND ts >= ? AND ts < ?
  AND type = 'event' AND event_name IS NOT NULL AND event_name != ''
GROUP BY event_name
ORDER BY count DESC, event_name ASC
LIMIT ?`

	rows, err := s.db.QueryContext(ctx, query, q.SiteID, q.From, q.To, limitOf(q))
	if err != nil {
		return nil, fmt.Errorf("duckdb: events: %w", err)
	}
	defer rows.Close()

	out := []storage.EventStat{}
	for rows.Next() {
		var e storage.EventStat
		if err := rows.Scan(&e.Name, &e.Count, &e.Visitors); err != nil {
			return nil, fmt.Errorf("duckdb: scan event: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// EventProps returns the property breakdown for one custom event.
func (s *Store) EventProps(ctx context.Context, q storage.Query, name string) ([]storage.PropStat, error) {
	if err := validate(q); err != nil {
		return nil, err
	}
	if name == "" {
		return nil, fmt.Errorf("duckdb: event name is required")
	}

	// unnest(json_keys(props)) turns one row with N properties into N rows, so
	// each key/value pair can be counted independently. The key is quoted into
	// a JSON path -- it comes from stored data, not from a URL, and ingestion
	// already capped its length and shape.
	const query = `
SELECT k AS key,
       json_extract_string(props, '$."' || k || '"') AS value,
       count(*) AS count
FROM (
	SELECT props, unnest(json_keys(props)) AS k
	FROM events
	WHERE site_id = ? AND ts >= ? AND ts < ?
	  AND type = 'event' AND event_name = ? AND props IS NOT NULL
)
GROUP BY key, value
ORDER BY count DESC, key ASC, value ASC
LIMIT ?`

	rows, err := s.db.QueryContext(ctx, query, q.SiteID, q.From, q.To, name, limitOf(q))
	if err != nil {
		return nil, fmt.Errorf("duckdb: event props: %w", err)
	}
	defer rows.Close()

	out := []storage.PropStat{}
	for rows.Next() {
		var p storage.PropStat
		var value sql.NullString
		if err := rows.Scan(&p.Key, &value, &p.Count); err != nil {
			return nil, fmt.Errorf("duckdb: scan prop: %w", err)
		}
		p.Value = value.String
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeleteSite removes every event for a site.
func (s *Store) DeleteSite(ctx context.Context, siteID string) error {
	if siteID == "" {
		return fmt.Errorf("duckdb: site id is required")
	}

	if _, err := s.db.ExecContext(ctx, `DELETE FROM events WHERE site_id = ?`, siteID); err != nil {
		return fmt.Errorf("duckdb: delete site events: %w", err)
	}
	return nil
}

// Realtime returns visitors active within the last storage.RealtimeWindow.
func (s *Store) Realtime(ctx context.Context, siteID string) (int64, error) {
	if siteID == "" {
		return 0, fmt.Errorf("duckdb: site id is required")
	}

	const query = `
SELECT count(DISTINCT visitor_hash)
FROM events
WHERE site_id = ? AND ts >= ?`

	since := time.Now().UTC().Add(-storage.RealtimeWindow)

	var n int64
	if err := s.db.QueryRowContext(ctx, query, siteID, since).Scan(&n); err != nil {
		return 0, fmt.Errorf("duckdb: realtime: %w", err)
	}
	return n, nil
}

// validate rejects a query that could not be answered safely.
func validate(q storage.Query) error {
	// The one invariant that matters: no site, no query. A store method that
	// ran without a site id would read every client's traffic at once.
	if q.SiteID == "" {
		return fmt.Errorf("duckdb: site id is required")
	}
	if q.From.IsZero() || q.To.IsZero() {
		return fmt.Errorf("duckdb: query needs a from and a to")
	}
	if !q.To.After(q.From) {
		return fmt.Errorf("duckdb: to (%s) must be after from (%s)", q.To, q.From)
	}
	return nil
}

func limitOf(q storage.Query) int {
	switch {
	case q.Limit <= 0:
		return defaultLimit
	case q.Limit > maxLimit:
		return maxLimit
	default:
		return q.Limit
	}
}

func scanCounts(rows *sql.Rows) ([]storage.Count, error) {
	// Never nil: an empty breakdown must encode as [] rather than null, or
	// every consumer needs a null check the empty case does not warrant.
	out := []storage.Count{}

	for rows.Next() {
		var c storage.Count
		if err := rows.Scan(&c.Label, &c.Visitors, &c.Pageviews); err != nil {
			return nil, fmt.Errorf("duckdb: scan count: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
