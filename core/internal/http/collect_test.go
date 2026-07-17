package http_test

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/zenith/core/internal/storage"
)

// recordingEvents is a real DuckDB store that also remembers what was written.
//
// Wrapping the real store rather than faking it means the stats tests exercise
// the actual SQL through HTTP, while the ingestion tests can still inspect the
// exact event a request produced.
type recordingEvents struct {
	storage.EventStore

	mu     sync.Mutex
	events []storage.Event
}

func (e *recordingEvents) Insert(ctx context.Context, events ...storage.Event) error {
	e.mu.Lock()
	e.events = append(e.events, events...)
	e.mu.Unlock()

	return e.EventStore.Insert(ctx, events...)
}

func (e *recordingEvents) all() []storage.Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]storage.Event(nil), e.events...)
}

func (e *recordingEvents) one(t *testing.T) storage.Event {
	t.Helper()
	all := e.all()
	if len(all) != 1 {
		t.Fatalf("recorded %d events, want exactly 1", len(all))
	}
	return all[0]
}

const testSiteKey = "zk_test_public_key"

// browserUA is a real browser's user agent.
//
// Tests that expect an event to land must send one. Go's HTTP client otherwise
// identifies itself as "Go-http-client/1.1", which ingestion correctly
// classifies as automation and drops.
const browserUA = "Mozilla/5.0 (X11; Linux x86_64; rv:128.0) Gecko/20100101 Firefox/128.0"

// seedSite inserts a site so collect has a key to resolve.
func seedSite(t *testing.T, h *harness) {
	t.Helper()

	if err := h.app.CreateSite(context.Background(), storage.Site{
		ID:      "site-1",
		Name:    "Test Site",
		Domain:  "example.com",
		SiteKey: testSiteKey,
		APIKey:  "zk_secret_api_key",
	}); err != nil {
		t.Fatalf("seed site: %v", err)
	}
}

func collectBody(siteKey, url, name string) string {
	return `{"site_key":"` + siteKey + `","url":"` + url + `","name":"` + name + `"}`
}

// A real pageview must land, with the URL parsed server-side.
func TestCollectPageview(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)

	resp := h.postUA(t, "/api/collect",
		`{"site_key":"`+testSiteKey+`","url":"https://example.com/pricing","referrer":"https://news.ycombinator.com/item?id=1"}`,
		"Mozilla/5.0 (X11; Linux x86_64) Gecko/20100101 Firefox/128.0")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status %d, want 204", resp.StatusCode)
	}

	event := h.events.one(t)
	if event.SiteID != "site-1" {
		t.Errorf("site_id = %q, want site-1", event.SiteID)
	}
	if event.Type != "pageview" {
		t.Errorf("type = %q, want pageview", event.Type)
	}
	if event.Path != "/pricing" {
		t.Errorf("path = %q, want /pricing", event.Path)
	}
	if event.EventName != "" {
		t.Errorf("event_name = %q, want empty for a pageview", event.EventName)
	}
	if event.VisitorHash == "" {
		t.Error("no visitor hash")
	}
	if event.Browser != "Firefox" {
		t.Errorf("browser = %q, want Firefox", event.Browser)
	}
	if event.OS != "Linux" {
		t.Errorf("os = %q, want Linux", event.OS)
	}
	if event.Device != "desktop" {
		t.Errorf("device = %q, want desktop", event.Device)
	}
}

// Only the referrer's hostname is kept: full referring URLs leak private paths.
func TestCollectStoresOnlyReferrerHost(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)

	resp := h.postUA(t, "/api/collect",
		`{"site_key":"`+testSiteKey+`","url":"https://example.com/","referrer":"https://www.google.com/search?q=private+search+phrase"}`,
		browserUA)
	resp.Body.Close()

	event := h.events.one(t)
	if event.Referrer != "google.com" {
		t.Errorf("referrer = %q, want google.com (host only, www stripped)", event.Referrer)
	}
	if strings.Contains(event.Referrer, "private+search+phrase") {
		t.Error("the referring query string was stored: that can leak private context")
	}
}

// UTM parameters are derived from the URL server-side.
func TestCollectExtractsUTM(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)

	resp := h.postUA(t, "/api/collect",
		`{"site_key":"`+testSiteKey+`","url":"https://example.com/lp?utm_source=hn&utm_medium=social&utm_campaign=launch&utm_term=analytics&utm_content=post"}`,
		browserUA)
	resp.Body.Close()

	event := h.events.one(t)
	if event.UTMSource != "hn" || event.UTMMedium != "social" || event.UTMCampaign != "launch" {
		t.Errorf("utm source/medium/campaign = %q/%q/%q, want hn/social/launch",
			event.UTMSource, event.UTMMedium, event.UTMCampaign)
	}
	if event.UTMTerm != "analytics" || event.UTMContent != "post" {
		t.Errorf("utm term/content = %q/%q, want analytics/post", event.UTMTerm, event.UTMContent)
	}
	// The path must not carry the query string.
	if event.Path != "/lp" {
		t.Errorf("path = %q, want /lp", event.Path)
	}
}

// A custom event lands as type=event with its name and properties.
func TestCollectCustomEvent(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)

	resp := h.postUA(t, "/api/collect",
		`{"site_key":"`+testSiteKey+`","url":"https://example.com/signup","name":"signup","props":{"plan":"pro","seats":3,"trial":true}}`,
		browserUA)
	resp.Body.Close()

	event := h.events.one(t)
	if event.Type != "event" {
		t.Errorf("type = %q, want event", event.Type)
	}
	if event.EventName != "signup" {
		t.Errorf("event_name = %q, want signup", event.EventName)
	}
	if event.Props["plan"] != "pro" {
		t.Errorf("props[plan] = %v, want pro", event.Props["plan"])
	}
	if event.Props["trial"] != true {
		t.Errorf("props[trial] = %v, want true", event.Props["trial"])
	}
}

// The whole point of the two-key split: an unknown key writes nothing.
func TestCollectRejectsUnknownSiteKey(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)

	resp := h.post(t, "/api/collect",
		collectBody("zk_not_a_real_key", "https://example.com/", "pageview"), "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status %d, want 401", resp.StatusCode)
	}
	if n := len(h.events.all()); n != 0 {
		t.Errorf("recorded %d events for an unknown key, want 0", n)
	}
}

func TestCollectRejectsMissingSiteKey(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)

	resp := h.post(t, "/api/collect", `{"url":"https://example.com/"}`, "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status %d, want 400", resp.StatusCode)
	}
}

// Only real page URLs. javascript: and data: are not pages anyone visited.
func TestCollectRejectsBadURLs(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)

	for _, url := range []string{
		"",
		"not-a-url",
		"javascript:alert(1)",
		"data:text/html,<h1>hi",
		"file:///etc/passwd",
		"ftp://example.com/x",
	} {
		body := `{"site_key":"` + testSiteKey + `","url":"` + url + `"}`
		resp := h.post(t, "/api/collect", body, "")
		resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("url %q: status %d, want 400", url, resp.StatusCode)
		}
	}
	if n := len(h.events.all()); n != 0 {
		t.Errorf("recorded %d events from bad URLs, want 0", n)
	}
}

// Bots are accepted but never counted: crawler traffic would inflate every
// number on the dashboard.
func TestCollectIgnoresBots(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)

	resp := h.postUA(t, "/api/collect",
		collectBody(testSiteKey, "https://example.com/", "pageview"),
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)")
	defer resp.Body.Close()

	// The crawler is told everything is fine...
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status %d, want 204", resp.StatusCode)
	}
	// ...but nothing was recorded.
	if n := len(h.events.all()); n != 0 {
		t.Errorf("recorded %d events from a bot, want 0", n)
	}
}

// The same visitor hitting two pages must produce one identity.
func TestCollectVisitorHashIsStableAcrossPages(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)

	for _, path := range []string{"/", "/pricing"} {
		resp := h.postUA(t, "/api/collect",
			collectBody(testSiteKey, "https://example.com"+path, "pageview"),
			"Mozilla/5.0 (X11; Linux x86_64) Firefox/128.0")
		resp.Body.Close()
	}

	all := h.events.all()
	if len(all) != 2 {
		t.Fatalf("recorded %d events, want 2", len(all))
	}
	if all[0].VisitorHash != all[1].VisitorHash {
		t.Error("the same visitor got two identities across two pages: " +
			"unique visitors would be over-counted")
	}
}

// Different user agents are different visitors.
func TestCollectDifferentAgentsAreDifferentVisitors(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)

	agents := []string{
		"Mozilla/5.0 (X11; Linux x86_64) Firefox/128.0",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Chrome/120.0.0.0 Safari/537.36",
	}
	for _, agent := range agents {
		resp := h.postUA(t, "/api/collect",
			collectBody(testSiteKey, "https://example.com/", "pageview"), agent)
		resp.Body.Close()
	}

	all := h.events.all()
	if len(all) != 2 {
		t.Fatalf("recorded %d events, want 2", len(all))
	}
	if all[0].VisitorHash == all[1].VisitorHash {
		t.Error("two different browsers hashed to one visitor")
	}
}

// No raw address or user agent may reach the stored event.
func TestCollectStoresNoRawIdentifiers(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)

	const agent = "Mozilla/5.0 (X11; Linux x86_64) Firefox/128.0"
	resp := h.postUA(t, "/api/collect",
		collectBody(testSiteKey, "https://example.com/", "pageview"), agent)
	resp.Body.Close()

	event := h.events.one(t)

	// The event struct has no IP field at all; assert the agent string did not
	// smuggle itself into a text column either.
	for name, value := range map[string]string{
		"path": event.Path, "referrer": event.Referrer,
		"browser": event.Browser, "os": event.OS,
		"visitor_hash": event.VisitorHash, "event_name": event.EventName,
	} {
		if strings.Contains(value, agent) {
			t.Errorf("%s contains the raw user agent: %q", name, value)
		}
		if strings.Contains(value, "127.0.0.1") || strings.Contains(value, "[::1]") {
			t.Errorf("%s contains the raw client address: %q", name, value)
		}
	}
}

func TestCollectRejectsOversizedBody(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)

	huge := `{"site_key":"` + testSiteKey + `","url":"https://example.com/` +
		strings.Repeat("a", 20_000) + `"}`
	resp := h.post(t, "/api/collect", huge, "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status %d, want 400", resp.StatusCode)
	}
}

// Nested props would make the stats API's property breakdown meaningless.
func TestCollectRejectsNestedProps(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)

	resp := h.post(t, "/api/collect",
		`{"site_key":"`+testSiteKey+`","url":"https://example.com/","name":"signup","props":{"nested":{"a":1}}}`, "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status %d, want 400", resp.StatusCode)
	}
}

func TestCollectRejectsTooManyProps(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)

	var b strings.Builder
	b.WriteString(`{"site_key":"` + testSiteKey + `","url":"https://example.com/","name":"e","props":{`)
	for i := range 20 {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`"k` + string(rune('a'+i)) + `":"v"`)
	}
	b.WriteString(`}}`)

	resp := h.post(t, "/api/collect", b.String(), "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status %d, want 400", resp.StatusCode)
	}
}

// Ingestion is public and must not accept unknown fields silently.
func TestCollectRejectsUnknownFields(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)

	resp := h.post(t, "/api/collect",
		`{"site_key":"`+testSiteKey+`","url":"https://example.com/","country":"US"}`, "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status %d, want 400: a client must not be able to set its own country", resp.StatusCode)
	}
}

// The snippet sends JSON under a text/plain content type, because text/plain
// is CORS-safelisted and so needs no preflight -- and a preflight is exactly
// what sendBeacon cannot survive against a wildcard origin. Rejecting the body
// on content type would silently break every real page.
func TestCollectAcceptsTextPlainBody(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)

	req, err := http.NewRequest(http.MethodPost, h.srv.URL+"/api/collect",
		strings.NewReader(collectBody(testSiteKey, "https://example.com/", "pageview")))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "text/plain;charset=UTF-8")
	req.Header.Set("User-Agent", browserUA)

	resp, err := h.srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status %d, want 204", resp.StatusCode)
	}
	if n := len(h.events.all()); n != 1 {
		t.Errorf("recorded %d events, want 1", n)
	}
}

// The snippet runs on domains Zenith cannot know in advance, so the browser
// must be allowed to post from any origin.
func TestCollectAllowsCrossOrigin(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)

	resp := h.post(t, "/api/collect",
		collectBody(testSiteKey, "https://example.com/", "pageview"), "")
	defer resp.Body.Close()

	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want *: the browser would block every event", got)
	}
}

// The preflight must succeed, or the POST never happens.
func TestCollectPreflight(t *testing.T) {
	h := newHarness(t)

	req, err := http.NewRequest(http.MethodOptions, h.srv.URL+"/api/collect", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")

	resp, err := h.srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status %d, want 204", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Methods"); !strings.Contains(got, "POST") {
		t.Errorf("Access-Control-Allow-Methods = %q, want it to include POST", got)
	}
}

// Ingestion must never require a session: it is called from a stranger's page.
func TestCollectNeedsNoAuth(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)

	resp := h.postUA(t, "/api/collect",
		collectBody(testSiteKey, "https://example.com/", "pageview"), browserUA)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status %d, want 204", resp.StatusCode)
	}
	// Assert the event actually landed. A bot is also answered 204, so status
	// alone would pass even if nothing was recorded.
	if n := len(h.events.all()); n != 1 {
		t.Errorf("recorded %d events, want 1", n)
	}
}
