-- Per-page audit findings. One row per page the worker rendered.

CREATE TABLE audit_results (
	id       TEXT PRIMARY KEY,
	job_id   TEXT NOT NULL REFERENCES audit_jobs(id) ON DELETE CASCADE,
	page_url TEXT NOT NULL,
	-- The findings for this page (title, meta, headings, alt text, canonical,
	-- robots, broken links, JSON-LD, web vitals) as JSON. The check set is
	-- expected to grow; a column per check would mean a migration per check.
	checks   TEXT NOT NULL CHECK (json_valid(checks)),
	-- This page's score, 0-100. Drives the console's score pill.
	score    INTEGER,

	-- A page is audited once per job. Makes a re-run safely re-insertable.
	UNIQUE (job_id, page_url)
);

CREATE INDEX idx_audit_results_job ON audit_results (job_id);
