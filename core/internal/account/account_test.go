package account_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/zenith/core/internal/account"
	"github.com/zenith/core/internal/auth"
	"github.com/zenith/core/internal/storage"
	"github.com/zenith/core/internal/storage/sqlite"
)

const password = "correct horse battery staple"

func newStore(t *testing.T) storage.AppStore {
	t.Helper()
	ctx := context.Background()

	s, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "app.sqlite"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestProvisionCreatesDeveloper(t *testing.T) {
	ctx := context.Background()
	app := newStore(t)

	created, err := account.Provision(ctx, app, "dev@example.com", password)
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if !created {
		t.Fatal("provision reported no account created")
	}

	user, err := app.UserByEmail(ctx, "dev@example.com")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if user.Role != storage.RoleDeveloper {
		t.Errorf("role = %q, want developer", user.Role)
	}
	if err := auth.VerifyPassword(user.PasswordHash, password); err != nil {
		t.Errorf("stored password does not verify: %v", err)
	}
}

// Core calls Provision on every boot with the same environment variables. A
// restart must never reset the developer's password to whatever is in the
// environment -- they may well have changed it since.
func TestProvisionNeverOverwritesAnExistingAccount(t *testing.T) {
	ctx := context.Background()
	app := newStore(t)

	if _, err := account.Provision(ctx, app, "dev@example.com", password); err != nil {
		t.Fatalf("first provision: %v", err)
	}

	created, err := account.Provision(ctx, app, "dev@example.com", "a completely different password")
	if err != nil {
		t.Fatalf("second provision: %v", err)
	}
	if created {
		t.Error("provision reported creating an account that already existed")
	}

	user, err := app.UserByEmail(ctx, "dev@example.com")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if err := auth.VerifyPassword(user.PasswordHash, password); err != nil {
		t.Error("the original password no longer works: provision overwrote it")
	}
	if err := auth.VerifyPassword(user.PasswordHash, "a completely different password"); err == nil {
		t.Error("the second password now works: provision overwrote the account")
	}
}

// Matching an existing account must not depend on how the email is cased.
func TestProvisionMatchesExistingAccountCaseInsensitively(t *testing.T) {
	ctx := context.Background()
	app := newStore(t)

	if _, err := account.Provision(ctx, app, "dev@example.com", password); err != nil {
		t.Fatalf("first provision: %v", err)
	}

	created, err := account.Provision(ctx, app, "DEV@EXAMPLE.COM", password)
	if err != nil {
		t.Fatalf("second provision: %v", err)
	}
	if created {
		t.Error("created a second account differing only in case")
	}

	n, err := app.CountUsers(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("users = %d, want 1", n)
	}
}

func TestProvisionRejectsBadInput(t *testing.T) {
	ctx := context.Background()

	cases := []struct{ name, email, password string }{
		{"no email", "", password},
		{"no password", "dev@example.com", ""},
		{"not an email", "notanemail", password},
		{"short password", "dev@example.com", "short"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			app := newStore(t)
			if _, err := account.Provision(ctx, app, c.email, c.password); err == nil {
				t.Errorf("Provision(%q, %q) succeeded, want an error", c.email, c.password)
			}
		})
	}
}
