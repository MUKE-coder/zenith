package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/zenith/core/internal/storage"
)

// put sends a PUT with a session token.
func (h *harness) put(t *testing.T, path, body, token string) *http.Response {
	t.Helper()
	return h.do(t, http.MethodPut, path, body, token)
}

func (h *harness) patch(t *testing.T, path, body, token string) *http.Response {
	t.Helper()
	return h.do(t, http.MethodPatch, path, body, token)
}

func (h *harness) del(t *testing.T, path, token string) *http.Response {
	t.Helper()
	return h.do(t, http.MethodDelete, path, "", token)
}

func (h *harness) do(t *testing.T, method, path, body, token string) *http.Response {
	t.Helper()

	req, err := http.NewRequest(method, h.srv.URL+path, bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", contentJSON)
	if token != "" {
		req.Header.Set(authHeaderFn, "Bearer "+token)
	}

	resp, err := h.srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	return resp
}

func TestSettingsStartEmpty(t *testing.T) {
	h := newHarness(t)
	token := h.login(t)

	body := decode[struct {
		ResendConfigured bool   `json:"resend_configured"`
		MailFrom         string `json:"mail_from"`
		EmailReady       bool   `json:"email_ready"`
	}](t, h.get(t, "/api/settings", token))

	if body.ResendConfigured || body.EmailReady {
		t.Errorf("a fresh deployment reports email configured: %+v", body)
	}
}

func TestUpdateSettings(t *testing.T) {
	h := newHarness(t)
	token := h.login(t)

	resp := h.put(t, "/api/settings",
		`{"resend_api_key":"re_live_secret_key","mail_from":"Zenith <reports@example.com>"}`, token)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}

	body := decode[struct {
		ResendConfigured bool   `json:"resend_configured"`
		MailFrom         string `json:"mail_from"`
		EmailReady       bool   `json:"email_ready"`
	}](t, h.get(t, "/api/settings", token))

	if !body.ResendConfigured || !body.EmailReady {
		t.Errorf("settings did not stick: %+v", body)
	}
	if body.MailFrom != "Zenith <reports@example.com>" {
		t.Errorf("mail_from = %q", body.MailFrom)
	}
}

// The API key must never come back out. Not masked-but-present, not truncated:
// a secret that leaves the server can leak from a browser, a log, or a
// screenshot.
func TestSettingsNeverReturnTheApiKey(t *testing.T) {
	h := newHarness(t)
	token := h.login(t)

	const secret = "re_live_a_very_real_secret_key"

	write := h.put(t, "/api/settings",
		`{"resend_api_key":"`+secret+`","mail_from":"a@example.com"}`, token)
	writeBody, _ := io.ReadAll(write.Body)
	write.Body.Close()

	read := h.get(t, "/api/settings", token)
	readBody, _ := io.ReadAll(read.Body)
	read.Body.Close()

	for name, body := range map[string][]byte{"PUT": writeBody, "GET": readBody} {
		if bytes.Contains(body, []byte(secret)) {
			t.Errorf("%s response contains the Resend key: %s", name, body)
		}
		if bytes.Contains(body, []byte("re_live")) {
			t.Errorf("%s response contains part of the key: %s", name, body)
		}
	}
}

// The UI shows a mask, and echoing it back must not overwrite the real key
// with the mask -- otherwise changing MAIL FROM would destroy the API key.
func TestEchoingTheMaskKeepsTheKey(t *testing.T) {
	h := newHarness(t)
	token := h.login(t)
	ctx := context.Background()

	first := h.put(t, "/api/settings",
		`{"resend_api_key":"re_live_secret","mail_from":"a@example.com"}`, token)
	first.Body.Close()

	// The UI reads back the mask, edits only MAIL FROM, and sends both fields.
	second := h.put(t, "/api/settings",
		`{"resend_api_key":"••••••••","mail_from":"b@example.com"}`, token)
	second.Body.Close()

	settings, err := h.app.Settings(ctx)
	if err != nil {
		t.Fatalf("settings: %v", err)
	}
	if settings.ResendAPIKey != "re_live_secret" {
		t.Errorf("api key = %q; echoing the mask overwrote the real key", settings.ResendAPIKey)
	}
	if settings.MailFrom != "b@example.com" {
		t.Errorf("mail_from = %q, want b@example.com", settings.MailFrom)
	}
}

// Omitting a field must leave it alone.
func TestPartialSettingsUpdate(t *testing.T) {
	h := newHarness(t)
	token := h.login(t)

	h.put(t, "/api/settings",
		`{"resend_api_key":"re_live_secret","mail_from":"a@example.com"}`, token).Body.Close()

	h.put(t, "/api/settings", `{"mail_from":"changed@example.com"}`, token).Body.Close()

	settings, err := h.app.Settings(context.Background())
	if err != nil {
		t.Fatalf("settings: %v", err)
	}
	if settings.ResendAPIKey != "re_live_secret" {
		t.Errorf("api key = %q, want it untouched", settings.ResendAPIKey)
	}
	if settings.MailFrom != "changed@example.com" {
		t.Errorf("mail_from = %q", settings.MailFrom)
	}
}

// A typo'd MAIL FROM would only surface a month later, when the report was
// due. Catch it now.
func TestRejectsBadMailFrom(t *testing.T) {
	h := newHarness(t)
	token := h.login(t)

	for _, from := range []string{
		"not an email",
		"missing-at.example.com",
		"a@nodot",
		"Name <not an email>",
		"Name <a@b.c",
	} {
		body, err := json.Marshal(map[string]string{"mail_from": from})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		resp := h.put(t, "/api/settings", string(body), token)
		resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("mail_from %q: status %d, want 400", from, resp.StatusCode)
		}
	}
}

func TestAcceptsValidMailFrom(t *testing.T) {
	h := newHarness(t)
	token := h.login(t)

	for _, from := range []string{
		"reports@example.com",
		"Zenith <reports@example.com>",
		"Zenith Reports <no-reply@mail.example.co.uk>",
	} {
		body, err := json.Marshal(map[string]string{"mail_from": from})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		resp := h.put(t, "/api/settings", string(body), token)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("mail_from %q: status %d, want 200", from, resp.StatusCode)
		}
	}
}

// Settings are the deployment's, not a client's.
func TestOwnerCannotReadSettings(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := ownerToken(t, h, "site-1")

	read := h.get(t, "/api/settings", token)
	read.Body.Close()
	if read.StatusCode != http.StatusForbidden {
		t.Errorf("read: status %d, want 403", read.StatusCode)
	}

	write := h.put(t, "/api/settings", `{"mail_from":"a@example.com"}`, token)
	write.Body.Close()
	if write.StatusCode != http.StatusForbidden {
		t.Errorf("write: status %d, want 403", write.StatusCode)
	}
}

func TestSettingsRequireAuth(t *testing.T) {
	h := newHarness(t)

	resp := h.get(t, "/api/settings", "")
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status %d, want 401", resp.StatusCode)
	}
}

func TestUpdateSiteOwnerEmail(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := h.login(t)

	resp := h.patch(t, "/api/sites/site-1", `{"owner_email":"new@client.com"}`, token)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}

	site, err := h.app.SiteByID(context.Background(), "site-1")
	if err != nil {
		t.Fatalf("site: %v", err)
	}
	if site.OwnerEmail != "new@client.com" {
		t.Errorf("owner_email = %q", site.OwnerEmail)
	}
	// The untouched fields must survive a partial patch.
	if site.Name != "Test Site" || site.Domain != "example.com" {
		t.Errorf("patch clobbered other fields: %+v", site)
	}
}

// Clearing the owner email turns the monthly report off. It is a real choice.
func TestClearingOwnerEmail(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := h.login(t)

	h.patch(t, "/api/sites/site-1", `{"owner_email":""}`, token).Body.Close()

	site, err := h.app.SiteByID(context.Background(), "site-1")
	if err != nil {
		t.Fatalf("site: %v", err)
	}
	if site.OwnerEmail != "" {
		t.Errorf("owner_email = %q, want empty", site.OwnerEmail)
	}
}

// The keys are not editable through the edit form: rotating one breaks every
// installed snippet or every dashboard session.
func TestPatchCannotChangeKeys(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := h.login(t)

	resp := h.patch(t, "/api/sites/site-1", `{"site_key":"zk_i_picked_this"}`, token)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status %d, want 400: a client rotated a site key through an edit form",
			resp.StatusCode)
	}

	site, err := h.app.SiteByID(context.Background(), "site-1")
	if err != nil {
		t.Fatalf("site: %v", err)
	}
	if site.SiteKey != testSiteKey {
		t.Errorf("site_key = %q, want it unchanged", site.SiteKey)
	}
}

func TestPatchNormalizesDomain(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := h.login(t)

	h.patch(t, "/api/sites/site-1", `{"domain":"https://www.NewDomain.com/"}`, token).Body.Close()

	site, err := h.app.SiteByID(context.Background(), "site-1")
	if err != nil {
		t.Fatalf("site: %v", err)
	}
	if site.Domain != "newdomain.com" {
		t.Errorf("domain = %q, want newdomain.com", site.Domain)
	}
}

func TestPatchUnknownSite(t *testing.T) {
	h := newHarness(t)
	token := h.login(t)

	resp := h.patch(t, "/api/sites/nope", `{"name":"X"}`, token)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status %d, want 404", resp.StatusCode)
	}
}

func TestOwnerCannotEditSites(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := ownerToken(t, h, "site-1")

	resp := h.patch(t, "/api/sites/site-1", `{"name":"Renamed"}`, token)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status %d, want 403", resp.StatusCode)
	}
}

// A failed send must be visible, not something the developer hears about from
// their client.
func TestReportHistorySurfacesFailures(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := h.login(t)
	ctx := context.Background()

	err := h.app.RecordReport(ctx, storage.Report{
		ID: "r1", SiteID: "site-1", Period: "2026-06",
		Status: storage.ReportFailed, Err: "domain is not verified",
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}

	body := decode[struct {
		Reports []struct {
			Period string `json:"period"`
			Status string `json:"status"`
			Error  string `json:"error"`
		} `json:"reports"`
	}](t, h.get(t, "/api/sites/site-1/reports", token))

	if len(body.Reports) != 1 {
		t.Fatalf("got %d reports, want 1", len(body.Reports))
	}
	if body.Reports[0].Status != "failed" {
		t.Errorf("status = %q, want failed", body.Reports[0].Status)
	}
	if body.Reports[0].Error != "domain is not verified" {
		t.Errorf("error = %q, want Resend's reason", body.Reports[0].Error)
	}
}

// Without a Resend key, "send test" must say so rather than fail obscurely.
func TestTestReportWithoutEmailConfigured(t *testing.T) {
	h := newHarness(t)
	seedSite(t, h)
	token := h.login(t)

	resp := h.post(t, "/api/sites/site-1/reports/test", "", token)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", resp.StatusCode)
	}

	var body struct {
		Error string `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&body)

	if body.Error == "" {
		t.Fatal("no error message")
	}
	// It must say how to fix it.
	if !bytes.Contains([]byte(body.Error), []byte("settings")) {
		t.Errorf("error %q does not say where to configure email", body.Error)
	}
}
