package http_test

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestTrackerScriptIsServed(t *testing.T) {
	h := newHarness(t)

	resp := h.get(t, "/track.js", "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("content-type = %q, want javascript", ct)
	}
	// Loaded by a script tag on a client's own domain.
	if origin := resp.Header.Get("Access-Control-Allow-Origin"); origin != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want *", origin)
	}
}

// The snippet only ever runs in a browser, so nothing type-checks it. Balanced
// delimiters are a cheap proxy for "parses", and would catch a botched edit.
func TestTrackerScriptIsBalanced(t *testing.T) {
	h := newHarness(t)

	resp := h.get(t, "/track.js", "")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	script := string(body)

	pairs := map[rune]rune{')': '(', ']': '[', '}': '{'}
	counts := map[rune]int{}
	for _, c := range script {
		switch c {
		case '(', '[', '{':
			counts[c]++
		case ')', ']', '}':
			counts[pairs[c]]--
		}
	}
	for open, count := range counts {
		if count != 0 {
			t.Errorf("unbalanced %q: %d unclosed", open, count)
		}
	}

	// The two things the snippet cannot work without.
	for _, required := range []string{"data-site-key", "data-endpoint", "sendBeacon"} {
		if !strings.Contains(script, required) {
			t.Errorf("the snippet does not reference %q", required)
		}
	}
}

// The snippet posts text/plain, not application/json — the same CORS-safelist
// trick the npm tracker uses to avoid a preflight that sendBeacon can't
// survive. If this regresses, every drop-in install silently stops tracking.
func TestTrackerScriptPostsTextPlain(t *testing.T) {
	h := newHarness(t)

	resp := h.get(t, "/track.js", "")
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "text/plain") {
		t.Error("the snippet does not post text/plain: a preflight will block sendBeacon")
	}
}
