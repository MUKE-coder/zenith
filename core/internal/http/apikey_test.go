package http_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	zhttp "github.com/zenith/core/internal/http"
	"github.com/zenith/core/internal/storage"
)

// apiKeyGet reads stats with a site's api key instead of a session.
func (h *harness) apiKeyGet(t *testing.T, path, apiKey string) *http.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, h.srv.URL+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set(zhttp.APIKeyHeader, apiKey)

	resp, err := h.srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	return resp
}

// The domain-native proxy has no session: it reads with the site's api key.
func TestApiKeyReadsStats(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)

	seedTraffic(t, h, view("site-1", "v1", "/a", statsBase))

	from := statsBase.Add(-time.Hour).Format(time.RFC3339)
	to := statsBase.Add(time.Hour).Format(time.RFC3339)

	resp := h.apiKeyGet(t, "/api/stats/summary?from="+from+"&to="+to, "zk_secret_api_key")
	body := decode[struct {
		Pageviews int64 `json:"pageviews"`
	}](t, resp)

	if body.Pageviews != 1 {
		t.Errorf("pageviews = %d, want 1", body.Pageviews)
	}
}

// The key names the site, so a proxy cannot ask for another client's.
func TestApiKeyCannotReadAnotherSite(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)

	if err := h.app.CreateSite(context.Background(), storage.Site{
		ID: "site-2", Name: "Someone Else", Domain: "other.com",
		SiteKey: "zk_pub_other", APIKey: "zk_sec_other",
	}); err != nil {
		t.Fatalf("create second site: %v", err)
	}
	seedTraffic(t, h, view("site-2", "v9", "/secret", statsBase))

	// site-1's key, asking for site-2.
	resp := h.apiKeyGet(t, "/api/stats/summary?site=site-2", "zk_secret_api_key")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status %d, want 403: an api key read another client's site", resp.StatusCode)
	}
}

func TestUnknownApiKeyIsRejected(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)

	resp := h.apiKeyGet(t, "/api/stats/summary", "zk_not_a_real_key")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status %d, want 401", resp.StatusCode)
	}
}

// The public site key is in every visitor's page source. It must never read.
func TestSiteKeyCannotReadStats(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)

	resp := h.apiKeyGet(t, "/api/stats/summary", testSiteKey)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status %d, want 401: the public site key read a client's analytics", resp.StatusCode)
	}
}

// An empty header falls through to session auth rather than matching a site
// whose api_key is NULL. (The column is nullable for databases created before
// api keys existed; SiteByAPIKey rejects the empty string outright.)
func TestEmptyApiKeyIsRejected(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)

	resp := h.apiKeyGet(t, "/api/stats/summary", "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status %d, want 401", resp.StatusCode)
	}
}

// An api key is not a developer: it must not enumerate every site.
func TestApiKeyCannotListSites(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)

	resp := h.apiKeyGet(t, "/api/sites", "zk_secret_api_key")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status %d, want 401: an api key listed every site", resp.StatusCode)
	}
}

// A session token must keep working: the api key is an addition, not a
// replacement.
func TestSessionStillReadsStats(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := h.login(t)

	seedTraffic(t, h, view("site-1", "v1", "/a", statsBase))

	body := decode[struct {
		Pageviews int64 `json:"pageviews"`
	}](t, h.get(t, statsURL("summary", "site-1"), token))

	if body.Pageviews != 1 {
		t.Errorf("pageviews = %d, want 1", body.Pageviews)
	}
}

// The client's own dashboard reads audits with the site's api key: it has no
// session, only the password gate on the owner's site and the key behind it.
// Without this its SEO tab could never show the audit the developer ran.
func TestAPIKeyReadsAudits(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)

	resp := h.apiKeyGet(t, "/api/audits", "zk_secret_api_key")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200 — the owner's SEO tab cannot load", resp.StatusCode)
	}
}

// Reading is one thing; spending the deployment's crawl budget is another. An
// api key resolves to owner claims, and running an audit is developer-only.
func TestAPIKeyCannotRunAnAudit(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)

	req, err := http.NewRequest(http.MethodPost, h.srv.URL+"/api/audits",
		strings.NewReader(`{"site_id":"site-1"}`))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set(zhttp.APIKeyHeader, "zk_secret_api_key")
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status %d, want 403: a client must not be able to trigger a crawl", resp.StatusCode)
	}
}
