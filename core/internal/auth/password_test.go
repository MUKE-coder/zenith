package auth_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/zenith/core/internal/auth"
)

const goodPassword = "correct horse battery staple"

func TestHashAndVerify(t *testing.T) {
	hash, err := auth.HashPassword(goodPassword)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := auth.VerifyPassword(hash, goodPassword); err != nil {
		t.Errorf("verify correct password: %v", err)
	}
}

// The hash must not contain the password, in any form.
func TestHashDoesNotContainPassword(t *testing.T) {
	hash, err := auth.HashPassword(goodPassword)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if strings.Contains(hash, goodPassword) {
		t.Fatal("hash contains the plaintext password")
	}
	if hash == goodPassword {
		t.Fatal("hash is the plaintext password")
	}
}

// Same password, different salt: two hashes must never match, or a stolen
// database would reveal which accounts share a password.
func TestHashIsSalted(t *testing.T) {
	first, err := auth.HashPassword(goodPassword)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	second, err := auth.HashPassword(goodPassword)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if first == second {
		t.Error("hashing the same password twice produced identical hashes")
	}
	// Both must still verify.
	if err := auth.VerifyPassword(second, goodPassword); err != nil {
		t.Errorf("verify second hash: %v", err)
	}
}

func TestVerifyRejectsWrongPassword(t *testing.T) {
	hash, err := auth.HashPassword(goodPassword)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := auth.VerifyPassword(hash, "wrong password entirely"); !errors.Is(err, auth.ErrBadCredentials) {
		t.Errorf("got %v, want ErrBadCredentials", err)
	}
}

func TestVerifyRejectsGarbageHash(t *testing.T) {
	if err := auth.VerifyPassword("not-a-bcrypt-hash", goodPassword); err == nil {
		t.Error("verified against a malformed hash, want rejection")
	}
}

func TestHashRejectsShortPassword(t *testing.T) {
	if _, err := auth.HashPassword("short"); !errors.Is(err, auth.ErrWeakPassword) {
		t.Errorf("got %v, want ErrWeakPassword", err)
	}
}

func TestHashRejectsEmptyPassword(t *testing.T) {
	if _, err := auth.HashPassword(""); err == nil {
		t.Error("hashed an empty password, want rejection")
	}
}

// bcrypt ignores everything past 72 bytes. Accepting a longer password would
// mean two different passwords silently unlocking the same account.
func TestHashRejectsOverlongPassword(t *testing.T) {
	long := strings.Repeat("a", 73)
	if _, err := auth.HashPassword(long); err == nil {
		t.Error("hashed a 73-byte password, want rejection")
	}
}

// Length is counted in runes, so a short-but-multibyte password is still short.
func TestHashCountsRunesNotBytes(t *testing.T) {
	// 8 runes, 24 bytes: under the rune minimum, over it in bytes.
	if _, err := auth.HashPassword("日本語日本語日本"); !errors.Is(err, auth.ErrWeakPassword) {
		t.Errorf("got %v, want ErrWeakPassword for an 8-rune password", err)
	}
}
