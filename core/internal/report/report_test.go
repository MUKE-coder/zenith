package report_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zenith/core/internal/report"
	"github.com/zenith/core/internal/storage"
	"github.com/zenith/core/internal/storage/duckdb"
)

func store(t *testing.T, events ...storage.Event) *duckdb.Store {
	t.Helper()

	s, err := duckdb.Open(context.Background(), filepath.Join(t.TempDir(), "events.duckdb"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	if len(events) > 0 {
		if err := s.Insert(context.Background(), events...); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	return s
}

var site = storage.Site{ID: "site-1", Name: "Acme", Domain: "acme.com"}

func view(at time.Time, visitor, path string) storage.Event {
	return storage.Event{
		SiteID: "site-1", TS: at, Type: "pageview", Path: path, VisitorHash: visitor,
	}
}

// A report sent on the 1st covers the month that just finished.
func TestPeriodIsThePreviousMonth(t *testing.T) {
	cases := map[string]string{
		"2026-07-01T09:00:00Z": "2026-06",
		"2026-07-17T12:00:00Z": "2026-06",
		"2026-01-01T00:00:00Z": "2025-12", // across a year boundary
		"2026-03-01T09:00:00Z": "2026-02",
	}

	for nowStr, want := range cases {
		now, err := time.Parse(time.RFC3339, nowStr)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if got := report.Period(now); got != want {
			t.Errorf("Period(%s) = %q, want %q", nowStr, got, want)
		}
	}
}

// The classic date trap: on the 31st, naively subtracting a month lands on
// March 3rd, not February.
func TestPeriodOnTheLastDayOfALongMonth(t *testing.T) {
	now := time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)

	if got := report.Period(now); got != "2026-02" {
		t.Errorf("Period(Mar 31) = %q, want 2026-02", got)
	}
}

// The report must count the reported month and nothing else.
func TestBuildCoversExactlyThePeriod(t *testing.T) {
	s := store(t,
		// May: before the period.
		view(time.Date(2026, 5, 31, 23, 59, 0, 0, time.UTC), "v0", "/old"),
		// June: the reported month.
		view(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), "v1", "/"),
		view(time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC), "v1", "/pricing"),
		view(time.Date(2026, 6, 30, 23, 59, 0, 0, time.UTC), "v2", "/"),
		// July: after it.
		view(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), "v3", "/new"),
	)

	data, err := report.Build(context.Background(), s, site, "2026-06")
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if data.Summary.Pageviews != 3 {
		t.Errorf("pageviews = %d, want 3 (June only)", data.Summary.Pageviews)
	}
	if data.Summary.Visitors != 2 {
		t.Errorf("visitors = %d, want 2", data.Summary.Visitors)
	}
	if data.PeriodLabel != "June 2026" {
		t.Errorf("label = %q, want June 2026", data.PeriodLabel)
	}
}

// The comparison is against the month before the reported one.
func TestBuildComparesToThePriorMonth(t *testing.T) {
	s := store(t,
		// May: 1 pageview.
		view(time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC), "v0", "/"),
		// June: 2.
		view(time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC), "v1", "/"),
		view(time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC), "v2", "/"),
	)

	data, err := report.Build(context.Background(), s, site, "2026-06")
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if data.Previous.Pageviews != 1 {
		t.Errorf("previous pageviews = %d, want 1 (May)", data.Previous.Pageviews)
	}
	if data.Change.Pageviews == nil {
		t.Fatal("no change computed")
	}
	if *data.Change.Pageviews != 100 {
		t.Errorf("change = %v, want 100 (1 -> 2)", *data.Change.Pageviews)
	}
}

// Growth from zero has no percentage. The template says "First month".
func TestBuildFirstMonthHasNoPercentage(t *testing.T) {
	s := store(t, view(time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC), "v1", "/"))

	data, err := report.Build(context.Background(), s, site, "2026-06")
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if data.Change.Pageviews != nil {
		t.Errorf("change = %v, want nil: there is no percentage from zero", *data.Change.Pageviews)
	}

	html, err := report.Render(data)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(html, "First month") {
		t.Error("the email does not say this is the first month")
	}
}

func TestBuildRejectsBadPeriod(t *testing.T) {
	s := store(t)

	for _, period := range []string{"", "June", "2026", "2026-13-01", "nonsense"} {
		if _, err := report.Build(context.Background(), s, site, period); err == nil {
			t.Errorf("accepted period %q", period)
		}
	}
}

// A month with no traffic is a real answer, and the email says so rather than
// showing an empty shell.
func TestRenderEmptyMonth(t *testing.T) {
	s := store(t)

	data, err := report.Build(context.Background(), s, site, "2026-06")
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	html, err := report.Render(data)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(html, "No traffic recorded") {
		t.Error("an empty month does not say so")
	}
}

// Country codes mean nothing to the client this email is for.
func TestBuildResolvesCountryNames(t *testing.T) {
	s := store(t, storage.Event{
		SiteID: "site-1", TS: time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC),
		Type: "pageview", Path: "/", VisitorHash: "v1", Country: "UG",
	})

	data, err := report.Build(context.Background(), s, site, "2026-06")
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if len(data.TopCountries) != 1 {
		t.Fatalf("got %d countries, want 1", len(data.TopCountries))
	}
	if data.TopCountries[0].Label != "Uganda" {
		t.Errorf("country = %q, want Uganda", data.TopCountries[0].Label)
	}
}

// Internal navigation is not a referral: a site's top referrer must not be
// itself.
func TestBuildExcludesSelfReferrals(t *testing.T) {
	at := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	s := store(t,
		storage.Event{SiteID: "site-1", TS: at, Type: "pageview", Path: "/b",
			VisitorHash: "v1", Referrer: "acme.com"},
		storage.Event{SiteID: "site-1", TS: at, Type: "pageview", Path: "/a",
			VisitorHash: "v2", Referrer: "news.ycombinator.com"},
	)

	data, err := report.Build(context.Background(), s, site, "2026-06")
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	for _, r := range data.TopReferrers {
		if r.Label == "acme.com" {
			t.Error("the site's own domain is listed as a referrer")
		}
	}
}

// The subject lands in an inbox next to everything else and has to be
// identifiable unopened.
func TestSubjectNamesTheSiteAndMonth(t *testing.T) {
	subject := report.Subject(report.Data{SiteName: "Acme", PeriodLabel: "June 2026"})

	if !strings.Contains(subject, "Acme") || !strings.Contains(subject, "June 2026") {
		t.Errorf("subject = %q, want it to name the site and the month", subject)
	}
}

// Email clients strip <style> blocks, so every rule has to be on the element.
func TestRenderInlinesItsStyles(t *testing.T) {
	s := store(t, view(time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC), "v1", "/"))

	data, err := report.Build(context.Background(), s, site, "2026-06")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	html, err := report.Render(data)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if strings.Contains(html, "<style") {
		t.Error("the email has a <style> block, which many clients strip")
	}
	if !strings.Contains(html, `style="`) {
		t.Error("the email has no inline styles")
	}
	// Outlook renders with Word: no flex, no grid.
	for _, banned := range []string{"display:flex", "display:grid", "var(--"} {
		if strings.Contains(html, banned) {
			t.Errorf("the email uses %q, which email clients do not render", banned)
		}
	}
}

// A site name is developer-entered, but it is interpolated into HTML that gets
// mailed. Escape it.
func TestRenderEscapesTheSiteName(t *testing.T) {
	data := report.Data{
		SiteName:    `Acme <script>alert(1)</script>`,
		SiteDomain:  "acme.com",
		PeriodLabel: "June 2026",
	}

	html, err := report.Render(data)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(html, "<script>alert(1)</script>") {
		t.Error("the site name was interpolated unescaped")
	}
}

func TestNumbersAreGrouped(t *testing.T) {
	s := store(t)

	data, err := report.Build(context.Background(), s, site, "2026-06")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	data.Summary = storage.Summary{Pageviews: 1234567, Visitors: 1000, Sessions: 999}

	html, err := report.Render(data)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	// Grouped and exact: an email is forwarded, and "1.2M" invites the
	// question the real number answers.
	if !strings.Contains(html, "1,234,567") {
		t.Error("large numbers are not grouped")
	}
	if !strings.Contains(html, "1,000") {
		t.Error("thousands are not grouped")
	}
	if !strings.Contains(html, "999") {
		t.Error("small numbers are wrong")
	}
}
