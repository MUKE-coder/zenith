package id_test

import (
	"strings"
	"testing"

	"github.com/zenith/core/internal/id"
)

// Collisions would silently merge two sites' data. Not an exhaustive proof of
// randomness -- a guard against a generator that returns a constant.
func TestNewIsUnique(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for range 1000 {
		v, err := id.New()
		if err != nil {
			t.Fatalf("new: %v", err)
		}
		if seen[v] {
			t.Fatalf("duplicate id %q", v)
		}
		seen[v] = true
	}
}

func TestNewSiteKeyIsUnique(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for range 1000 {
		v, err := id.NewSiteKey()
		if err != nil {
			t.Fatalf("new site key: %v", err)
		}
		if seen[v] {
			t.Fatalf("duplicate site key %q", v)
		}
		seen[v] = true
	}
}

// A site key authorizes ingestion, so it must be long enough not to be guessed
// and recognizable enough to be spotted in a leaked log.
func TestSiteKeyShape(t *testing.T) {
	key, err := id.NewSiteKey()
	if err != nil {
		t.Fatalf("new site key: %v", err)
	}

	if !strings.HasPrefix(key, "zk_") {
		t.Errorf("site key %q lacks the zk_ prefix", key)
	}
	// 32 random bytes -> 43 base64url chars, plus the prefix.
	if len(key) < 40 {
		t.Errorf("site key %q is %d chars: too short to resist guessing", key, len(key))
	}
	// Must survive a URL and a config file without escaping.
	if strings.ContainsAny(key, "+/=") {
		t.Errorf("site key %q contains characters that need URL escaping", key)
	}
}
