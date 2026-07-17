package visitor_test

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenith/core/internal/visitor"
)

const (
	site = "site-1"
	ip   = "203.0.113.7"
	ua   = "Mozilla/5.0 (X11; Linux x86_64) Firefox/128.0"
)

func newHasher(t *testing.T) *visitor.Hasher {
	t.Helper()
	h, err := visitor.NewHasher()
	if err != nil {
		t.Fatalf("new hasher: %v", err)
	}
	return h
}

// The same visitor, same site, same day must hash the same -- otherwise every
// pageview looks like a new person and "unique visitors" means nothing.
func TestSameVisitorSameDayIsStable(t *testing.T) {
	h := newHasher(t)

	first, err := h.Hash(site, ip, ua)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	second, err := h.Hash(site, ip, ua)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}

	if first != second {
		t.Errorf("same visitor hashed differently: %q then %q", first, second)
	}
}

// Different visitors must not collide into one.
func TestDifferentInputsDiffer(t *testing.T) {
	h := newHasher(t)

	base, err := h.Hash(site, ip, ua)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}

	cases := map[string]struct{ site, ip, ua string }{
		"different site":       {"site-2", ip, ua},
		"different ip":         {site, "198.51.100.9", ua},
		"different user agent": {site, ip, "Mozilla/5.0 Chrome/120.0"},
	}

	for name, c := range cases {
		other, err := h.Hash(c.site, c.ip, c.ua)
		if err != nil {
			t.Fatalf("%s: hash: %v", name, err)
		}
		if other == base {
			t.Errorf("%s: hashed the same as the base visitor", name)
		}
	}
}

// The same person on two sites must not be linkable across them. site_id is in
// the hash precisely so one operator's data cannot be joined to another's.
func TestSameVisitorOnDifferentSitesIsUnlinkable(t *testing.T) {
	h := newHasher(t)

	a, err := h.Hash("site-a", ip, ua)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	b, err := h.Hash("site-b", ip, ua)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}

	if a == b {
		t.Error("the same visitor produced one hash across two sites: cross-site tracking")
	}
}

// Field boundaries must be unambiguous. Without length-prefixing, ("a","bc")
// and ("ab","c") could hash identically and merge two visitors into one.
func TestFieldsCannotBeConfused(t *testing.T) {
	h := newHasher(t)

	first, err := h.Hash("site", "1.2.3.4", "ua")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	// Same concatenation, different field split.
	second, err := h.Hash("site1", ".2.3.4", "ua")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}

	if first == second {
		t.Error("two different field splits produced the same hash: fields are ambiguous")
	}
}

// The day is in the hash, so crossing midnight must change it -- that is what
// makes yesterday's visitor unlinkable to today's.
func TestHashChangesWhenTheDayTurns(t *testing.T) {
	h := newHasher(t)

	day1 := time.Date(2026, 7, 17, 23, 59, 0, 0, time.UTC)
	day2 := time.Date(2026, 7, 18, 0, 1, 0, 0, time.UTC)

	visitor.SetNow(h, func() time.Time { return day1 })
	before, err := h.Hash(site, ip, ua)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}

	visitor.SetNow(h, func() time.Time { return day2 })
	after, err := h.Hash(site, ip, ua)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}

	if before == after {
		t.Error("the same visitor hashed identically across a date boundary: " +
			"yesterday's identity is still linkable today")
	}
}

// Crossing midnight must rotate the salt exactly once, not on every call.
func TestSaltRotatesOncePerDay(t *testing.T) {
	h := newHasher(t)

	day := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	visitor.SetNow(h, func() time.Time { return day })

	start := h.Rotations()
	for range 5 {
		if _, err := h.Hash(site, ip, ua); err != nil {
			t.Fatalf("hash: %v", err)
		}
	}
	if h.Rotations() != start {
		t.Errorf("salt rotated %d times within one day, want 0", h.Rotations()-start)
	}

	next := day.Add(24 * time.Hour)
	visitor.SetNow(h, func() time.Time { return next })
	if _, err := h.Hash(site, ip, ua); err != nil {
		t.Fatalf("hash: %v", err)
	}
	if h.Rotations() != start+1 {
		t.Errorf("salt rotated %d times across the boundary, want 1", h.Rotations()-start)
	}
}

// Two processes must not derive the same identity for a visitor: the salt is
// per-process and random, never a fixed constant.
func TestSaltsAreNotSharedAcrossHashers(t *testing.T) {
	first := newHasher(t)
	second := newHasher(t)

	a, err := first.Hash(site, ip, ua)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	b, err := second.Hash(site, ip, ua)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}

	if a == b {
		t.Error("two hashers produced the same hash: the salt is not random")
	}
}

// The hash must not contain, encode, or reveal its inputs.
func TestHashRevealsNoInput(t *testing.T) {
	h := newHasher(t)

	hash, err := h.Hash(site, ip, ua)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}

	for _, secret := range []string{ip, ua, site} {
		if strings.Contains(hash, secret) {
			t.Errorf("hash %q contains its input %q", hash, secret)
		}
	}
}

func TestHashShape(t *testing.T) {
	h := newHasher(t)

	hash, err := h.Hash(site, ip, ua)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}

	// 16 bytes, hex encoded.
	if len(hash) != 32 {
		t.Errorf("hash %q is %d chars, want 32", hash, len(hash))
	}
	if strings.ContainsFunc(hash, func(r rune) bool {
		return !strings.ContainsRune("0123456789abcdef", r)
	}) {
		t.Errorf("hash %q is not hex", hash)
	}
}

// Ingestion is concurrent; rotation must not tear or race.
func TestHashIsConcurrencySafe(t *testing.T) {
	h := newHasher(t)

	day := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	visitor.SetNow(h, func() time.Time { return day })

	want, err := h.Hash(site, ip, ua)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}

	var wg sync.WaitGroup
	results := make(chan string, 50)

	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := h.Hash(site, ip, ua)
			if err != nil {
				t.Errorf("hash: %v", err)
				return
			}
			results <- got
		}()
	}

	wg.Wait()
	close(results)

	for got := range results {
		if got != want {
			t.Fatalf("concurrent hash = %q, want %q", got, want)
		}
	}
}

// An empty user agent or address must still produce a usable hash rather than
// an error that drops the event.
func TestHashHandlesEmptyFields(t *testing.T) {
	h := newHasher(t)

	hash, err := h.Hash(site, "", "")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if hash == "" {
		t.Error("empty inputs produced an empty hash")
	}
}
