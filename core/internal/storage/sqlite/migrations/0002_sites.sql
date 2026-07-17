-- Tracked websites. One row per site the developer manages.

CREATE TABLE sites (
	id          TEXT PRIMARY KEY,
	name        TEXT NOT NULL,
	domain      TEXT NOT NULL,
	-- Scopes ingestion and the npm proxy to this one site. UNIQUE gives us the
	-- index /api/collect needs: every event resolves a site_key on the hot path.
	site_key    TEXT NOT NULL UNIQUE,
	-- Where the monthly report goes. Optional: a site can exist before its
	-- owner is known.
	owner_email TEXT,
	created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
