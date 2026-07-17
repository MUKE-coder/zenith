package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/zenith/core/internal/storage"
)

func TestCreateSite(t *testing.T) {
	h := newHarness(t)
	token := h.login(t)

	resp := h.post(t, "/api/sites",
		`{"name":"Client Site","domain":"client.com","owner_email":"owner@client.com"}`, token)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d, want 201", resp.StatusCode)
	}

	var site struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Domain     string `json:"domain"`
		SiteKey    string `json:"site_key"`
		APIKey     string `json:"api_key"`
		OwnerEmail string `json:"owner_email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&site); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if site.ID == "" {
		t.Error("no id")
	}
	if site.Name != "Client Site" || site.Domain != "client.com" {
		t.Errorf("site = %+v, want Client Site / client.com", site)
	}
	if site.OwnerEmail != "owner@client.com" {
		t.Errorf("owner_email = %q", site.OwnerEmail)
	}
	// Both keys, and they must not be the same value.
	if site.SiteKey == "" || site.APIKey == "" {
		t.Fatalf("missing a key: site_key=%q api_key=%q", site.SiteKey, site.APIKey)
	}
	if site.SiteKey == site.APIKey {
		t.Error("site_key and api_key are identical: the public key would read analytics")
	}
}

// The public and secret keys must be independently unguessable.
func TestCreatedSitesGetDistinctKeys(t *testing.T) {
	h := newHarness(t)
	token := h.login(t)

	seen := map[string]bool{}
	for i := range 5 {
		resp := h.post(t, "/api/sites",
			`{"name":"Site","domain":"site`+string(rune('a'+i))+`.com"}`, token)

		var site struct {
			SiteKey string `json:"site_key"`
			APIKey  string `json:"api_key"`
		}
		json.NewDecoder(resp.Body).Decode(&site)
		resp.Body.Close()

		for _, key := range []string{site.SiteKey, site.APIKey} {
			if seen[key] {
				t.Fatalf("key %q was issued twice", key)
			}
			seen[key] = true
		}
	}
}

// People paste URLs, www, and mixed case interchangeably. The stored domain
// has to be a bare host or the self-referral filter silently stops matching.
func TestCreateSiteNormalizesDomain(t *testing.T) {
	h := newHarness(t)
	token := h.login(t)

	cases := map[string]string{
		"https://example.com/":   "example.com",
		"http://www.example.com": "example.com",
		"WWW.Example.COM":        "example.com",
		"example.com/":           "example.com",
		"example.com:8080":       "example.com",
		"  example.com  ":        "example.com",
	}

	for input, want := range cases {
		body, err := json.Marshal(map[string]string{
			"name":   "Site",
			"domain": input,
		})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		resp := h.post(t, "/api/sites", string(body), token)

		var site struct {
			Domain string `json:"domain"`
		}
		json.NewDecoder(resp.Body).Decode(&site)
		resp.Body.Close()

		if site.Domain != want {
			t.Errorf("domain %q normalized to %q, want %q", input, site.Domain, want)
		}
	}
}

func TestCreateSiteRejectsBadInput(t *testing.T) {
	h := newHarness(t)
	token := h.login(t)

	cases := map[string]string{
		"no name":       `{"domain":"example.com"}`,
		"empty name":    `{"name":"","domain":"example.com"}`,
		"no domain":     `{"name":"Site"}`,
		"empty domain":  `{"name":"Site","domain":""}`,
		"not a domain":  `{"name":"Site","domain":"not a domain"}`,
		"no dot":        `{"name":"Site","domain":"localhost"}`,
		"bad email":     `{"name":"Site","domain":"example.com","owner_email":"nope"}`,
		"unknown field": `{"name":"Site","domain":"example.com","site_key":"zk_mine"}`,
	}

	for name, body := range cases {
		resp := h.post(t, "/api/sites", body, token)
		resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, resp.StatusCode)
		}
	}
}

// A client must not be able to choose their own site key.
func TestCreateSiteIgnoresClientSuppliedKeys(t *testing.T) {
	h := newHarness(t)
	token := h.login(t)

	resp := h.post(t, "/api/sites",
		`{"name":"Site","domain":"example.com","api_key":"zk_i_picked_this"}`, token)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status %d, want 400: a client set its own api_key", resp.StatusCode)
	}
}

func TestListSites(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := h.login(t)

	resp := h.get(t, "/api/sites", token)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}

	var body struct {
		Sites []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Domain string `json:"domain"`
		} `json:"sites"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(body.Sites) != 1 {
		t.Fatalf("got %d sites, want 1", len(body.Sites))
	}
	if body.Sites[0].ID != "site-1" || body.Sites[0].Domain != "example.com" {
		t.Errorf("site = %+v", body.Sites[0])
	}
}

// No sites yet is an empty list, not null: the dashboard renders an empty
// state, not a crash.
func TestListSitesEmpty(t *testing.T) {
	h := newHarness(t)
	token := h.login(t)

	resp := h.get(t, "/api/sites", token)
	defer resp.Body.Close()

	var raw map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(raw["sites"]) != "[]" {
		t.Errorf("sites = %s, want []", raw["sites"])
	}
}

// An owner has one site and no business enumerating anyone else's.
func TestOwnerCannotListSites(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := ownerToken(t, h, "site-1")

	resp := h.get(t, "/api/sites", token)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status %d, want 403: an owner listed every site", resp.StatusCode)
	}
}

func TestOwnerCannotCreateSites(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := ownerToken(t, h, "site-1")

	resp := h.post(t, "/api/sites", `{"name":"Mine","domain":"mine.com"}`, token)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status %d, want 403", resp.StatusCode)
	}
}

func TestSitesRequireAuth(t *testing.T) {
	h := newHarness(t)

	list := h.get(t, "/api/sites", "")
	list.Body.Close()
	if list.StatusCode != http.StatusUnauthorized {
		t.Errorf("list: status %d, want 401", list.StatusCode)
	}

	create := h.post(t, "/api/sites", `{"name":"Site","domain":"example.com"}`, "")
	create.Body.Close()
	if create.StatusCode != http.StatusUnauthorized {
		t.Errorf("create: status %d, want 401", create.StatusCode)
	}
}

// A site created through the API must immediately accept events on its key.
func TestCreatedSiteAcceptsEvents(t *testing.T) {
	h := newHarness(t)
	token := h.login(t)

	resp := h.post(t, "/api/sites", `{"name":"Live","domain":"live.com"}`, token)
	var site struct {
		SiteKey string `json:"site_key"`
	}
	json.NewDecoder(resp.Body).Decode(&site)
	resp.Body.Close()

	event := h.postUA(t, "/api/collect",
		`{"site_key":"`+site.SiteKey+`","url":"https://live.com/"}`, browserUA)
	defer event.Body.Close()

	if event.StatusCode != http.StatusNoContent {
		t.Errorf("collect: status %d, want 204", event.StatusCode)
	}
	if n := len(h.events.all()); n != 1 {
		t.Errorf("recorded %d events, want 1", n)
	}
}

func TestDeleteSite(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := h.login(t)

	// Give the site some traffic and an audit, so we can prove they go too.
	seedTraffic(t, h, view("site-1", "v1", "/a", statsBase))
	if err := h.app.CreateAuditJob(context.Background(), storage.AuditJob{
		ID: "job-1", SiteID: "site-1", Status: storage.AuditQueued,
	}); err != nil {
		t.Fatalf("create audit: %v", err)
	}

	resp := h.del(t, "/api/sites/site-1", token)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status %d, want 204", resp.StatusCode)
	}

	ctx := context.Background()

	// The site is gone.
	if _, err := h.app.SiteByID(ctx, "site-1"); err == nil {
		t.Error("the site still exists after delete")
	}

	// Its audit cascaded.
	if _, err := h.app.AuditJobByID(ctx, "job-1"); err == nil {
		t.Error("the audit job survived the site delete")
	}

	// Its events are gone from the event store: a deleted site's traffic must
	// not linger in aggregate queries.
	summary, err := h.events.EventStore.Summary(ctx, storage.Query{
		SiteID: "site-1", From: statsBase.Add(-time.Hour), To: statsBase.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.Pageviews != 0 {
		t.Errorf("deleted site still has %d pageviews", summary.Pageviews)
	}
}

func TestDeleteUnknownSite(t *testing.T) {
	h := newHarness(t)
	token := h.login(t)

	resp := h.del(t, "/api/sites/nope", token)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status %d, want 404", resp.StatusCode)
	}
}

func TestOwnerCannotDeleteSites(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := ownerToken(t, h, "site-1")

	resp := h.del(t, "/api/sites/site-1", token)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status %d, want 403", resp.StatusCode)
	}
}

// The api_key must never authorize writing events.
func TestApiKeyIsNotASiteKey(t *testing.T) {
	h := newHarness(t)
	token := h.login(t)

	resp := h.post(t, "/api/sites", `{"name":"Live","domain":"live.com"}`, token)
	var site struct {
		APIKey string `json:"api_key"`
	}
	json.NewDecoder(resp.Body).Decode(&site)
	resp.Body.Close()

	event := h.postUA(t, "/api/collect",
		`{"site_key":"`+site.APIKey+`","url":"https://live.com/"}`, browserUA)
	defer event.Body.Close()

	if event.StatusCode != http.StatusUnauthorized {
		t.Errorf("status %d, want 401: the secret api_key was accepted as a public site_key",
			event.StatusCode)
	}
}
