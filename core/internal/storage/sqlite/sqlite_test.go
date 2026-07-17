package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/zenith/core/internal/storage"
	"github.com/zenith/core/internal/storage/sqlite"
)

func open(t *testing.T) *sqlite.Store {
	t.Helper()

	s, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// Migrate runs on every boot, so re-running it must be a no-op rather than an
// error or a duplicate.
func TestMigrateIsIdempotent(t *testing.T) {
	ctx := context.Background()
	s := open(t)

	for i := range 3 {
		if err := s.Migrate(ctx); err != nil {
			t.Fatalf("migrate run %d: %v", i+1, err)
		}
	}

	// The singleton settings row must not have been inserted three times.
	var n int
	if err := s.DB().QueryRowContext(ctx, `SELECT count(*) FROM settings`).Scan(&n); err != nil {
		t.Fatalf("count settings: %v", err)
	}
	if n != 1 {
		t.Errorf("settings rows = %d, want exactly 1 after repeated migration", n)
	}
}

// Re-opening an existing database must not re-apply migrations that already ran.
func TestMigrateAcrossReopen(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "reopen.sqlite")

	for i := range 2 {
		s, err := sqlite.Open(ctx, path)
		if err != nil {
			t.Fatalf("open %d: %v", i+1, err)
		}
		if err := s.Migrate(ctx); err != nil {
			t.Fatalf("migrate %d: %v", i+1, err)
		}
		s.Close()
	}

	s, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s.Close()

	var applied int
	if err := s.DB().QueryRowContext(ctx,
		`SELECT count(*) FROM schema_migrations`).Scan(&applied); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if applied == 0 {
		t.Fatal("no migrations recorded")
	}

	var settings int
	if err := s.DB().QueryRowContext(ctx, `SELECT count(*) FROM settings`).Scan(&settings); err != nil {
		t.Fatalf("count settings: %v", err)
	}
	if settings != 1 {
		t.Errorf("settings rows = %d, want 1 across reopen", settings)
	}
}

// Every table the product needs must exist after migration.
func TestMigrateCreatesEveryTable(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	want := []string{
		"users", "sites", "revoked_tokens", "settings",
		"audit_jobs", "audit_results", "report_history",
	}

	for _, table := range want {
		var name string
		err := s.DB().QueryRowContext(ctx,
			`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`,
			table).Scan(&name)
		if err != nil {
			t.Errorf("table %q missing: %v", table, err)
		}
	}
}

func TestCreateAndReadUser(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	want := storage.User{
		ID:           "u1",
		Email:        "dev@example.com",
		PasswordHash: "$2a$10$notarealhashbutlongenough",
		Role:         storage.RoleDeveloper,
	}
	if err := s.CreateUser(ctx, want); err != nil {
		t.Fatalf("create user: %v", err)
	}

	got, err := s.UserByEmail(ctx, "dev@example.com")
	if err != nil {
		t.Fatalf("user by email: %v", err)
	}
	if got.ID != want.ID || got.Email != want.Email || got.Role != want.Role {
		t.Errorf("got %+v, want id/email/role from %+v", got, want)
	}
	if got.CreatedAt.IsZero() {
		t.Error("created_at was not populated")
	}
}

// Login lookups must not depend on how the user typed their email.
func TestUserByEmailIsCaseInsensitive(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if err := s.CreateUser(ctx, storage.User{
		ID: "u1", Email: "Dev@Example.com",
		PasswordHash: "hash", Role: storage.RoleDeveloper,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	if _, err := s.UserByEmail(ctx, "dev@example.com"); err != nil {
		t.Errorf("lookup with different case: %v", err)
	}
}

// Two accounts differing only in case must not both exist.
func TestDuplicateEmailConflicts(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	first := storage.User{
		ID: "u1", Email: "dev@example.com",
		PasswordHash: "hash", Role: storage.RoleDeveloper,
	}
	if err := s.CreateUser(ctx, first); err != nil {
		t.Fatalf("create first: %v", err)
	}

	second := storage.User{
		ID: "u2", Email: "DEV@EXAMPLE.COM",
		PasswordHash: "hash", Role: storage.RoleDeveloper,
	}
	if err := s.CreateUser(ctx, second); !errors.Is(err, storage.ErrConflict) {
		t.Errorf("got %v, want ErrConflict", err)
	}
}

func TestUserByEmailNotFound(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if _, err := s.UserByEmail(ctx, "nobody@example.com"); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

// An unknown role is rejected by CreateUser...
func TestCreateUserRejectsUnknownRole(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	err := s.CreateUser(ctx, storage.User{
		ID: "u1", Email: "x@example.com",
		PasswordHash: "hash", Role: storage.Role("admin"),
	})
	if err == nil {
		t.Error("created a user with role \"admin\", want rejection")
	}
}

// ...and independently by the schema, which is what still holds if some future
// caller reaches the table without going through CreateUser.
func TestRoleCheckConstraintIsEnforced(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if _, err := s.DB().ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash, role) VALUES ('u1', 'x@example.com', 'h', 'admin')`,
	); err == nil {
		t.Error("schema accepted role \"admin\", want the CHECK to reject it")
	}
}

// An empty password hash means an account nobody can authenticate as, but that
// a future bug could treat as valid. Refuse it at the door.
func TestCreateUserRejectsEmptyHash(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	err := s.CreateUser(ctx, storage.User{
		ID: "u1", Email: "x@example.com",
		PasswordHash: "", Role: storage.RoleDeveloper,
	})
	if err == nil {
		t.Error("created a user with an empty password hash, want rejection")
	}
}

func TestCountUsers(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	n, err := s.CountUsers(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("fresh database has %d users, want 0", n)
	}

	if err := s.CreateUser(ctx, storage.User{
		ID: "u1", Email: "a@example.com",
		PasswordHash: "hash", Role: storage.RoleDeveloper,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	if n, err = s.CountUsers(ctx); err != nil || n != 1 {
		t.Errorf("count = %d (err %v), want 1", n, err)
	}
}

// The settings singleton is enforced by the schema, not by convention.
func TestSettingsSingletonIsEnforced(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if _, err := s.DB().ExecContext(ctx, `INSERT INTO settings (id) VALUES (2)`); err == nil {
		t.Error("inserted a second settings row, want the CHECK to reject it")
	}
}

// A client must never receive two copies of the same month's report.
func TestReportHistoryRejectsDuplicatePeriod(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if _, err := s.DB().ExecContext(ctx,
		`INSERT INTO sites (id, name, domain, site_key) VALUES ('s1', 'Site', 'a.com', 'zk_1')`,
	); err != nil {
		t.Fatalf("insert site: %v", err)
	}

	const q = `INSERT INTO report_history (id, site_id, period, status) VALUES (?, 's1', '2026-01', 'sent')`
	if _, err := s.DB().ExecContext(ctx, q, "r1"); err != nil {
		t.Fatalf("first report: %v", err)
	}
	if _, err := s.DB().ExecContext(ctx, q, "r2"); err == nil {
		t.Error("recorded the same site/period twice, want the UNIQUE to reject it")
	}
}

// Deleting a site must not strand its audits and reports.
func TestForeignKeysCascade(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if _, err := s.DB().ExecContext(ctx,
		`INSERT INTO sites (id, name, domain, site_key) VALUES ('s1', 'Site', 'a.com', 'zk_1')`,
	); err != nil {
		t.Fatalf("insert site: %v", err)
	}
	if _, err := s.DB().ExecContext(ctx,
		`INSERT INTO audit_jobs (id, site_id, status) VALUES ('j1', 's1', 'queued')`,
	); err != nil {
		t.Fatalf("insert job: %v", err)
	}
	if _, err := s.DB().ExecContext(ctx,
		`INSERT INTO audit_results (id, job_id, page_url, checks, score)
		 VALUES ('r1', 'j1', 'https://a.com/', '{"title":"ok"}', 90)`,
	); err != nil {
		t.Fatalf("insert result: %v", err)
	}

	if _, err := s.DB().ExecContext(ctx, `DELETE FROM sites WHERE id = 's1'`); err != nil {
		t.Fatalf("delete site: %v", err)
	}

	var jobs, results int
	if err := s.DB().QueryRowContext(ctx, `SELECT count(*) FROM audit_jobs`).Scan(&jobs); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if err := s.DB().QueryRowContext(ctx, `SELECT count(*) FROM audit_results`).Scan(&results); err != nil {
		t.Fatalf("count results: %v", err)
	}
	if jobs != 0 || results != 0 {
		t.Errorf("after deleting the site: %d jobs, %d results; want 0 and 0", jobs, results)
	}
}

// checks is JSON that the console renders. Garbage must not reach it.
func TestAuditResultsRejectsInvalidJSON(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if _, err := s.DB().ExecContext(ctx,
		`INSERT INTO sites (id, name, domain, site_key) VALUES ('s1', 'Site', 'a.com', 'zk_1')`,
	); err != nil {
		t.Fatalf("insert site: %v", err)
	}
	if _, err := s.DB().ExecContext(ctx,
		`INSERT INTO audit_jobs (id, site_id, status) VALUES ('j1', 's1', 'queued')`,
	); err != nil {
		t.Fatalf("insert job: %v", err)
	}

	if _, err := s.DB().ExecContext(ctx,
		`INSERT INTO audit_results (id, job_id, page_url, checks) VALUES ('r1', 'j1', '/', 'not json')`,
	); err == nil {
		t.Error("stored invalid JSON in checks, want the CHECK to reject it")
	}
}
