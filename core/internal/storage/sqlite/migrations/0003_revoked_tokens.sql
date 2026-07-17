-- JWT revocation.
--
-- Auth is stateless: a valid signature plus an unexpired `exp` is normally
-- enough. This table is the denylist that makes logout mean something before
-- the token would have expired on its own.
--
-- Only revoked tokens live here, so it stays small. A row is dead weight once
-- expires_at passes -- the signature check rejects the token anyway -- so a
-- periodic sweep deletes expired rows.

CREATE TABLE revoked_tokens (
	-- The JWT's `jti` claim.
	jti        TEXT PRIMARY KEY,
	user_id    TEXT REFERENCES users(id) ON DELETE CASCADE,
	-- Mirrors the token's `exp`: when this passes, the row can be swept.
	expires_at TEXT NOT NULL,
	revoked_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE INDEX idx_revoked_tokens_expires_at ON revoked_tokens (expires_at);
