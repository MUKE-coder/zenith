// Package visitor derives the cookieless identity Zenith counts visitors with.
//
// The whole privacy position rests on this file. There is no cookie, no
// fingerprint, and no persistent identifier: a visitor is a keyed hash of
// things we already saw in the request, and the key changes every day.
//
//	visitor_hash = HMAC-SHA256(daily_salt, date | site | ip | user_agent)
//
// The same person on the same site on the same day hashes to the same value,
// so they are counted once. Tomorrow the salt is different, so tomorrow's hash
// is unlinkable to today's -- the data cannot answer "was this the same person
// last week?" because the key that would prove it no longer exists anywhere.
//
// This is the Plausible method: no consent banner, minimal legal surface.
package visitor

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// hashBytes is how much of the HMAC we keep. 16 bytes is 2^128 values: far
// more than enough to avoid collisions among a day's visitors to one site, and
// half the storage of the full digest.
const hashBytes = 16

// saltBytes is the size of the daily salt.
const saltBytes = 32

// Hasher computes visitor hashes under a salt that rotates daily.
//
// The salt lives in memory and is never written down -- not to disk, not to a
// log, not to either database. That is deliberate: a stored salt could be used
// later to re-derive who a hash belonged to, which is exactly the linkability
// the design exists to prevent. The cost is that restarting the process starts
// a new salt, so a visitor already counted today may be counted once more. We
// take that trade: over-counting a few visitors is a rounding error, while a
// recoverable salt would be a permanent privacy hole.
type Hasher struct {
	mu       sync.Mutex
	salt     []byte
	saltDay  string
	rotation int

	// now is overridable so tests can cross a date boundary without waiting.
	now func() time.Time
}

// NewHasher returns a Hasher with a fresh salt.
func NewHasher() (*Hasher, error) {
	h := &Hasher{now: time.Now}
	if err := h.rotate(h.today()); err != nil {
		return nil, err
	}
	return h, nil
}

// Hash returns the visitor hash for one request.
//
// ip and userAgent are read but never returned, stored, or logged: they exist
// only as input to the HMAC.
func (h *Hasher) Hash(siteID, ip, userAgent string) (string, error) {
	day := h.today()

	salt, err := h.saltFor(day)
	if err != nil {
		return "", err
	}

	mac := hmac.New(sha256.New, salt)

	// Length-prefix every field. Joining with a plain separator would let
	// ("a", "b|c") and ("a|b", "c") produce the same digest, quietly merging
	// two visitors into one.
	for _, field := range []string{day, siteID, ip, userAgent} {
		fmt.Fprintf(mac, "%d:", len(field))
		mac.Write([]byte(field))
	}

	return hex.EncodeToString(mac.Sum(nil)[:hashBytes]), nil
}

// saltFor returns the salt for day, rotating first if the day has turned.
func (h *Hasher) saltFor(day string) ([]byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.saltDay != day {
		if err := h.rotateLocked(day); err != nil {
			return nil, err
		}
	}

	// Copy: the caller must not be able to retain a reference to the live salt
	// that rotation will overwrite.
	salt := make([]byte, len(h.salt))
	copy(salt, h.salt)
	return salt, nil
}

// Rotations reports how many times the salt has been generated. Used by tests
// and by nothing else.
func (h *Hasher) Rotations() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.rotation
}

func (h *Hasher) rotate(day string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.rotateLocked(day)
}

func (h *Hasher) rotateLocked(day string) error {
	salt := make([]byte, saltBytes)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("visitor: generate salt: %w", err)
	}

	// The previous salt is dropped here and nothing else holds it, so
	// yesterday's hashes become permanently un-derivable.
	h.salt = salt
	h.saltDay = day
	h.rotation++
	return nil
}

// today is the salt's rotation boundary, in UTC so it does not depend on where
// the server happens to be.
func (h *Hasher) today() string {
	return h.now().UTC().Format("2006-01-02")
}
