package http_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/zenith/core/internal/storage"
)

// statsBase is a fixed point in time so expectations are hand-checkable.
var statsBase = time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)

// seedTraffic writes events straight to the store, bypassing HTTP: these tests
// are about reading, not ingesting.
func seedTraffic(t *testing.T, h *harness, events ...storage.Event) {
	t.Helper()

	if err := h.events.EventStore.Insert(context.Background(), events...); err != nil {
		t.Fatalf("seed traffic: %v", err)
	}
}

func view(site, visitor, path string, at time.Time) storage.Event {
	return storage.Event{
		SiteID: site, TS: at, Type: "pageview",
		Path: path, VisitorHash: visitor,
	}
}

// statsURL builds a stats request covering a day either side of statsBase.
func statsURL(path, site string, extra ...string) string {
	from := statsBase.Add(-24 * time.Hour).Format(time.RFC3339)
	to := statsBase.Add(24 * time.Hour).Format(time.RFC3339)

	url := fmt.Sprintf("/api/stats/%s?site=%s&from=%s&to=%s", path, site, from, to)
	for _, e := range extra {
		url += "&" + e
	}
	return url
}

// ownerToken mints a token scoped to one site, as the domain-native proxy will
// in phase 7.
func ownerToken(t *testing.T, h *harness, siteID string) string {
	t.Helper()

	token, _, err := h.issuer.IssueOwner(siteID)
	if err != nil {
		t.Fatalf("issue owner token: %v", err)
	}
	return token
}

func decode[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}

	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func TestSummaryEndpoint(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := h.login(t)

	seedTraffic(t, h,
		view("site-1", "v1", "/a", statsBase),
		view("site-1", "v1", "/b", statsBase.Add(5*time.Minute)),
		view("site-1", "v2", "/a", statsBase),
	)

	body := decode[struct {
		Pageviews int64 `json:"pageviews"`
		Visitors  int64 `json:"visitors"`
		Sessions  int64 `json:"sessions"`
	}](t, h.get(t, statsURL("summary", "site-1"), token))

	if body.Pageviews != 3 || body.Visitors != 2 || body.Sessions != 2 {
		t.Errorf("summary = %+v, want pageviews 3, visitors 2, sessions 2", body)
	}
}

// This is the boundary the product rests on: an owner's token names their
// site, so asking for another one must fail rather than answer.
func TestOwnerCannotReadAnotherSite(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)

	// A second site the owner has no claim to.
	if err := h.app.CreateSite(context.Background(), storage.Site{
		ID: "site-2", Name: "Someone Else", Domain: "other.com",
		SiteKey: "zk_pub_other", APIKey: "zk_sec_other",
	}); err != nil {
		t.Fatalf("create second site: %v", err)
	}

	seedTraffic(t, h, view("site-2", "v9", "/secret", statsBase))

	token := ownerToken(t, h, "site-1")

	resp := h.get(t, statsURL("summary", "site-2"), token)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d, want 403: an owner read another client's site", resp.StatusCode)
	}
}

// An owner naming no site gets their own, not an error and not everything.
func TestOwnerDefaultsToTheirOwnSite(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)

	seedTraffic(t, h, view("site-1", "v1", "/a", statsBase))

	token := ownerToken(t, h, "site-1")

	from := statsBase.Add(-24 * time.Hour).Format(time.RFC3339)
	to := statsBase.Add(24 * time.Hour).Format(time.RFC3339)

	body := decode[struct {
		Pageviews int64 `json:"pageviews"`
	}](t, h.get(t, "/api/stats/summary?from="+from+"&to="+to, token))

	if body.Pageviews != 1 {
		t.Errorf("pageviews = %d, want 1", body.Pageviews)
	}
}

// A developer sees every site, but must say which one.
func TestDeveloperMustNameASite(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := h.login(t)

	resp := h.get(t, "/api/stats/summary", token)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status %d, want 400", resp.StatusCode)
	}
}

func TestStatsRequireAuth(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)

	for _, path := range []string{
		"summary", "timeseries", "pages", "referrers", "geo", "tech", "events", "realtime",
	} {
		resp := h.get(t, statsURL(path, "site-1"), "")
		resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s: status %d, want 401", path, resp.StatusCode)
		}
	}
}

func TestStatsForUnknownSite(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := h.login(t)

	resp := h.get(t, statsURL("summary", "no-such-site"), token)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status %d, want 404", resp.StatusCode)
	}
}

// The previous period must be the equal-length span immediately before, and
// the two must tile exactly.
func TestSummaryComparison(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := h.login(t)

	from := statsBase
	to := statsBase.Add(time.Hour)

	seedTraffic(t, h,
		// Current period: 2 pageviews.
		view("site-1", "v1", "/a", from),
		view("site-1", "v2", "/b", from.Add(30*time.Minute)),
		// Previous period (the hour before): 1 pageview.
		view("site-1", "v3", "/c", from.Add(-30*time.Minute)),
		// Older still: must not be counted in either.
		view("site-1", "v4", "/d", from.Add(-3*time.Hour)),
	)

	url := fmt.Sprintf("/api/stats/summary?site=site-1&from=%s&to=%s&compare=true",
		from.Format(time.RFC3339), to.Format(time.RFC3339))

	body := decode[struct {
		Pageviews int64 `json:"pageviews"`
		Previous  *struct {
			Pageviews int64 `json:"pageviews"`
		} `json:"previous"`
		Change *struct {
			Pageviews *float64 `json:"pageviews"`
		} `json:"change"`
	}](t, h.get(t, url, token))

	if body.Pageviews != 2 {
		t.Errorf("pageviews = %d, want 2", body.Pageviews)
	}
	if body.Previous == nil {
		t.Fatal("no previous period in the response")
	}
	if body.Previous.Pageviews != 1 {
		t.Errorf("previous pageviews = %d, want 1", body.Previous.Pageviews)
	}
	if body.Change == nil || body.Change.Pageviews == nil {
		t.Fatal("no change in the response")
	}
	// 1 -> 2 is +100%.
	if *body.Change.Pageviews != 100 {
		t.Errorf("change = %v, want 100", *body.Change.Pageviews)
	}
}

// Growth from zero has no percentage. Reporting 0% or +100% would both be
// lies, so the field is null and the interface can say something true.
func TestComparisonFromZeroHasNoPercentage(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := h.login(t)

	from := statsBase
	to := statsBase.Add(time.Hour)

	// Traffic now, nothing in the previous hour.
	seedTraffic(t, h, view("site-1", "v1", "/a", from))

	url := fmt.Sprintf("/api/stats/summary?site=site-1&from=%s&to=%s&compare=true",
		from.Format(time.RFC3339), to.Format(time.RFC3339))

	body := decode[struct {
		Change *struct {
			Pageviews *float64 `json:"pageviews"`
		} `json:"change"`
	}](t, h.get(t, url, token))

	if body.Change == nil {
		t.Fatal("no change object in the response")
	}
	if body.Change.Pageviews != nil {
		t.Errorf("change = %v, want null: there is no percentage change from zero",
			*body.Change.Pageviews)
	}
}

// Without ?compare, the comparison fields must be absent rather than zero.
func TestComparisonIsOptional(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := h.login(t)

	resp := h.get(t, statsURL("summary", "site-1"), token)
	defer resp.Body.Close()

	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if _, found := raw["previous"]; found {
		t.Error("previous is present without ?compare=true")
	}
	if _, found := raw["change"]; found {
		t.Error("change is present without ?compare=true")
	}
}

func TestTimeseriesEndpoint(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := h.login(t)

	seedTraffic(t, h,
		view("site-1", "v1", "/a", statsBase),
		view("site-1", "v2", "/b", statsBase.Add(2*time.Hour)),
	)

	url := fmt.Sprintf("/api/stats/timeseries?site=site-1&from=%s&to=%s&granularity=hour",
		statsBase.Format(time.RFC3339), statsBase.Add(3*time.Hour).Format(time.RFC3339))

	body := decode[struct {
		Granularity string `json:"granularity"`
		Buckets     []struct {
			TS        time.Time `json:"ts"`
			Pageviews int64     `json:"pageviews"`
		} `json:"buckets"`
	}](t, h.get(t, url, token))

	if body.Granularity != "hour" {
		t.Errorf("granularity = %q, want hour", body.Granularity)
	}
	if len(body.Buckets) != 3 {
		t.Fatalf("got %d buckets, want 3", len(body.Buckets))
	}
	// The quiet hour must be present as a zero.
	if body.Buckets[1].Pageviews != 0 {
		t.Errorf("middle bucket = %d, want 0", body.Buckets[1].Pageviews)
	}
}

func TestTimeseriesRejectsBadGranularity(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := h.login(t)

	resp := h.get(t, statsURL("timeseries", "site-1", "granularity=century"), token)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status %d, want 400", resp.StatusCode)
	}
}

// A year of hourly buckets is thousands of points nobody can read. The default
// must scale with the range.
func TestTimeseriesGranularityDefaultsToTheRange(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := h.login(t)

	cases := map[string]struct {
		span time.Duration
		want string
	}{
		"one day":    {24 * time.Hour, "hour"},
		"one month":  {30 * 24 * time.Hour, "day"},
		"one year":   {365 * 24 * time.Hour, "week"},
		"five years": {5 * 365 * 24 * time.Hour, "month"},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			to := statsBase
			from := to.Add(-c.span)

			url := fmt.Sprintf("/api/stats/timeseries?site=site-1&from=%s&to=%s",
				from.Format(time.RFC3339), to.Format(time.RFC3339))

			body := decode[struct {
				Granularity string `json:"granularity"`
			}](t, h.get(t, url, token))

			if body.Granularity != c.want {
				t.Errorf("granularity = %q, want %q", body.Granularity, c.want)
			}
		})
	}
}

func TestPagesEndpoint(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := h.login(t)

	seedTraffic(t, h,
		view("site-1", "v1", "/home", statsBase),
		view("site-1", "v1", "/pricing", statsBase.Add(2*time.Minute)),
		view("site-1", "v2", "/home", statsBase),
	)

	body := decode[struct {
		Top   []countJSONTest `json:"top"`
		Entry []countJSONTest `json:"entry"`
		Exit  []countJSONTest `json:"exit"`
	}](t, h.get(t, statsURL("pages", "site-1"), token))

	if len(body.Top) != 2 {
		t.Fatalf("got %d top pages, want 2", len(body.Top))
	}
	if body.Top[0].Label != "/home" || body.Top[0].Visitors != 2 {
		t.Errorf("top page = %+v, want /home with 2 visitors", body.Top[0])
	}
	if len(body.Entry) == 0 || body.Entry[0].Label != "/home" {
		t.Errorf("entry pages = %+v, want /home first", body.Entry)
	}
	if len(body.Exit) == 0 {
		t.Error("no exit pages")
	}
}

type countJSONTest struct {
	Label     string `json:"label"`
	Visitors  int64  `json:"visitors"`
	Pageviews int64  `json:"pageviews"`
}

func TestReferrersEndpoint(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := h.login(t)

	seedTraffic(t,
		h,
		storage.Event{
			SiteID: "site-1", TS: statsBase, Type: "pageview", Path: "/",
			VisitorHash: "v1", Referrer: "news.ycombinator.com",
			UTMSource: "hn", UTMMedium: "social", UTMCampaign: "launch",
		},
		// The site's own domain: internal navigation, not a referral.
		storage.Event{
			SiteID: "site-1", TS: statsBase, Type: "pageview", Path: "/b",
			VisitorHash: "v1", Referrer: "example.com",
		},
	)

	body := decode[struct {
		Sources []countJSONTest `json:"sources"`
		UTM     struct {
			Source   []countJSONTest `json:"source"`
			Campaign []countJSONTest `json:"campaign"`
		} `json:"utm"`
	}](t, h.get(t, statsURL("referrers", "site-1"), token))

	for _, s := range body.Sources {
		if s.Label == "example.com" {
			t.Error("the site's own domain appeared as a referrer: internal navigation is not a referral")
		}
	}
	if len(body.Sources) != 1 || body.Sources[0].Label != "news.ycombinator.com" {
		t.Errorf("sources = %+v, want only news.ycombinator.com", body.Sources)
	}
	if len(body.UTM.Source) != 1 || body.UTM.Source[0].Label != "hn" {
		t.Errorf("utm sources = %+v, want hn", body.UTM.Source)
	}
	if len(body.UTM.Campaign) != 1 || body.UTM.Campaign[0].Label != "launch" {
		t.Errorf("utm campaigns = %+v, want launch", body.UTM.Campaign)
	}
}

func TestGeoAndTechEndpoints(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := h.login(t)

	seedTraffic(t, h, storage.Event{
		SiteID: "site-1", TS: statsBase, Type: "pageview", Path: "/",
		VisitorHash: "v1", Country: "UG", Device: "desktop",
		Browser: "Firefox", OS: "Linux",
	})

	geo := decode[struct {
		Countries []countJSONTest `json:"countries"`
	}](t, h.get(t, statsURL("geo", "site-1"), token))

	if len(geo.Countries) != 1 || geo.Countries[0].Label != "UG" {
		t.Errorf("countries = %+v, want UG", geo.Countries)
	}

	tech := decode[struct {
		Devices  []countJSONTest `json:"devices"`
		Browsers []countJSONTest `json:"browsers"`
		OS       []countJSONTest `json:"os"`
	}](t, h.get(t, statsURL("tech", "site-1"), token))

	if len(tech.Devices) != 1 || tech.Devices[0].Label != "desktop" {
		t.Errorf("devices = %+v, want desktop", tech.Devices)
	}
	if len(tech.Browsers) != 1 || tech.Browsers[0].Label != "Firefox" {
		t.Errorf("browsers = %+v, want Firefox", tech.Browsers)
	}
	if len(tech.OS) != 1 || tech.OS[0].Label != "Linux" {
		t.Errorf("os = %+v, want Linux", tech.OS)
	}
}

func TestEventsEndpoint(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := h.login(t)

	seedTraffic(t, h,
		storage.Event{
			SiteID: "site-1", TS: statsBase, Type: "event", EventName: "signup",
			VisitorHash: "v1", Props: map[string]any{"plan": "pro"},
		},
		storage.Event{
			SiteID: "site-1", TS: statsBase, Type: "event", EventName: "signup",
			VisitorHash: "v2", Props: map[string]any{"plan": "free"},
		},
	)

	body := decode[struct {
		Events []struct {
			Name     string `json:"name"`
			Count    int64  `json:"count"`
			Visitors int64  `json:"visitors"`
		} `json:"events"`
		Props []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
			Count int64  `json:"count"`
		} `json:"props"`
	}](t, h.get(t, statsURL("events", "site-1"), token))

	if len(body.Events) != 1 || body.Events[0].Name != "signup" || body.Events[0].Count != 2 {
		t.Errorf("events = %+v, want signup with count 2", body.Events)
	}
	// No ?name, so no property breakdown.
	if len(body.Props) != 0 {
		t.Errorf("props = %+v, want none without ?name", body.Props)
	}
}

func TestEventPropsBreakdown(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := h.login(t)

	seedTraffic(t, h,
		storage.Event{
			SiteID: "site-1", TS: statsBase, Type: "event", EventName: "signup",
			VisitorHash: "v1", Props: map[string]any{"plan": "pro"},
		},
		storage.Event{
			SiteID: "site-1", TS: statsBase, Type: "event", EventName: "signup",
			VisitorHash: "v2", Props: map[string]any{"plan": "pro"},
		},
	)

	body := decode[struct {
		Props []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
			Count int64  `json:"count"`
		} `json:"props"`
	}](t, h.get(t, statsURL("events", "site-1", "name=signup"), token))

	if len(body.Props) != 1 {
		t.Fatalf("props = %+v, want one row", body.Props)
	}
	if body.Props[0].Key != "plan" || body.Props[0].Value != "pro" || body.Props[0].Count != 2 {
		t.Errorf("props[0] = %+v, want plan=pro count 2", body.Props[0])
	}
}

func TestRealtimeEndpoint(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := h.login(t)

	now := time.Now().UTC()
	seedTraffic(t, h,
		view("site-1", "v1", "/a", now.Add(-time.Minute)),
		view("site-1", "v2", "/b", now.Add(-2*time.Minute)),
		view("site-1", "v3", "/c", now.Add(-time.Hour)), // long gone
	)

	body := decode[struct {
		Visitors      int64 `json:"visitors"`
		WindowSeconds int   `json:"window_seconds"`
	}](t, h.get(t, "/api/stats/realtime?site=site-1", token))

	if body.Visitors != 2 {
		t.Errorf("visitors = %d, want 2", body.Visitors)
	}
	if body.WindowSeconds != int(storage.RealtimeWindow.Seconds()) {
		t.Errorf("window_seconds = %d, want %d", body.WindowSeconds, int(storage.RealtimeWindow.Seconds()))
	}
}

// A bare `to` date must cover that whole day. Otherwise "July 1 to July 31"
// silently drops the last day.
func TestBareDateRangeCoversWholeDays(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := h.login(t)

	// Late on the 17th: inside "to=2026-07-17" only if the day is covered.
	seedTraffic(t, h, view("site-1", "v1", "/a",
		time.Date(2026, 7, 17, 23, 30, 0, 0, time.UTC)))

	body := decode[struct {
		Pageviews int64 `json:"pageviews"`
	}](t, h.get(t, "/api/stats/summary?site=site-1&from=2026-07-17&to=2026-07-17", token))

	if body.Pageviews != 1 {
		t.Errorf("pageviews = %d, want 1: a bare `to` date must cover the whole day", body.Pageviews)
	}
}

func TestBadRangeIsRejected(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := h.login(t)

	cases := map[string]string{
		"to before from": "from=2026-07-17&to=2026-07-01",
		"garbage from":   "from=yesterday&to=2026-07-17",
		"garbage to":     "from=2026-07-01&to=tomorrow",
		"only from":      "from=2026-07-01",
		"only to":        "to=2026-07-17",
	}

	for name, params := range cases {
		resp := h.get(t, "/api/stats/summary?site=site-1&"+params, token)
		resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, resp.StatusCode)
		}
	}
}

// No range at all is a valid request: it means "recently".
func TestNoRangeUsesADefault(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := h.login(t)

	seedTraffic(t, h, view("site-1", "v1", "/a", time.Now().UTC().Add(-time.Hour)))

	body := decode[struct {
		Pageviews int64 `json:"pageviews"`
	}](t, h.get(t, "/api/stats/summary?site=site-1", token))

	if body.Pageviews != 1 {
		t.Errorf("pageviews = %d, want 1", body.Pageviews)
	}
}

func TestBadLimitIsRejected(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := h.login(t)

	for _, limit := range []string{"0", "-5", "1000000", "lots"} {
		resp := h.get(t, statsURL("pages", "site-1", "limit="+limit), token)
		resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("limit %q: status %d, want 400", limit, resp.StatusCode)
		}
	}
}

// Empty panels must encode as [] rather than null, so the dashboard can render
// an empty state without a null check in every panel.
func TestEmptyBreakdownsEncodeAsArrays(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := h.login(t)

	resp := h.get(t, statsURL("pages", "site-1"), token)
	defer resp.Body.Close()

	var raw map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}

	for _, key := range []string{"top", "entry", "exit"} {
		if string(raw[key]) != "[]" {
			t.Errorf("%s = %s, want []", key, raw[key])
		}
	}
}
