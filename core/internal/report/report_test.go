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
	if data.TopCountries[0].Name != "Uganda" {
		t.Errorf("country = %q, want Uganda", data.TopCountries[0].Name)
	}
	// The code is kept alongside the name: the layout shows both.
	if data.TopCountries[0].Code != "UG" {
		t.Errorf("code = %q, want UG", data.TopCountries[0].Code)
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

// A report sent on demand covers the month in progress. A site added this
// month has no finished month, and mailing a client zeroes for a month that
// predates their site would be worse than sending nothing.
func TestMonthToDateCoversThisMonth(t *testing.T) {
	now := time.Date(2026, 7, 20, 15, 0, 0, 0, time.UTC)
	w := report.MonthToDate(now)

	if w.Period != "2026-07" {
		t.Errorf("period = %q, want 2026-07", w.Period)
	}
	if !strings.Contains(w.Label, "July 2026") || !strings.Contains(w.Label, "so far") {
		t.Errorf("label = %q, want it to say July 2026 and that it is partial", w.Label)
	}
}

// The comparison must be like for like: the first 20 days of July against the
// first 20 of June, not against the whole of June -- which would make every
// month-to-date report look like a collapse.
func TestMonthToDateComparesTheSameRunOfDays(t *testing.T) {
	june := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	july := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	s := store(t,
		// Five visitors early in June, then five more late in June that a
		// month-to-date window ending on the 10th must not count.
		view(june.AddDate(0, 0, 1), "a", "/"),
		view(june.AddDate(0, 0, 2), "b", "/"),
		view(june.AddDate(0, 0, 20), "c", "/"),
		view(june.AddDate(0, 0, 21), "d", "/"),
		view(june.AddDate(0, 0, 22), "e", "/"),

		// Two in the first ten days of July.
		view(july.AddDate(0, 0, 1), "f", "/"),
		view(july.AddDate(0, 0, 2), "g", "/"),
	)

	now := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	data, err := report.BuildWindow(context.Background(), s, site, report.MonthToDate(now))
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if data.Summary.Visitors != 2 {
		t.Errorf("visitors = %d, want the 2 so far this month", data.Summary.Visitors)
	}
	// June 1-10 had two visitors; the three later in June are outside the
	// comparable window.
	if data.Previous.Visitors != 2 {
		t.Errorf("previous = %d, want the 2 from the same days of June", data.Previous.Visitors)
	}
}

// The delta caption has to name what it compared against, or a month-to-date
// number reads as a full-month one.
func TestMonthToDateSaysWhatItComparedAgainst(t *testing.T) {
	s := store(t)
	now := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)

	data, err := report.BuildWindow(context.Background(), s, site, report.MonthToDate(now))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(data.CompareLabel, "same days") {
		t.Errorf("compare label = %q, want it to say it is the same days", data.CompareLabel)
	}
}

// The link is the whole point of recording a dashboard path; without one the
// button must be omitted rather than guessed at.
func TestDashboardURLOnlyWhenAPathIsRecorded(t *testing.T) {
	s := store(t)

	withPath := site
	withPath.DashboardPath = "/zenith"

	data, err := report.Build(context.Background(), s, withPath, "2026-06")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if data.DashboardURL != "https://acme.com/zenith" {
		t.Errorf("url = %q, want https://acme.com/zenith", data.DashboardURL)
	}

	bare, err := report.Build(context.Background(), s, site, "2026-06")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if bare.DashboardURL != "" {
		t.Errorf("url = %q, want empty when no path is recorded", bare.DashboardURL)
	}
}

// The email a client actually receives. Every section the report can carry has
// to survive Build and Render together -- unit-testing the data and trusting
// the template would miss a block guarded on the wrong field.
func TestRenderedReportCarriesEverySection(t *testing.T) {
	june := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

	events := []storage.Event{
		{SiteID: "site-1", TS: june, Type: "pageview", Path: "/pricing",
			VisitorHash: "a", Referrer: "news.ycombinator.com", Country: "UG", Device: "mobile"},
		{SiteID: "site-1", TS: june, Type: "pageview", Path: "/pricing",
			VisitorHash: "b", Referrer: "google.com", Country: "KE", Device: "desktop"},
	}

	withDashboard := site
	withDashboard.DashboardPath = "/zenith"

	data, err := report.Build(context.Background(), store(t, events...), withDashboard, "2026-06")
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	html, err := report.Render(data)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	for _, want := range []string{
		"Top pages", "/pricing",
		"Top referrers", "news.ycombinator.com",
		// Resolved, because an email has no JavaScript to do it with.
		"Top countries", "Uganda",
		"Devices", "mobile",
		// The link that had nowhere to point until a path was recorded.
		"https://acme.com/zenith", "View full dashboard",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the email is missing %q", want)
		}
	}
}

// The three "how well" numbers a client actually asks about, alongside the
// three "how much" ones. Rendered as text rather than counts, so this checks
// the formatters survive the template too.
func TestRenderedReportCarriesTheRateMetrics(t *testing.T) {
	july := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)

	s := store(t,
		// One session of two pages two minutes apart, and one bounce.
		view(july, "a", "/"),
		view(july.Add(2*time.Minute), "a", "/pricing"),
		view(july, "b", "/"),
	)

	data, err := report.Build(context.Background(), s, site, "2026-07")
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	html, err := report.Render(data)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	for _, want := range []string{
		"Views per visit", "1.50",
		// One of two sessions saw a single page.
		"Bounce rate", "50%",
		// (120s + 0s) / 2 sessions.
		"Visit duration", "1m",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the email is missing %q", want)
		}
	}
}
