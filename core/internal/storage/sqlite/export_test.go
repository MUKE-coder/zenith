package sqlite

import "database/sql"

// DB exposes the underlying handle so tests can assert on schema constraints
// directly. This file is compiled only under test, so the production API stays
// narrow.
func (s *Store) DB() *sql.DB { return s.db }
