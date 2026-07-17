-- Accounts. A developer sees every site; an owner sees exactly one.

CREATE TABLE users (
	id            TEXT PRIMARY KEY,
	-- NOCASE: nobody should be able to register Alice@ alongside alice@.
	email         TEXT NOT NULL UNIQUE COLLATE NOCASE,
	-- bcrypt/argon2 output. A plaintext password never reaches this column.
	password_hash TEXT NOT NULL,
	role          TEXT NOT NULL CHECK (role IN ('developer', 'owner')),
	created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
