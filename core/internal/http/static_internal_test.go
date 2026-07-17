package http

import "testing"

// A hashed name is a new URL on every build, so it can be cached forever. A
// stable one changes content under a fixed URL and must be revalidated.
func TestIsHashedAsset(t *testing.T) {
	cases := map[string]bool{
		// Vite's real output. The hash in the second case contains a hyphen,
		// which is what broke the first version of this: splitting on "-" and
		// taking the last piece saw "gN", called it too short, and shipped the
		// font with no-cache.
		"/assets/geist-latin-wght-normal-BgDaEnEv.woff2":        true,
		"/assets/geist-cyrillic-ext-wght-normal-DjL33-gN.woff2": true,
		"/assets/zenith-vendor-a1B2c3D4.js":                     true,

		// Stable entry points: same URL, new content each build.
		"/assets/zenith.js":  false,
		"/assets/zenith.css": false,
		"/index.html":        false,

		// Not hash-shaped.
		"/assets/logo.svg":        false,
		"/assets/some-file.woff2": false,
		"/assets/a-bc.js":         false,
	}

	for path, want := range cases {
		if got := isHashedAsset(path); got != want {
			t.Errorf("isHashedAsset(%q) = %v, want %v", path, got, want)
		}
	}
}
