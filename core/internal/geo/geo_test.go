package geo_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zenith/core/internal/geo"
)

// TestResolvesRealDatabase exercises the MaxMind path against an actual
// database. It skips unless one is supplied, because the database cannot be
// redistributed with the source:
//
//	curl -L -o /tmp/country.mmdb \
//	  https://github.com/maxmind/MaxMind-DB/raw/main/test-data/GeoIP2-Country-Test.mmdb
//	ZENITH_TEST_GEOIP_DB=/tmp/country.mmdb go test ./internal/geo/
func TestResolvesRealDatabase(t *testing.T) {
	path := os.Getenv("ZENITH_TEST_GEOIP_DB")
	if path == "" {
		t.Skip("set ZENITH_TEST_GEOIP_DB to a MaxMind country database to run this")
	}

	resolver, err := geo.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer resolver.Close()

	// Addresses from MaxMind's published test fixture.
	public := map[string]string{
		"81.2.69.142":   "GB",
		"89.160.20.128": "SE",
		"216.160.83.56": "US",
	}
	for ip, want := range public {
		if got := resolver.Country(ip); got != want {
			t.Errorf("Country(%q) = %q, want %q", ip, got, want)
		}
	}

	// An address with no country must resolve to Unknown rather than error out
	// and drop the event. Loopback and private ranges are the common case in
	// development and behind a misconfigured proxy.
	for _, ip := range []string{"127.0.0.1", "10.0.0.5", "192.168.1.1", "::1", "garbage", ""} {
		if got := resolver.Country(ip); got != geo.Unknown {
			t.Errorf("Country(%q) = %q, want Unknown", ip, got)
		}
	}
}

// Geo is opt-in. A deployment with no database must still ingest events, just
// without country data.
func TestOpenWithoutPathIsUnavailable(t *testing.T) {
	resolver, err := geo.Open("")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer resolver.Close()

	if got := resolver.Country("8.8.8.8"); got != geo.Unknown {
		t.Errorf("country = %q, want Unknown with no database", got)
	}
}

// A configured but missing database is an operator mistake, and silently
// resolving nothing would hide it until someone noticed an empty geo panel.
func TestOpenWithMissingFileFails(t *testing.T) {
	_, err := geo.Open(filepath.Join(t.TempDir(), "does-not-exist.mmdb"))
	if err == nil {
		t.Fatal("opened a database that does not exist, want an error")
	}
}

func TestUnavailableResolvesNothing(t *testing.T) {
	var resolver geo.Resolver = geo.Unavailable{}

	for _, ip := range []string{"8.8.8.8", "2001:4860:4860::8888", "", "garbage"} {
		if got := resolver.Country(ip); got != geo.Unknown {
			t.Errorf("Country(%q) = %q, want Unknown", ip, got)
		}
	}
	if err := resolver.Close(); err != nil {
		t.Errorf("close: %v", err)
	}
}
