// Package id generates the identifiers Zenith stores.
package id

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// New returns a random 128-bit identifier as hex, for primary keys.
func New() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// NewSiteKey returns a site key.
//
// This is a credential, not just an identifier: it is what authorizes an event
// as belonging to a site, and it ships in the owner's project. It is 256 bits
// from crypto/rand, prefixed so a leaked key is recognizable on sight.
func NewSiteKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("id: site key: %w", err)
	}
	return "zk_" + base64.RawURLEncoding.EncodeToString(b), nil
}
