-- SEO audit queue.
--
-- This table *is* the queue: the worker polls it. At one developer's scale a
-- broker would be infrastructure to operate for no gain.

CREATE TABLE audit_jobs (
	id           TEXT PRIMARY KEY,
	site_id      TEXT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
	status       TEXT NOT NULL DEFAULT 'queued'
	             CHECK (status IN ('queued', 'running', 'done', 'failed')),
	requested_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
	-- Set when a worker claims the job. Also how a crashed worker's job is
	-- found: still 'running', started_at long past, no finished_at.
	started_at   TEXT,
	finished_at  TEXT,
	-- Site-wide score, 0-100. Per-page scores live on audit_results.
	score        INTEGER,
	-- Why it failed, in the interface's voice -- this reaches the console.
	error        TEXT
);

-- The worker's poll: oldest queued job first.
CREATE INDEX idx_audit_jobs_claim ON audit_jobs (status, requested_at);

-- The console's view: this site's audits, newest first.
CREATE INDEX idx_audit_jobs_site ON audit_jobs (site_id, requested_at DESC);
