package duckdb_test

import (
	"context"
	"testing"
	"time"

	"github.com/zenith/core/internal/storage"
	"github.com/zenith/core/internal/storage/duckdb"
)

// base is a fixed point in time so every expectation is hand-checkable.
var base = time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)

func day(from time.Time) storage.Query {
	return storage.Query{
		SiteID: "site-1",
		From:   from.Add(-24 * time.Hour),
		To:     from.Add(24 * time.Hour),
	}
}

// seed inserts events and returns a store holding them.
func seed(t *testing.T, events ...storage.Event) *duckdb.Store {
	t.Helper()

	s := open(t)
	if err := s.Insert(context.Background(), events...); err != nil {
		t.Fatalf("insert: %v", err)
	}
	return s
}

func pageview(visitor, path string, at time.Time) storage.Event {
	return storage.Event{
		SiteID: "site-1", TS: at, Type: "pageview",
		Path: path, VisitorHash: visitor,
	}
}

// A visitor idle longer than the session gap starts a new session; the same
// visitor browsing continuously is one session.
func TestSummaryCountsSessionsByGap(t *testing.T) {
	s := seed(t,
		// v1: two pages five minutes apart, then a return 90 minutes later.
		// One gap exceeds 30 minutes, so: 2 sessions.
		pageview("v1", "/a", base),
		pageview("v1", "/b", base.Add(5*time.Minute)),
		pageview("v1", "/c", base.Add(95*time.Minute)),
		// v2: a single page. 1 session.
		pageview("v2", "/a", base),
	)

	got, err := s.Summary(context.Background(), day(base))
	if err != nil {
		t.Fatalf("summary: %v", err)
	}

	want := storage.Summary{Pageviews: 4, Visitors: 2, Sessions: 3}
	if got != want {
		t.Errorf("summary = %+v, want %+v", got, want)
	}
}

// Exactly at the gap boundary is still the same session: the rule is "longer
// than", not "at least".
func TestSummarySessionBoundaryIsExclusive(t *testing.T) {
	s := seed(t,
		pageview("v1", "/a", base),
		pageview("v1", "/b", base.Add(storage.SessionGap)),
	)

	got, err := s.Summary(context.Background(), day(base))
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if got.Sessions != 1 {
		t.Errorf("sessions = %d, want 1: a gap of exactly %s should not split a session",
			got.Sessions, storage.SessionGap)
	}
}

// Custom events count toward visitors and sessions, but are not pageviews.
func TestSummaryCustomEventsAreNotPageviews(t *testing.T) {
	s := seed(t,
		pageview("v1", "/signup", base),
		storage.Event{
			SiteID: "site-1", TS: base.Add(time.Minute), Type: "event",
			EventName: "signup", VisitorHash: "v1",
		},
	)

	got, err := s.Summary(context.Background(), day(base))
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if got.Pageviews != 1 {
		t.Errorf("pageviews = %d, want 1: a custom event is not a pageview", got.Pageviews)
	}
	if got.Visitors != 1 || got.Sessions != 1 {
		t.Errorf("visitors/sessions = %d/%d, want 1/1", got.Visitors, got.Sessions)
	}
}

// The range is half-open: [from, to). Adjacent periods must tile exactly.
func TestSummaryRangeIsHalfOpen(t *testing.T) {
	from := base
	to := base.Add(time.Hour)

	s := seed(t,
		pageview("v1", "/before", from.Add(-time.Second)), // outside
		pageview("v2", "/at-from", from),                  // inside: inclusive
		pageview("v3", "/at-to", to),                      // outside: exclusive
	)

	got, err := s.Summary(context.Background(), storage.Query{
		SiteID: "site-1", From: from, To: to,
	})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if got.Pageviews != 1 {
		t.Errorf("pageviews = %d, want 1: only the event at `from` is in [from, to)", got.Pageviews)
	}
}

// One site's traffic must never appear in another's numbers.
func TestQueriesAreScopedBySite(t *testing.T) {
	ctx := context.Background()
	s := open(t)

	err := s.Insert(ctx,
		pageview("v1", "/a", base),
		storage.Event{SiteID: "site-2", TS: base, Type: "pageview", Path: "/other", VisitorHash: "v9"},
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := s.Summary(ctx, day(base))
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if got.Pageviews != 1 {
		t.Errorf("pageviews = %d, want 1: another site's traffic leaked in", got.Pageviews)
	}

	pages, err := s.Breakdown(ctx, day(base), storage.DimPath)
	if err != nil {
		t.Fatalf("breakdown: %v", err)
	}
	for _, p := range pages {
		if p.Label == "/other" {
			t.Error("another site's page appeared in this site's breakdown")
		}
	}
}

// A query with no site would read every client's traffic at once.
func TestQueryWithoutSiteIsRejected(t *testing.T) {
	ctx := context.Background()
	s := open(t)

	q := storage.Query{From: base, To: base.Add(time.Hour)}

	if _, err := s.Summary(ctx, q); err == nil {
		t.Error("Summary ran without a site id")
	}
	if _, err := s.Breakdown(ctx, q, storage.DimPath); err == nil {
		t.Error("Breakdown ran without a site id")
	}
	if _, err := s.Timeseries(ctx, q, storage.GranularityDay); err == nil {
		t.Error("Timeseries ran without a site id")
	}
	if _, err := s.Realtime(ctx, ""); err == nil {
		t.Error("Realtime ran without a site id")
	}
}

func TestQueryWithBadRangeIsRejected(t *testing.T) {
	ctx := context.Background()
	s := open(t)

	cases := map[string]storage.Query{
		"no from":        {SiteID: "site-1", To: base},
		"no to":          {SiteID: "site-1", From: base},
		"to before from": {SiteID: "site-1", From: base, To: base.Add(-time.Hour)},
		"to equals from": {SiteID: "site-1", From: base, To: base},
	}

	for name, q := range cases {
		if _, err := s.Summary(ctx, q); err == nil {
			t.Errorf("%s: Summary accepted the range", name)
		}
	}
}

// A quiet hour must appear as a zero, not vanish. Otherwise the chart draws a
// line straight across the gap it exists to show.
func TestTimeseriesFillsEmptyBuckets(t *testing.T) {
	s := seed(t,
		pageview("v1", "/a", base),                  // 10:00
		pageview("v2", "/b", base.Add(2*time.Hour)), // 12:00, nothing at 11:00
	)

	buckets, err := s.Timeseries(context.Background(), storage.Query{
		SiteID: "site-1", From: base, To: base.Add(3 * time.Hour),
	}, storage.GranularityHour)
	if err != nil {
		t.Fatalf("timeseries: %v", err)
	}

	if len(buckets) != 3 {
		t.Fatalf("got %d buckets, want 3 (10:00, 11:00, 12:00)", len(buckets))
	}
	want := []int64{1, 0, 1}
	for i, b := range buckets {
		if b.Pageviews != want[i] {
			t.Errorf("bucket %d (%s): pageviews = %d, want %d",
				i, b.TS.Format(time.RFC3339), b.Pageviews, want[i])
		}
	}
	if !buckets[1].TS.Equal(base.Add(time.Hour)) {
		t.Errorf("empty bucket is at %s, want %s", buckets[1].TS, base.Add(time.Hour))
	}
}

func TestTimeseriesBucketsAreOrdered(t *testing.T) {
	s := seed(t,
		pageview("v1", "/a", base.Add(3*time.Hour)),
		pageview("v2", "/b", base),
		pageview("v3", "/c", base.Add(time.Hour)),
	)

	buckets, err := s.Timeseries(context.Background(), storage.Query{
		SiteID: "site-1", From: base, To: base.Add(4 * time.Hour),
	}, storage.GranularityHour)
	if err != nil {
		t.Fatalf("timeseries: %v", err)
	}

	for i := 1; i < len(buckets); i++ {
		if !buckets[i].TS.After(buckets[i-1].TS) {
			t.Fatalf("bucket %d (%s) is not after bucket %d (%s)",
				i, buckets[i].TS, i-1, buckets[i-1].TS)
		}
	}
}

// DuckDB's date_trunc('week') starts on Monday; Go's Weekday starts on Sunday.
// If the fill disagreed with the query, every week bucket would be a zero.
func TestTimeseriesWeekBucketsAlignWithQuery(t *testing.T) {
	// A Friday.
	friday := time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)
	if friday.Weekday() != time.Friday {
		t.Fatalf("fixture is a %s, expected Friday", friday.Weekday())
	}

	s := seed(t, pageview("v1", "/a", friday))

	buckets, err := s.Timeseries(context.Background(), storage.Query{
		SiteID: "site-1", From: friday.AddDate(0, 0, -3), To: friday.AddDate(0, 0, 3),
	}, storage.GranularityWeek)
	if err != nil {
		t.Fatalf("timeseries: %v", err)
	}

	var total int64
	for _, b := range buckets {
		total += b.Pageviews
		if b.TS.Weekday() != time.Monday {
			t.Errorf("week bucket starts on %s, want Monday", b.TS.Weekday())
		}
	}
	if total != 1 {
		t.Errorf("pageviews across week buckets = %d, want 1: the fill missed the query's bucket", total)
	}
}

func TestTimeseriesRejectsUnknownGranularity(t *testing.T) {
	s := open(t)

	_, err := s.Timeseries(context.Background(), day(base), storage.Granularity("century"))
	if err == nil {
		t.Error("accepted granularity \"century\"")
	}
}

func TestBreakdownByPath(t *testing.T) {
	s := seed(t,
		pageview("v1", "/pricing", base),
		pageview("v2", "/pricing", base),
		pageview("v1", "/docs", base),
	)

	got, err := s.Breakdown(context.Background(), day(base), storage.DimPath)
	if err != nil {
		t.Fatalf("breakdown: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2", len(got))
	}
	// Busiest first.
	if got[0].Label != "/pricing" || got[0].Visitors != 2 {
		t.Errorf("top row = %+v, want /pricing with 2 visitors", got[0])
	}
	if got[1].Label != "/docs" || got[1].Visitors != 1 {
		t.Errorf("second row = %+v, want /docs with 1 visitor", got[1])
	}
}

// A top-pages table is ranked by views: that is what makes a page top. Ranking
// it by visitors would let a page with many views sit below one with fewer,
// and -- worse -- be cut by the LIMIT before it was ever shown.
func TestBreakdownByPathRanksByPageviews(t *testing.T) {
	s := seed(t,
		// /popular: one visitor, many views.
		pageview("v1", "/popular", base),
		pageview("v1", "/popular", base.Add(time.Minute)),
		pageview("v1", "/popular", base.Add(2*time.Minute)),
		pageview("v1", "/popular", base.Add(3*time.Minute)),
		// /shallow: two visitors, fewer views.
		pageview("v2", "/shallow", base),
		pageview("v3", "/shallow", base),
	)

	got, err := s.Breakdown(context.Background(), day(base), storage.DimPath)
	if err != nil {
		t.Fatalf("breakdown: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2", len(got))
	}
	if got[0].Label != "/popular" {
		t.Errorf("top page = %q with %d views, want /popular with 4: "+
			"pages must rank by views, not visitors", got[0].Label, got[0].Pageviews)
	}
}

// Everything that is not a page ranks by people, not views.
func TestBreakdownByReferrerRanksByVisitors(t *testing.T) {
	s := seed(t,
		// One devoted reader from a small forum, many views.
		storage.Event{SiteID: "site-1", TS: base, Type: "pageview", Path: "/a",
			VisitorHash: "v1", Referrer: "smallforum.com"},
		storage.Event{SiteID: "site-1", TS: base.Add(time.Minute), Type: "pageview", Path: "/b",
			VisitorHash: "v1", Referrer: "smallforum.com"},
		storage.Event{SiteID: "site-1", TS: base.Add(2 * time.Minute), Type: "pageview", Path: "/c",
			VisitorHash: "v1", Referrer: "smallforum.com"},
		// Two people from a big one.
		storage.Event{SiteID: "site-1", TS: base, Type: "pageview", Path: "/a",
			VisitorHash: "v2", Referrer: "news.ycombinator.com"},
		storage.Event{SiteID: "site-1", TS: base, Type: "pageview", Path: "/a",
			VisitorHash: "v3", Referrer: "news.ycombinator.com"},
	)

	got, err := s.Breakdown(context.Background(), day(base), storage.DimReferrer)
	if err != nil {
		t.Fatalf("breakdown: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2", len(got))
	}
	if got[0].Label != "news.ycombinator.com" {
		t.Errorf("top referrer = %q, want news.ycombinator.com: "+
			"referrers must rank by visitors, not views", got[0].Label)
	}
}

// A page is something that was viewed. A custom event carries the path it
// fired on, and counting it here would put a page nobody ever loaded into the
// table of top pages, showing zero pageviews next to it.
func TestBreakdownByPathCountsPageviewsOnly(t *testing.T) {
	s := seed(t,
		pageview("v1", "/home", base),
		// A signup event fired on a page that was never itself viewed.
		storage.Event{
			SiteID: "site-1", TS: base, Type: "event",
			EventName: "signup", Path: "/signup", VisitorHash: "v1",
		},
	)

	got, err := s.Breakdown(context.Background(), day(base), storage.DimPath)
	if err != nil {
		t.Fatalf("breakdown: %v", err)
	}

	for _, row := range got {
		if row.Label == "/signup" {
			t.Errorf("/signup is in top pages with %d pageviews: a custom event "+
				"is not a page view", row.Pageviews)
		}
		if row.Pageviews == 0 {
			t.Errorf("row %q has zero pageviews and does not belong in top pages", row.Label)
		}
	}
	if len(got) != 1 || got[0].Label != "/home" {
		t.Errorf("top pages = %+v, want only /home", got)
	}
}

// Other dimensions must still count the visitor: someone who fired an event
// without a pageview is still a real visitor from a real country on a real
// browser.
func TestBreakdownByCountryCountsEventOnlyVisitors(t *testing.T) {
	s := seed(t, storage.Event{
		SiteID: "site-1", TS: base, Type: "event",
		EventName: "signup", VisitorHash: "v1", Country: "UG",
	})

	got, err := s.Breakdown(context.Background(), day(base), storage.DimCountry)
	if err != nil {
		t.Fatalf("breakdown: %v", err)
	}

	if len(got) != 1 || got[0].Label != "UG" || got[0].Visitors != 1 {
		t.Errorf("countries = %+v, want UG with 1 visitor", got)
	}
}

// An empty dimension is an absence, not a category: a blank row at the top of
// the referrers table would be noise.
func TestBreakdownSkipsEmptyValues(t *testing.T) {
	s := seed(t,
		storage.Event{SiteID: "site-1", TS: base, Type: "pageview", Path: "/a",
			VisitorHash: "v1", Referrer: "news.ycombinator.com"},
		storage.Event{SiteID: "site-1", TS: base, Type: "pageview", Path: "/a",
			VisitorHash: "v2", Referrer: ""}, // direct traffic
	)

	got, err := s.Breakdown(context.Background(), day(base), storage.DimReferrer)
	if err != nil {
		t.Fatalf("breakdown: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1: direct traffic must not be a referrer row", len(got))
	}
	if got[0].Label != "news.ycombinator.com" {
		t.Errorf("label = %q, want news.ycombinator.com", got[0].Label)
	}
}

// Internal navigation sets a referrer, so without excluding it a site's top
// referrer is always itself.
func TestBreakdownExcludesSelfReferrals(t *testing.T) {
	s := seed(t,
		storage.Event{SiteID: "site-1", TS: base, Type: "pageview", Path: "/b",
			VisitorHash: "v1", Referrer: "example.com"},
		storage.Event{SiteID: "site-1", TS: base, Type: "pageview", Path: "/a",
			VisitorHash: "v2", Referrer: "news.ycombinator.com"},
	)

	q := day(base)
	q.ExcludeReferrer = "example.com"

	got, err := s.Breakdown(context.Background(), q, storage.DimReferrer)
	if err != nil {
		t.Fatalf("breakdown: %v", err)
	}

	for _, row := range got {
		if row.Label == "example.com" {
			t.Error("the site's own domain appeared as a referrer")
		}
	}
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1", len(got))
	}
}

func TestBreakdownRejectsUnknownDimension(t *testing.T) {
	s := open(t)

	// A column that exists but must never be groupable, and one that does not.
	for _, d := range []storage.Dimension{"visitor_hash", "ts", "props", "1; DROP TABLE events"} {
		if _, err := s.Breakdown(context.Background(), day(base), d); err == nil {
			t.Errorf("accepted dimension %q", d)
		}
	}
}

func TestBreakdownRespectsLimit(t *testing.T) {
	events := []storage.Event{}
	for i := range 10 {
		events = append(events, pageview("v"+string(rune('a'+i)), "/p"+string(rune('a'+i)), base))
	}
	s := seed(t, events...)

	q := day(base)
	q.Limit = 3

	got, err := s.Breakdown(context.Background(), q, storage.DimPath)
	if err != nil {
		t.Fatalf("breakdown: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("got %d rows, want 3", len(got))
	}
}

// Nobody needs ten thousand referrer rows, and building them is how one URL
// pins the server.
func TestBreakdownCapsAbsurdLimit(t *testing.T) {
	s := seed(t, pageview("v1", "/a", base))

	q := day(base)
	q.Limit = 1_000_000

	if _, err := s.Breakdown(context.Background(), q, storage.DimPath); err != nil {
		t.Fatalf("breakdown: %v", err)
	}
	// The cap is enforced in SQL; this asserts it does not error or hang.
}

func TestEntryAndExitPages(t *testing.T) {
	ctx := context.Background()
	s := seed(t,
		// v1: /home -> /pricing -> /checkout, one session.
		pageview("v1", "/home", base),
		pageview("v1", "/pricing", base.Add(2*time.Minute)),
		pageview("v1", "/checkout", base.Add(4*time.Minute)),
		// v2: lands on /pricing and leaves.
		pageview("v2", "/pricing", base),
	)

	entries, err := s.EntryPages(ctx, day(base))
	if err != nil {
		t.Fatalf("entry pages: %v", err)
	}

	entryCounts := map[string]int64{}
	for _, e := range entries {
		entryCounts[e.Label] = e.Pageviews
	}
	if entryCounts["/home"] != 1 || entryCounts["/pricing"] != 1 {
		t.Errorf("entries = %v, want /home:1 and /pricing:1", entryCounts)
	}
	if _, found := entryCounts["/checkout"]; found {
		t.Error("/checkout was never an entry page")
	}

	exits, err := s.ExitPages(ctx, day(base))
	if err != nil {
		t.Fatalf("exit pages: %v", err)
	}

	exitCounts := map[string]int64{}
	for _, e := range exits {
		exitCounts[e.Label] = e.Pageviews
	}
	if exitCounts["/checkout"] != 1 || exitCounts["/pricing"] != 1 {
		t.Errorf("exits = %v, want /checkout:1 and /pricing:1", exitCounts)
	}
	if _, found := exitCounts["/home"]; found {
		t.Error("/home was never an exit page")
	}
}

// A returning visitor starts a second session, so their landing page is an
// entry twice.
func TestEntryPagesCountEachSession(t *testing.T) {
	s := seed(t,
		pageview("v1", "/home", base),
		pageview("v1", "/docs", base.Add(2*time.Minute)),
		// Returns after the gap: a second session, entering at /home again.
		pageview("v1", "/home", base.Add(90*time.Minute)),
	)

	entries, err := s.EntryPages(context.Background(), day(base))
	if err != nil {
		t.Fatalf("entry pages: %v", err)
	}

	for _, e := range entries {
		if e.Label == "/home" && e.Pageviews != 2 {
			t.Errorf("/home entries = %d, want 2 (one per session)", e.Pageviews)
		}
	}
}

func TestEvents(t *testing.T) {
	custom := func(visitor, name string, at time.Time, props map[string]any) storage.Event {
		return storage.Event{
			SiteID: "site-1", TS: at, Type: "event",
			EventName: name, VisitorHash: visitor, Props: props,
		}
	}

	s := seed(t,
		custom("v1", "signup", base, map[string]any{"plan": "pro"}),
		custom("v2", "signup", base, map[string]any{"plan": "free"}),
		custom("v1", "download", base, nil),
		pageview("v1", "/", base),
	)

	got, err := s.Events(context.Background(), day(base))
	if err != nil {
		t.Fatalf("events: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("got %d events, want 2", len(got))
	}
	if got[0].Name != "signup" || got[0].Count != 2 || got[0].Visitors != 2 {
		t.Errorf("top event = %+v, want signup count 2 visitors 2", got[0])
	}
	if got[1].Name != "download" || got[1].Count != 1 {
		t.Errorf("second event = %+v, want download count 1", got[1])
	}
}

func TestEventProps(t *testing.T) {
	custom := func(visitor string, props map[string]any) storage.Event {
		return storage.Event{
			SiteID: "site-1", TS: base, Type: "event",
			EventName: "signup", VisitorHash: visitor, Props: props,
		}
	}

	s := seed(t,
		custom("v1", map[string]any{"plan": "pro", "seats": 3}),
		custom("v2", map[string]any{"plan": "pro", "seats": 1}),
		custom("v3", map[string]any{"plan": "free"}),
	)

	got, err := s.EventProps(context.Background(), day(base), "signup")
	if err != nil {
		t.Fatalf("event props: %v", err)
	}

	counts := map[string]int64{}
	for _, p := range got {
		counts[p.Key+"="+p.Value] = p.Count
	}

	if counts["plan=pro"] != 2 {
		t.Errorf("plan=pro count = %d, want 2 (got %v)", counts["plan=pro"], counts)
	}
	if counts["plan=free"] != 1 {
		t.Errorf("plan=free count = %d, want 1", counts["plan=free"])
	}
	if counts["seats=3"] != 1 {
		t.Errorf("seats=3 count = %d, want 1", counts["seats=3"])
	}
}

func TestEventPropsRequiresName(t *testing.T) {
	s := open(t)

	if _, err := s.EventProps(context.Background(), day(base), ""); err == nil {
		t.Error("accepted an empty event name")
	}
}

func TestRealtimeCountsRecentVisitorsOnly(t *testing.T) {
	now := time.Now().UTC()

	s := seed(t,
		pageview("v1", "/a", now.Add(-time.Minute)),   // active
		pageview("v2", "/b", now.Add(-2*time.Minute)), // active
		pageview("v1", "/c", now.Add(-3*time.Minute)), // same visitor again
		pageview("v3", "/d", now.Add(-time.Hour)),     // long gone
	)

	got, err := s.Realtime(context.Background(), "site-1")
	if err != nil {
		t.Fatalf("realtime: %v", err)
	}
	if got != 2 {
		t.Errorf("realtime = %d, want 2 (v1 and v2, counted once each)", got)
	}
}

// Empty results must encode as [] rather than null.
func TestBreakdownsAreNeverNil(t *testing.T) {
	ctx := context.Background()
	s := open(t) // no events at all

	pages, err := s.Breakdown(ctx, day(base), storage.DimPath)
	if err != nil {
		t.Fatalf("breakdown: %v", err)
	}
	if pages == nil {
		t.Error("empty breakdown is nil, want an empty slice")
	}

	entries, err := s.EntryPages(ctx, day(base))
	if err != nil {
		t.Fatalf("entry pages: %v", err)
	}
	if entries == nil {
		t.Error("empty entry pages is nil, want an empty slice")
	}

	events, err := s.Events(ctx, day(base))
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if events == nil {
		t.Error("empty events is nil, want an empty slice")
	}
}

// An empty period is a real answer, not an error.
func TestSummaryOfEmptyPeriod(t *testing.T) {
	s := open(t)

	got, err := s.Summary(context.Background(), day(base))
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if (got != storage.Summary{}) {
		t.Errorf("summary of an empty period = %+v, want all zeroes", got)
	}
}
