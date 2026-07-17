-- Split a site's one key into two, because writing events and reading
-- analytics have different threat models.
--
-- site_key (0002) is public: it ships inside the tracking snippet, so anyone
-- can read it out of page source. It authorizes writing events and nothing
-- else -- the worst a leak buys is junk traffic on one site.
--
-- api_key is secret: it lives server-side in the owner's zenith.config.js and
-- never reaches a browser. It authorizes reading that site's analytics.
--
-- Additive rather than a rewrite of 0002: a database created before this
-- migration has 0002 already recorded and would never see the change.

-- Nullable because SQLite cannot ADD COLUMN with NOT NULL and no default.
-- Site creation (phase 6) always sets it.
ALTER TABLE sites ADD COLUMN api_key TEXT;

-- SQLite also refuses ADD COLUMN ... UNIQUE, so the constraint is an index.
-- It permits multiple NULLs, which is what lets the column stay nullable.
CREATE UNIQUE INDEX idx_sites_api_key ON sites (api_key);
