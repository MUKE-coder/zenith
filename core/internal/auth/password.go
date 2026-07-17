// Package auth handles credentials and tokens.
package auth

import (
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

// MinPasswordLength is the shortest password Zenith will hash. Length is the
// property that actually resists an offline attack on a stolen hash, so it is
// the only rule -- composition rules mostly produce "Passw0rd!".
const MinPasswordLength = 6

// ErrWeakPassword is returned when a password is too short to accept.
var ErrWeakPassword = fmt.Errorf("password must be at least %d characters", MinPasswordLength)

// ErrBadCredentials is returned when a password does not match a hash.
//
// It is deliberately vague: callers must not tell an attacker whether the email
// or the password was the part that was wrong.
var ErrBadCredentials = errors.New("incorrect email or password")

// HashPassword returns a bcrypt hash of password.
func HashPassword(password string) (string, error) {
	if utf8.RuneCountInString(password) < MinPasswordLength {
		return "", ErrWeakPassword
	}
	// bcrypt silently truncates past 72 bytes, which would make two different
	// long passwords interchangeable. Reject rather than quietly weaken.
	if len(password) > 72 {
		return "", errors.New("password must be at most 72 bytes")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		// Never wrap: the error could carry the password into a log line.
		return "", errors.New("auth: could not hash password")
	}
	return string(hash), nil
}

// VerifyPassword reports whether password matches hash.
//
// bcrypt's comparison is constant-time, so this does not leak the hash through
// response timing.
func VerifyPassword(hash, password string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return ErrBadCredentials
	}
	return nil
}

// decoyHash is a bcrypt hash of a random value nobody knows, at the same cost
// as a real one. Computed once, on first use.
var decoyHash = sync.OnceValue(func() []byte {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		// Fall back to a fixed value: this hash only has to cost time, and
		// failing login entirely would be worse than a predictable decoy.
		secret = []byte("zenith-decoy-password-placeholder")
	}
	hash, err := bcrypt.GenerateFromPassword(secret, bcrypt.DefaultCost)
	if err != nil {
		return nil
	}
	return hash
})

// VerifyDecoy spends the time a real password check would, and always fails.
//
// Call it on the "no such user" path. Without it, login answers unknown emails
// in microseconds and known ones in ~60ms, and that difference is a working
// account-enumeration oracle: an attacker learns who has an account here
// without ever guessing a password.
func VerifyDecoy() error {
	hash := decoyHash()
	if hash == nil {
		return ErrBadCredentials
	}
	_ = bcrypt.CompareHashAndPassword(hash, []byte("wrong"))
	return ErrBadCredentials
}
