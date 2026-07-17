package audit_test

import (
	"strings"
	"testing"

	"github.com/zenith/audit-worker/internal/audit"
)

// find returns the check with an id, or fails.
func find(t *testing.T, checks []audit.Check, id string) audit.Check {
	t.Helper()

	for _, check := range checks {
		if check.ID == id {
			return check
		}
	}
	t.Fatalf("no check %q in %v", id, ids(checks))
	return audit.Check{}
}

func has(checks []audit.Check, id string) bool {
	for _, check := range checks {
		if check.ID == id {
			return true
		}
	}
	return false
}

func ids(checks []audit.Check) []string {
	out := make([]string, 0, len(checks))
	for _, check := range checks {
		out = append(out, check.ID)
	}
	return out
}

// good is a page with nothing wrong with it.
func good() audit.Page {
	return audit.Page{
		URL:         "https://example.com/",
		Title:       "Acme Storefront — Quality tools since 1994",
		Description: "Acme Storefront sells quality hand tools, delivered next day across the country. Browse our range today.",
		Canonical:   "https://example.com/",
		H1s:         []string{"Acme Storefront"},
		Headings:    []string{"h1", "h2", "h3"},
		ImagesTotal: 3,
		ImagesNoAlt: 0,
		JSONLD:      []string{`{"@context":"https://schema.org","@type":"Organization","name":"Acme"}`},
		Vitals:      audit.Vitals{LCP: 1200, CLS: 0.02, TTFB: 200},
	}
}

func TestAPerfectPageScores100(t *testing.T) {
	checks := audit.Evaluate(good(), nil)
	score := audit.Score(checks.Checks)

	if score != 100 {
		t.Errorf("score = %d, want 100. Findings: %v", score, nonOK(checks.Checks))
	}
}

func nonOK(checks []audit.Check) []string {
	out := []string{}
	for _, check := range checks {
		if check.Severity != audit.SeverityOK {
			out = append(out, check.ID+": "+check.Message)
		}
	}
	return out
}

func TestMissingTitle(t *testing.T) {
	page := good()
	page.Title = ""

	check := find(t, audit.Evaluate(page, nil).Checks, "title.missing")
	if check.Severity != audit.SeverityError {
		t.Errorf("severity = %q, want error", check.Severity)
	}
}

func TestTitleLength(t *testing.T) {
	cases := map[string]string{
		"Short": "title.short",
		strings.Repeat("a really long title ", 5): "title.long",
	}

	for title, wantID := range cases {
		page := good()
		page.Title = title

		if !has(audit.Evaluate(page, nil).Checks, wantID) {
			t.Errorf("title %q did not produce %q", title, wantID)
		}
	}
}

func TestMissingDescription(t *testing.T) {
	page := good()
	page.Description = ""

	check := find(t, audit.Evaluate(page, nil).Checks, "description.missing")
	if check.Severity != audit.SeverityError {
		t.Errorf("severity = %q, want error", check.Severity)
	}
}

func TestH1Rules(t *testing.T) {
	none := good()
	none.H1s = nil
	if find(t, audit.Evaluate(none, nil).Checks, "h1.missing").Severity != audit.SeverityError {
		t.Error("a page with no H1 is not an error")
	}

	many := good()
	many.H1s = []string{"One", "Two"}
	if !has(audit.Evaluate(many, nil).Checks, "h1.multiple") {
		t.Error("two H1s were not flagged")
	}
}

// A skipped level breaks the outline a screen reader navigates by.
func TestSkippedHeadingLevels(t *testing.T) {
	page := good()
	page.Headings = []string{"h1", "h4"}

	if !has(audit.Evaluate(page, nil).Checks, "headings.skipped") {
		t.Error("an h1 -> h4 jump was not flagged")
	}
}

func TestHeadingOrderAllowsGoingBackUp(t *testing.T) {
	// h1, h2, h3, h2 is a normal document: two sections, the second starting
	// over. Only jumps *down* past a level are a defect.
	page := good()
	page.Headings = []string{"h1", "h2", "h3", "h2", "h3"}

	if has(audit.Evaluate(page, nil).Checks, "headings.skipped") {
		t.Error("a heading order that returns to a shallower level was flagged")
	}
}

func TestMissingAltText(t *testing.T) {
	page := good()
	page.ImagesTotal = 4
	page.ImagesNoAlt = 2

	check := find(t, audit.Evaluate(page, nil).Checks, "images.alt_missing")
	if check.Severity != audit.SeverityError {
		t.Errorf("severity = %q, want error", check.Severity)
	}
	if !strings.Contains(check.Message, "2 of 4") {
		t.Errorf("message %q does not say how many", check.Message)
	}
}

// The check that is nearly always a real incident when it fires.
func TestNoindexIsAnError(t *testing.T) {
	page := good()
	page.Robots = "noindex, nofollow"

	check := find(t, audit.Evaluate(page, nil).Checks, "robots.noindex")
	if check.Severity != audit.SeverityError {
		t.Errorf("severity = %q, want error: a stray noindex loses all the traffic", check.Severity)
	}
}

func TestNoindexIsCaseInsensitive(t *testing.T) {
	page := good()
	page.Robots = "NoIndex"

	if !has(audit.Evaluate(page, nil).Checks, "robots.noindex") {
		t.Error("NoIndex was not recognized")
	}
}

func TestBrokenLinks(t *testing.T) {
	broken := []audit.BrokenLink{
		{URL: "https://example.com/gone", Reason: "returned 404"},
		{URL: "https://elsewhere.test/", Reason: "could not be reached"},
	}

	check := find(t, audit.Evaluate(good(), broken).Checks, "links.broken")
	if check.Severity != audit.SeverityError {
		t.Errorf("severity = %q, want error", check.Severity)
	}
	// The URLs have to be in there: "2 broken links" without saying which is
	// a fact the developer cannot act on.
	if !strings.Contains(check.Detail, "example.com/gone") {
		t.Errorf("detail %q does not name the broken links", check.Detail)
	}
}

func TestVitalsThresholds(t *testing.T) {
	cases := []struct {
		name   string
		vitals audit.Vitals
		wantID string
	}{
		{"fast", audit.Vitals{LCP: 1000}, "vitals.lcp_ok"},
		{"slow", audit.Vitals{LCP: 3000}, "vitals.lcp_slow"},
		{"poor", audit.Vitals{LCP: 5000}, "vitals.lcp_poor"},
		{"shifty", audit.Vitals{LCP: 1000, CLS: 0.3}, "vitals.cls_poor"},
		{"slow server", audit.Vitals{LCP: 1000, TTFB: 1500}, "vitals.ttfb_slow"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			page := good()
			page.Vitals = c.vitals

			if !has(audit.Evaluate(page, nil).Checks, c.wantID) {
				t.Errorf("vitals %+v did not produce %q", c.vitals, c.wantID)
			}
		})
	}
}

// An unmeasurable LCP is not a finding: reporting "0ms, excellent" would be a
// lie, and reporting it as slow would be a different one.
func TestUnmeasurableLCPIsNotAFinding(t *testing.T) {
	page := good()
	page.Vitals = audit.Vitals{LCP: 0}

	for _, check := range audit.Evaluate(page, nil).Checks {
		if strings.HasPrefix(check.ID, "vitals.lcp") && check.Severity != audit.SeverityOK {
			t.Errorf("an unmeasured LCP produced %q", check.ID)
		}
	}
}

func TestScoreWeighting(t *testing.T) {
	// An error costs more than a warning: the ones that lose traffic should
	// dominate the ones that are merely untidy.
	oneError := audit.Score([]audit.Check{{Severity: audit.SeverityError}})
	oneWarning := audit.Score([]audit.Check{{Severity: audit.SeverityWarning}})

	if oneError >= oneWarning {
		t.Errorf("an error (%d) costs no more than a warning (%d)", oneError, oneWarning)
	}
	if audit.Score([]audit.Check{{Severity: audit.SeverityOK}}) != 100 {
		t.Error("a passing check cost points")
	}
}

// A page cannot be worse than nothing.
func TestScoreFloorsAtZero(t *testing.T) {
	checks := make([]audit.Check, 20)
	for i := range checks {
		checks[i] = audit.Check{Severity: audit.SeverityError}
	}

	if score := audit.Score(checks); score != 0 {
		t.Errorf("score = %d, want 0", score)
	}
}

func TestSiteScoreAverages(t *testing.T) {
	if got := audit.SiteScore([]int{100, 50}); got != 75 {
		t.Errorf("SiteScore(100, 50) = %d, want 75", got)
	}
	if got := audit.SiteScore(nil); got != 0 {
		t.Errorf("SiteScore(nil) = %d, want 0", got)
	}
}
