// Package store is the worker's access to the shared job queue.
//
// The worker is a separate module from core, so it cannot import core's
// internal packages -- that is the language enforcing the boundary the
// architecture asks for: an out-of-memory audit must not be able to take
// analytics ingestion down with it, and two services that share a process are
// not two services.
//
// The cost is that these few queries duplicate knowledge of a schema core
// owns. That is the trade: core runs the migrations, the worker only reads the
// queue and writes results, and the surface is deliberately this small.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// timeLayout matches core's: ISO-8601 UTC, so both services read the same
// timestamps back.
const timeLayout = "2006-01-02T15:04:05Z"

// Job statuses, matching core's.
const (
	StatusQueued  = "queued"
	StatusRunning = "running"
	StatusDone    = "done"
	StatusFailed  = "failed"
)

// ErrNoJob means the queue is empty.
var ErrNoJob = errors.New("store: no queued jobs")

// Job is a claimed audit.
type Job struct {
	ID     string
	SiteID string

	// SiteName and SiteDomain come from the site the job names.
	SiteName   string
	SiteDomain string
}

// PageResult is one audited page, ready to store.
type PageResult struct {
	URL    string
	Score  int
	Checks string
}

// Store is the worker's SQLite handle.
type Store struct {
	db *sql.DB
}

// Open opens the shared application database.
func Open(ctx context.Context, path string) (*Store, error) {
	// Same pragmas as core: WAL so the worker's writes do not block core's
	// reads, and a busy timeout to absorb the contention that remains rather
	// than surfacing SQLITE_BUSY as a failed audit.
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: ping: %w", err)
	}
	return &Store{db: db}, nil
}

// Close releases the handle.
func (s *Store) Close() error { return s.db.Close() }

// Claim takes the oldest queued job and marks it running.
//
// The UPDATE and the SELECT are one statement, so two workers racing for the
// last job cannot both win: SQLite serializes writers, and the second one's
// subquery finds nothing left to claim.
func (s *Store) Claim(ctx context.Context) (Job, error) {
	const q = `
UPDATE audit_jobs
SET status = ?, started_at = ?
WHERE id = (
	SELECT id FROM audit_jobs
	WHERE status = ?
	ORDER BY requested_at ASC
	LIMIT 1
)
RETURNING id, site_id`

	var job Job
	err := s.db.QueryRowContext(ctx, q,
		StatusRunning, time.Now().UTC().Format(timeLayout), StatusQueued,
	).Scan(&job.ID, &job.SiteID)

	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrNoJob
	}
	if err != nil {
		return Job{}, fmt.Errorf("store: claim: %w", err)
	}

	const siteQuery = `SELECT name, domain FROM sites WHERE id = ?`
	if err := s.db.QueryRowContext(ctx, siteQuery, job.SiteID).
		Scan(&job.SiteName, &job.SiteDomain); err != nil {
		return Job{}, fmt.Errorf("store: site for job %s: %w", job.ID, err)
	}

	return job, nil
}

// SaveResults stores a job's per-page findings.
func (s *Store) SaveResults(ctx context.Context, jobID string, results []PageResult) error {
	if len(results) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin: %w", err)
	}
	defer tx.Rollback()

	// A page is audited once per job. The upsert makes a re-run of the same
	// job idempotent rather than a constraint violation.
	const q = `
INSERT INTO audit_results (id, job_id, page_url, checks, score)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (job_id, page_url) DO UPDATE SET
	checks = excluded.checks,
	score  = excluded.score`

	stmt, err := tx.PrepareContext(ctx, q)
	if err != nil {
		return fmt.Errorf("store: prepare: %w", err)
	}
	defer stmt.Close()

	for i, result := range results {
		resultID := fmt.Sprintf("%s-%d", jobID, i)
		if _, err := stmt.ExecContext(ctx, resultID, jobID, result.URL, result.Checks, result.Score); err != nil {
			return fmt.Errorf("store: save result: %w", err)
		}
	}

	return tx.Commit()
}

// Finish marks a job done with a site score.
func (s *Store) Finish(ctx context.Context, jobID string, score int) error {
	const q = `UPDATE audit_jobs SET status = ?, finished_at = ?, score = ?, error = NULL WHERE id = ?`

	_, err := s.db.ExecContext(ctx, q,
		StatusDone, time.Now().UTC().Format(timeLayout), score, jobID)
	if err != nil {
		return fmt.Errorf("store: finish: %w", err)
	}
	return nil
}

// Fail marks a job failed with a reason.
//
// The reason reaches the console, so it is written for the developer reading
// it there, not for a log.
func (s *Store) Fail(ctx context.Context, jobID, reason string) error {
	const q = `UPDATE audit_jobs SET status = ?, finished_at = ?, error = ? WHERE id = ?`

	_, err := s.db.ExecContext(ctx, q,
		StatusFailed, time.Now().UTC().Format(timeLayout), reason, jobID)
	if err != nil {
		return fmt.Errorf("store: fail: %w", err)
	}
	return nil
}

// ReclaimStale requeues jobs left running by a worker that died.
//
// A crashed worker leaves its job marked running forever, and nothing else
// will ever pick it up -- the claim only looks at queued. Without this, one
// crash means one audit that hangs at "running" until someone notices.
// Returns how many were requeued.
func (s *Store) ReclaimStale(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-olderThan).Format(timeLayout)

	const q = `
UPDATE audit_jobs
SET status = ?, started_at = NULL
WHERE status = ? AND started_at IS NOT NULL AND started_at < ?`

	res, err := s.db.ExecContext(ctx, q, StatusQueued, StatusRunning, cutoff)
	if err != nil {
		return 0, fmt.Errorf("store: reclaim: %w", err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return 0, nil
	}
	return n, nil
}
