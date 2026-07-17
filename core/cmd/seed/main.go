// Command seed creates the first developer account.
//
// Run it once against a fresh deployment:
//
//	ZENITH_ADMIN_EMAIL=you@example.com ZENITH_ADMIN_PASSWORD=... go run ./cmd/seed
//
// It is safe to re-run: an account that already exists is left untouched rather
// than overwritten, so this cannot silently reset a password.
//
// Core does the same thing on boot when both variables are set; this command
// exists for provisioning a deployment without restarting it.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/zenith/core/internal/account"
	"github.com/zenith/core/internal/config"
	"github.com/zenith/core/internal/storage/sqlite"
)

func main() {
	// Flags exist for convenience, but the password is read from the
	// environment only: a command-line password lands in shell history and in
	// the process list, where any other user on the box can read it.
	email := flag.String("email", "", "admin email (or set ZENITH_ADMIN_EMAIL)")
	flag.Parse()

	if err := run(*email); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
}

func run(emailFlag string) error {
	// Seeding issues no tokens, so it does not require a signing secret.
	cfg, err := config.LoadWithoutSecret()
	if err != nil {
		return err
	}

	email := strings.TrimSpace(firstNonEmpty(emailFlag, cfg.AdminEmail))
	password := cfg.AdminPassword

	if email == "" {
		return errors.New("set -email or ZENITH_ADMIN_EMAIL")
	}
	if password == "" {
		return errors.New("set ZENITH_ADMIN_PASSWORD")
	}

	ctx := context.Background()

	app, err := sqlite.Open(ctx, cfg.AppDBPath)
	if err != nil {
		return err
	}
	defer app.Close()

	if err := app.Migrate(ctx); err != nil {
		return err
	}

	created, err := account.Provision(ctx, app, email, password)
	if err != nil {
		return err
	}

	if !created {
		fmt.Printf("%s already exists. Nothing changed.\n", email)
		return nil
	}
	fmt.Printf("Created developer account %s.\n", email)
	return nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
