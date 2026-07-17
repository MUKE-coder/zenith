package audit

import (
	"strconv"
	"strings"
	"testing"
)

// The extraction script is a Go string that only ever runs inside a browser,
// so nothing type-checks it: a typo here is a runtime SyntaxError that fails
// every page of every audit at once. It has happened -- a `${...}` written
// into a Go raw string, which does not interpolate, reached Chrome verbatim.
func TestExtractScriptHasNoUnsubstitutedPlaceholders(t *testing.T) {
	if strings.Contains(extractScript, "${") {
		t.Error("the script contains ${...}: Go raw strings do not interpolate, " +
			"so this reaches the browser verbatim and fails to parse")
	}
	if strings.Contains(extractScript, "VITALS_COLLECT_MS") {
		t.Error("VITALS_COLLECT_MS was not substituted")
	}
	if !strings.Contains(extractScript, strconv.Itoa(vitalsCollectMs)) {
		t.Errorf("the collect delay (%d) is not in the script", vitalsCollectMs)
	}
}

// Balanced delimiters are a cheap proxy for "parses at all", and would have
// caught the placeholder bug.
func TestExtractScriptIsBalanced(t *testing.T) {
	pairs := map[rune]rune{')': '(', ']': '[', '}': '{'}
	counts := map[rune]int{}

	for _, c := range extractScript {
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
}

// LCP and CLS are not in the performance timeline. Reading them with
// getEntriesByType returns nothing and reports 0 for every page -- which looks
// exactly like a very fast site, and is why this went unnoticed once already.
func TestExtractScriptObservesVitals(t *testing.T) {
	for _, required := range []string{
		"PerformanceObserver",
		"buffered: true",
		"largest-contentful-paint",
		"layout-shift",
	} {
		if !strings.Contains(extractScript, required) {
			t.Errorf("the script does not use %q", required)
		}
	}

	if strings.Contains(extractScript, "getEntriesByType('largest-contentful-paint')") {
		t.Error("LCP is read with getEntriesByType, which always returns nothing")
	}
	if strings.Contains(extractScript, "getEntriesByType('layout-shift')") {
		t.Error("CLS is read with getEntriesByType, which always returns nothing")
	}
}

// The script returns a Promise, so the caller has to await it.
func TestExtractScriptIsAsync(t *testing.T) {
	if !strings.HasPrefix(extractScript, "(async") {
		t.Error("the script is not async, but the vitals collection needs to be")
	}
}
