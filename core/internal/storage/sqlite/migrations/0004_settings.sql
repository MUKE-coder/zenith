-- Global settings. Resend is configured once, for every site (v1 has no
-- per-site override).

CREATE TABLE settings (
	-- Singleton, enforced by the schema rather than by convention: there is
	-- exactly one settings row and it is always id = 1.
	id             INTEGER PRIMARY KEY CHECK (id = 1),
	-- A secret. Masked in the UI, never logged, never returned by the API.
	resend_api_key TEXT,
	mail_from      TEXT,
	updated_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

-- The row always exists, so settings reads are a plain SELECT and writes are a
-- plain UPDATE -- no upsert, no "missing row" branch.
INSERT INTO settings (id) VALUES (1);
