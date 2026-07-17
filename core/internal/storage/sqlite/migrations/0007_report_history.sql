-- Monthly report sends.
--
-- This is the scheduler's memory. Without it a restart on the 1st of the month
-- would mail every client a second copy of their report.

CREATE TABLE report_history (
	id      TEXT PRIMARY KEY,
	site_id TEXT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
	-- The month reported on, as 'YYYY-MM'.
	period  TEXT NOT NULL CHECK (period GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]'),
	sent_at TEXT,
	status  TEXT NOT NULL CHECK (status IN ('sent', 'failed')),
	-- Why a send failed. Surfaced in the console rather than swallowed.
	error   TEXT,

	-- One report per site per month, enforced here rather than trusted to the
	-- scheduler's timing. A retry after a failure updates this row; it cannot
	-- become a second email.
	UNIQUE (site_id, period)
);

CREATE INDEX idx_report_history_site ON report_history (site_id, period DESC);
