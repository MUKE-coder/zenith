package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/zenith/core/internal/storage"
)

const auditJobColumns = `id, site_id, status, requested_at, started_at, finished_at,
	COALESCE(score, 0), COALESCE(error, '')`

// CreateAuditJob enqueues an audit.
func (s *Store) CreateAuditJob(ctx context.Context, job storage.AuditJob) error {
	if job.ID == "" || job.SiteID == "" {
		return errors.New("sqlite: audit job needs an id and a site")
	}

	requestedAt := job.RequestedAt
	if requestedAt.IsZero() {
		requestedAt = time.Now().UTC()
	}

	const q = `INSERT INTO audit_jobs (id, site_id, status, requested_at)
	           VALUES (?, ?, ?, ?)`

	_, err := s.db.ExecContext(ctx, q,
		job.ID, job.SiteID, storage.AuditQueued, requestedAt.Format(timeLayout))
	if err != nil {
		return fmt.Errorf("sqlite: create audit job: %w", err)
	}
	return nil
}

// AuditJobByID returns one job.
func (s *Store) AuditJobByID(ctx context.Context, id string) (storage.AuditJob, error) {
	const q = `SELECT ` + auditJobColumns + ` FROM audit_jobs WHERE id = ?`

	job, err := scanAuditJob(s.db.QueryRowContext(ctx, q, id))
	if errors.Is(err, sql.ErrNoRows) {
		return storage.AuditJob{}, storage.ErrNotFound
	}
	if err != nil {
		return storage.AuditJob{}, err
	}
	return job, nil
}

// ActiveAuditForSite returns a site's queued or running audit.
func (s *Store) ActiveAuditForSite(ctx context.Context, siteID string) (storage.AuditJob, error) {
	const q = `SELECT ` + auditJobColumns + ` FROM audit_jobs
	           WHERE site_id = ? AND status IN (?, ?)
	           ORDER BY requested_at ASC LIMIT 1`

	job, err := scanAuditJob(s.db.QueryRowContext(ctx, q, siteID, storage.AuditQueued, storage.AuditRunning))
	if errors.Is(err, sql.ErrNoRows) {
		return storage.AuditJob{}, storage.ErrNotFound
	}
	if err != nil {
		return storage.AuditJob{}, err
	}
	return job, nil
}

// AuditJobsForSite returns a site's audits, newest first.
func (s *Store) AuditJobsForSite(ctx context.Context, siteID string) ([]storage.AuditJob, error) {
	const q = `SELECT ` + auditJobColumns + ` FROM audit_jobs
	           WHERE site_id = ? ORDER BY requested_at DESC LIMIT 50`

	rows, err := s.db.QueryContext(ctx, q, siteID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: audit jobs: %w", err)
	}
	defer rows.Close()

	jobs := []storage.AuditJob{}
	for rows.Next() {
		job, err := scanAuditJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

// AuditResultsForJob returns a job's per-page findings, worst first.
func (s *Store) AuditResultsForJob(ctx context.Context, jobID string) ([]storage.AuditResult, error) {
	// Worst first: the pages that need work are the reason anyone opened this.
	const q = `SELECT id, job_id, page_url, checks, COALESCE(score, 0)
	           FROM audit_results WHERE job_id = ?
	           ORDER BY score ASC, page_url ASC`

	rows, err := s.db.QueryContext(ctx, q, jobID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: audit results: %w", err)
	}
	defer rows.Close()

	results := []storage.AuditResult{}
	for rows.Next() {
		var r storage.AuditResult
		if err := rows.Scan(&r.ID, &r.JobID, &r.PageURL, &r.Checks, &r.Score); err != nil {
			return nil, fmt.Errorf("sqlite: scan audit result: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func scanAuditJob(row rowScanner) (storage.AuditJob, error) {
	var (
		job         storage.AuditJob
		requestedAt string
		startedAt   sql.NullString
		finishedAt  sql.NullString
	)

	err := row.Scan(&job.ID, &job.SiteID, &job.Status,
		&requestedAt, &startedAt, &finishedAt, &job.Score, &job.Err)
	if err != nil {
		return storage.AuditJob{}, err
	}

	job.RequestedAt = parseTimeOr(requestedAt)
	if startedAt.Valid {
		job.StartedAt = parseTimeOr(startedAt.String)
	}
	if finishedAt.Valid {
		job.FinishedAt = parseTimeOr(finishedAt.String)
	}
	return job, nil
}

func parseTimeOr(v string) time.Time {
	t, err := time.Parse(timeLayout, v)
	if err != nil {
		return time.Time{}
	}
	return t
}
