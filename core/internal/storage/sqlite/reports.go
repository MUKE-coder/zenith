package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/zenith/core/internal/storage"
)

const reportColumns = `id, site_id, period, sent_at, status, COALESCE(error, '')`

// RecordReport stores the outcome of a monthly send.
//
// Upsert on (site_id, period), which the schema already enforces as unique.
// That is the whole anti-duplicate design: a retry after a failure updates the
// row rather than adding a second one, and the scheduler reads the row to
// decide whether this month has already gone out.
func (s *Store) RecordReport(ctx context.Context, r storage.Report) error {
	if r.ID == "" || r.SiteID == "" || r.Period == "" {
		return errors.New("sqlite: report needs an id, site, and period")
	}
	if r.Status != storage.ReportSent && r.Status != storage.ReportFailed {
		return fmt.Errorf("sqlite: invalid report status %q", r.Status)
	}

	sentAt := r.SentAt
	if sentAt.IsZero() {
		sentAt = time.Now().UTC()
	}

	const q = `
INSERT INTO report_history (id, site_id, period, sent_at, status, error)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (site_id, period) DO UPDATE SET
	sent_at = excluded.sent_at,
	status  = excluded.status,
	error   = excluded.error`

	_, err := s.db.ExecContext(ctx, q,
		r.ID, r.SiteID, r.Period, sentAt.Format(timeLayout), r.Status, nullable(r.Err))
	if err != nil {
		return fmt.Errorf("sqlite: record report: %w", err)
	}
	return nil
}

// ReportFor returns a site's report for a period.
func (s *Store) ReportFor(ctx context.Context, siteID, period string) (storage.Report, error) {
	const q = `SELECT ` + reportColumns + ` FROM report_history WHERE site_id = ? AND period = ?`

	report, err := scanReport(s.db.QueryRowContext(ctx, q, siteID, period))
	if errors.Is(err, sql.ErrNoRows) {
		return storage.Report{}, storage.ErrNotFound
	}
	if err != nil {
		return storage.Report{}, err
	}
	return report, nil
}

// ReportsForSite returns a site's report history, newest first.
func (s *Store) ReportsForSite(ctx context.Context, siteID string) ([]storage.Report, error) {
	const q = `SELECT ` + reportColumns + ` FROM report_history WHERE site_id = ? ORDER BY period DESC`

	rows, err := s.db.QueryContext(ctx, q, siteID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: reports for site: %w", err)
	}
	defer rows.Close()

	reports := []storage.Report{}
	for rows.Next() {
		report, err := scanReport(rows)
		if err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	return reports, rows.Err()
}

func scanReport(row rowScanner) (storage.Report, error) {
	var (
		r      storage.Report
		sentAt sql.NullString
	)

	if err := row.Scan(&r.ID, &r.SiteID, &r.Period, &sentAt, &r.Status, &r.Err); err != nil {
		return storage.Report{}, err
	}

	if sentAt.Valid {
		if parsed, err := time.Parse(timeLayout, sentAt.String); err == nil {
			r.SentAt = parsed
		}
	}
	return r, nil
}
