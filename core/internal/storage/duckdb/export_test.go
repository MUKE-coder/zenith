package duckdb

import "database/sql"

// DB exposes the underlying handle so tests can query events directly. This
// file is compiled only under test, so the production API stays narrow: the
// EventStore interface is the only way in.
func (s *Store) DB() *sql.DB { return s.db }
