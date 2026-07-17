package http

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// dashboardAssets serves the built SPA.
//
// Two callers load these files. The console loads them same-origin. The
// domain-native proxy renders a page on the owner's server that names them, so
// the owner's browser fetches them cross-origin -- which is why they carry
// permissive CORS headers. They are public static assets: a JS bundle and a
// stylesheet, identical for every deployment, containing no data and no
// secrets.
func dashboardAssets(dir string) (http.Handler, bool) {
	if dir == "" {
		return nil, false
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil, false
	}

	files := http.FileServer(http.Dir(dir))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The stylesheet pulls font files relative to itself, so the browser
		// fetches those cross-origin too and blocks them without this.
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")

		if isHashedAsset(r.URL.Path) {
			// The hash is the version: a new build is a new URL, so these can
			// be cached forever.
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			// zenith.js and zenith.css keep their names across builds, so the
			// browser has to ask whether they changed. ETag makes that a 304.
			w.Header().Set("Cache-Control", "public, no-cache")
		}

		files.ServeHTTP(w, r)
	}), true
}

// viteHashLength is the length of the content hash Vite appends to an asset
// filename.
const viteHashLength = 8

// isHashedAsset reports whether a filename carries a content hash, and so can
// be cached indefinitely.
//
// Vite emits "geist-latin-wght-normal-BgDaEnEv.woff2": the name, a hyphen, and
// an 8-character base64url hash. The hash itself may contain a hyphen
// ("DjL33-gN"), so this checks the tail directly rather than splitting on "-"
// and hoping the last piece is the whole hash -- which it is not.
func isHashedAsset(path string) bool {
	name := filepath.Base(path)

	// The entry points keep stable names across builds precisely so the proxy
	// can name them, which means their content changes under a fixed URL.
	if name == "zenith.js" || name == "zenith.css" || name == "index.html" {
		return false
	}

	stem := strings.TrimSuffix(name, filepath.Ext(name))

	// Need "<something>-<8 chars>".
	if len(stem) < viteHashLength+2 {
		return false
	}
	if stem[len(stem)-viteHashLength-1] != '-' {
		return false
	}

	for _, c := range stem[len(stem)-viteHashLength:] {
		isBase64URL := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '-'
		if !isBase64URL {
			return false
		}
	}
	return true
}
