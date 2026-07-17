// Package geo resolves an IP address to a country.
//
// Country is as precise as Zenith ever gets. City-level geo, on top of a
// timestamp and a path, starts to identify individuals rather than describe an
// audience -- and it is not a question any dashboard here asks. The address
// itself is read from the request and never stored.
package geo

import (
	"fmt"
	"net"
	"os"

	"github.com/oschwald/geoip2-golang"
)

// Unknown is the country for an address we cannot place: a private range, a
// malformed value, or a deployment running without a geo database.
const Unknown = ""

// Resolver maps an IP address to an ISO-3166-1 alpha-2 country code.
type Resolver interface {
	// Country returns the country code, or Unknown.
	Country(ip string) string

	// Close releases the database handle.
	Close() error
}

// Unavailable is a Resolver for deployments with no geo database. Every lookup
// is Unknown.
//
// Country data is a nice-to-have, not a reason to fail an event: ingestion
// must keep working whether or not an operator supplied a database.
type Unavailable struct{}

// Country always returns Unknown.
func (Unavailable) Country(string) string { return Unknown }

// Close does nothing.
func (Unavailable) Close() error { return nil }

// mmdb resolves against a MaxMind-format database.
type mmdb struct {
	reader *geoip2.Reader
}

// Open loads a MaxMind country database (GeoLite2-Country.mmdb or DB-IP
// equivalent).
//
// An empty path returns Unavailable, so geo is opt-in rather than a
// prerequisite for running Zenith.
func Open(path string) (Resolver, error) {
	if path == "" {
		return Unavailable{}, nil
	}

	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("geo: cannot read database at %s: %w", path, err)
	}

	reader, err := geoip2.Open(path)
	if err != nil {
		return nil, fmt.Errorf("geo: open %s: %w", path, err)
	}
	return &mmdb{reader: reader}, nil
}

// Country returns the ISO country code for ip.
func (m *mmdb) Country(ip string) string {
	addr := net.ParseIP(ip)
	if addr == nil {
		return Unknown
	}

	// A private or loopback address has no country, and asking is pointless.
	// This is the common case in development and behind a misconfigured proxy.
	if addr.IsLoopback() || addr.IsPrivate() || addr.IsUnspecified() ||
		addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() {
		return Unknown
	}

	record, err := m.reader.Country(addr)
	if err != nil {
		// A lookup miss is not an error worth failing an event over.
		return Unknown
	}
	return record.Country.IsoCode
}

// Close releases the database handle.
func (m *mmdb) Close() error {
	return m.reader.Close()
}
