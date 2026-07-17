// Package account creates and finds Zenith accounts.
package account

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/zenith/core/internal/auth"
	"github.com/zenith/core/internal/id"
	"github.com/zenith/core/internal/storage"
)

// Provision creates a developer account if the email is not already taken. It
// reports whether it created one.
//
// It never overwrites an existing account: the seed command and every boot both
// call this with the same environment variables, so overwriting would mean a
// restart silently resetting the developer's password back to whatever is in
// the environment.
func Provision(ctx context.Context, app storage.AppStore, email, password string) (bool, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return false, errors.New("account: email is required")
	}
	if !strings.Contains(email, "@") {
		return false, fmt.Errorf("account: %q is not an email address", email)
	}
	if password == "" {
		return false, errors.New("account: password is required")
	}

	if _, err := app.UserByEmail(ctx, email); err == nil {
		return false, nil
	} else if !errors.Is(err, storage.ErrNotFound) {
		return false, err
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return false, err
	}

	userID, err := id.New()
	if err != nil {
		return false, err
	}

	err = app.CreateUser(ctx, storage.User{
		ID:           userID,
		Email:        email,
		PasswordHash: hash,
		Role:         storage.RoleDeveloper,
	})
	// Lost a race with a concurrent seed or boot. The account exists, which is
	// all the caller wanted.
	if errors.Is(err, storage.ErrConflict) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
