package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zenith/core/internal/account"
	"github.com/zenith/core/internal/auth"
	zhttp "github.com/zenith/core/internal/http"
	"github.com/zenith/core/internal/scheduler"
	"github.com/zenith/core/internal/storage"
	"github.com/zenith/core/internal/storage/duckdb"
	"github.com/zenith/core/internal/storage/sqlite"
	"github.com/zenith/core/internal/visitor"
)

const (
	testSecret   = "a-test-signing-secret-long-enough-to-pass"
	adminEmail   = "dev@example.com"
	adminPass    = "correct horse battery staple"
	contentJSON  = "application/json"
	authHeaderFn = "Authorization"
)

type harness struct {
	srv    *httptest.Server
	app    *sqlite.Store
	events *recordingEvents

	// issuer lets tests mint an owner token. Owner tokens have no endpoint
	// yet: they are issued by the domain-native proxy in phase 7.
	issuer *auth.Issuer
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()

	dir := t.TempDir()

	app, err := sqlite.Open(ctx, filepath.Join(dir, "app.sqlite"))
	if err != nil {
		t.Fatalf("open app store: %v", err)
	}
	if err := app.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	eventStore, err := duckdb.Open(ctx, filepath.Join(dir, "events.duckdb"))
	if err != nil {
		t.Fatalf("open event store: %v", err)
	}

	if _, err := account.Provision(ctx, app, adminEmail, adminPass); err != nil {
		t.Fatalf("provision: %v", err)
	}

	issuer, err := auth.NewIssuer(testSecret, time.Hour)
	if err != nil {
		t.Fatalf("new issuer: %v", err)
	}

	visitors, err := visitor.NewHasher()
	if err != nil {
		t.Fatalf("new hasher: %v", err)
	}

	events := &recordingEvents{EventStore: eventStore}

	// Discard logs: these tests assert on responses, not output.
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	// A real Reporter with no sender override, so it fails the way an
	// unconfigured deployment does rather than mailing anyone.
	reporter := scheduler.NewReporter(app, events, log)

	srv := httptest.NewServer(zhttp.New(zhttp.Deps{
		Events:   events,
		App:      app,
		Issuer:   issuer,
		Visitors: visitors,
		Log:      log,
		Reporter: reporter,
	}).Routes())

	t.Cleanup(func() {
		srv.Close()
		eventStore.Close()
		app.Close()
	})
	return &harness{srv: srv, app: app, events: events, issuer: issuer}
}

// postUA posts with a specific user agent, which is what device, browser, OS,
// and the visitor hash are all derived from.
func (h *harness) postUA(t *testing.T, path, body, userAgent string) *http.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, h.srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", contentJSON)
	req.Header.Set("User-Agent", userAgent)

	resp, err := h.srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	return resp
}

func (h *harness) post(t *testing.T, path, body, token string) *http.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, h.srv.URL+path, strings.NewReader(body))
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

func (h *harness) get(t *testing.T, path, token string) *http.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, h.srv.URL+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if token != "" {
		req.Header.Set(authHeaderFn, "Bearer "+token)
	}

	resp, err := h.srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	return resp
}

// login returns a valid developer token.
func (h *harness) login(t *testing.T) string {
	t.Helper()

	resp := h.post(t, "/api/auth/login",
		`{"email":"`+adminEmail+`","password":"`+adminPass+`"}`, "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login: status %d, want 200", resp.StatusCode)
	}

	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	if body.Token == "" {
		t.Fatal("login returned an empty token")
	}
	return body.Token
}

func TestLoginSucceeds(t *testing.T) {
	h := newHarness(t)

	resp := h.post(t, "/api/auth/login",
		`{"email":"`+adminEmail+`","password":"`+adminPass+`"}`, "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}

	var body struct {
		Token     string    `json:"token"`
		Role      string    `json:"role"`
		Email     string    `json:"email"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if body.Token == "" {
		t.Error("no token in response")
	}
	if body.Role != "developer" {
		t.Errorf("role = %q, want developer", body.Role)
	}
	if body.ExpiresAt.Before(time.Now()) {
		t.Errorf("expires_at %v is already past", body.ExpiresAt)
	}
}

// The response must never carry the password hash back to the client.
func TestLoginResponseLeaksNoHash(t *testing.T) {
	h := newHarness(t)

	resp := h.post(t, "/api/auth/login",
		`{"email":"`+adminEmail+`","password":"`+adminPass+`"}`, "")
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if bytes.Contains(raw, []byte("$2a$")) || bytes.Contains(raw, []byte("password_hash")) {
		t.Errorf("login response contains a password hash: %s", raw)
	}
	if bytes.Contains(raw, []byte(adminPass)) {
		t.Error("login response contains the plaintext password")
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	h := newHarness(t)

	resp := h.post(t, "/api/auth/login",
		`{"email":"`+adminEmail+`","password":"wrong password entirely"}`, "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status %d, want 401", resp.StatusCode)
	}
}

// The error for an unknown email must be identical to the one for a bad
// password, or it becomes an account-enumeration oracle.
func TestLoginDoesNotRevealWhetherAccountExists(t *testing.T) {
	h := newHarness(t)

	unknown := h.post(t, "/api/auth/login",
		`{"email":"nobody@example.com","password":"correct horse battery staple"}`, "")
	defer unknown.Body.Close()
	unknownBody, _ := io.ReadAll(unknown.Body)

	wrong := h.post(t, "/api/auth/login",
		`{"email":"`+adminEmail+`","password":"wrong password entirely"}`, "")
	defer wrong.Body.Close()
	wrongBody, _ := io.ReadAll(wrong.Body)

	if unknown.StatusCode != wrong.StatusCode {
		t.Errorf("status differs: unknown account %d, wrong password %d",
			unknown.StatusCode, wrong.StatusCode)
	}
	if !bytes.Equal(unknownBody, wrongBody) {
		t.Errorf("response body differs:\n unknown account: %s\n wrong password:  %s",
			unknownBody, wrongBody)
	}
}

func TestLoginRejectsMalformedJSON(t *testing.T) {
	h := newHarness(t)

	resp := h.post(t, "/api/auth/login", `{"email":`, "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status %d, want 400", resp.StatusCode)
	}
}

func TestLoginRejectsEmptyCredentials(t *testing.T) {
	h := newHarness(t)

	for _, body := range []string{
		`{"email":"","password":""}`,
		`{"email":"` + adminEmail + `","password":""}`,
		`{}`,
	} {
		resp := h.post(t, "/api/auth/login", body, "")
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("body %s: status %d, want 400", body, resp.StatusCode)
		}
	}
}

// An unknown field is a typo or a probe, not a silent default.
func TestLoginRejectsUnknownFields(t *testing.T) {
	h := newHarness(t)

	resp := h.post(t, "/api/auth/login",
		`{"email":"`+adminEmail+`","password":"`+adminPass+`","role":"developer"}`, "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status %d, want 400: a client must not be able to ask for a role", resp.StatusCode)
	}
}

// An unauthenticated endpoint must not let a caller allocate without bound.
func TestLoginRejectsHugeBody(t *testing.T) {
	h := newHarness(t)

	huge := `{"email":"` + strings.Repeat("a", 100_000) + `","password":"x"}`
	resp := h.post(t, "/api/auth/login", huge, "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status %d, want 400", resp.StatusCode)
	}
}

func TestProtectedRouteRejectsMissingToken(t *testing.T) {
	h := newHarness(t)

	resp := h.get(t, "/api/auth/me", "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status %d, want 401", resp.StatusCode)
	}
}

func TestProtectedRouteRejectsGarbageToken(t *testing.T) {
	h := newHarness(t)

	resp := h.get(t, "/api/auth/me", "not-a-real-token")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status %d, want 401", resp.StatusCode)
	}
}

// A token signed with the wrong secret must not open anything.
func TestProtectedRouteRejectsForeignToken(t *testing.T) {
	h := newHarness(t)

	other, err := auth.NewIssuer("a-completely-different-secret-of-length", time.Hour)
	if err != nil {
		t.Fatalf("new issuer: %v", err)
	}
	forged, _, err := other.IssueDeveloper("attacker")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	resp := h.get(t, "/api/auth/me", forged)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status %d, want 401: a token signed with another secret was accepted", resp.StatusCode)
	}
}

func TestProtectedRouteAcceptsValidToken(t *testing.T) {
	h := newHarness(t)
	token := h.login(t)

	resp := h.get(t, "/api/auth/me", token)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}

	var body struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Role != "developer" {
		t.Errorf("role = %q, want developer", body.Role)
	}
}

// The point of the whole revocation table: after logout, the token is dead
// even though its signature is still valid and it has not expired.
func TestLogoutInvalidatesTheToken(t *testing.T) {
	h := newHarness(t)
	token := h.login(t)

	// Works before.
	before := h.get(t, "/api/auth/me", token)
	before.Body.Close()
	if before.StatusCode != http.StatusOK {
		t.Fatalf("before logout: status %d, want 200", before.StatusCode)
	}

	out := h.post(t, "/api/auth/logout", "", token)
	out.Body.Close()
	if out.StatusCode != http.StatusOK {
		t.Fatalf("logout: status %d, want 200", out.StatusCode)
	}

	// Dead after, despite an intact signature and an unexpired exp.
	after := h.get(t, "/api/auth/me", token)
	after.Body.Close()
	if after.StatusCode != http.StatusUnauthorized {
		t.Errorf("after logout: status %d, want 401 -- the token still works", after.StatusCode)
	}
}

// Two tabs, or a retried request, must not turn logout into an error.
func TestLogoutTwiceIsNotAnError(t *testing.T) {
	h := newHarness(t)
	token := h.login(t)

	first := h.post(t, "/api/auth/logout", "", token)
	first.Body.Close()
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first logout: status %d, want 200", first.StatusCode)
	}

	// The second attempt is rejected because the token is already revoked --
	// a 401, not a 500.
	second := h.post(t, "/api/auth/logout", "", token)
	second.Body.Close()
	if second.StatusCode != http.StatusUnauthorized {
		t.Errorf("second logout: status %d, want 401", second.StatusCode)
	}
}

// Logging out of one session must not sign out the others.
func TestLogoutOnlyRevokesItsOwnToken(t *testing.T) {
	h := newHarness(t)

	first := h.login(t)
	second := h.login(t)

	out := h.post(t, "/api/auth/logout", "", first)
	out.Body.Close()

	resp := h.get(t, "/api/auth/me", second)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("second session: status %d, want 200 -- logging out one session killed another",
			resp.StatusCode)
	}
}

func TestLogoutRequiresAuth(t *testing.T) {
	h := newHarness(t)

	resp := h.post(t, "/api/auth/logout", "", "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status %d, want 401", resp.StatusCode)
	}
}

// Owners authenticate at their own site's password gate, never at the console
// login, even if a row somehow exists for them.
func TestOwnerCannotUseDeveloperLogin(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	hash, err := auth.HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := h.app.CreateUser(ctx, storage.User{
		ID: "owner-1", Email: "owner@example.com",
		PasswordHash: hash, Role: storage.RoleOwner,
	}); err != nil {
		t.Fatalf("create owner: %v", err)
	}

	resp := h.post(t, "/api/auth/login",
		`{"email":"owner@example.com","password":"correct horse battery staple"}`, "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status %d, want 401: an owner logged in at the console", resp.StatusCode)
	}
}

// Only the Bearer scheme is accepted, and only with a token.
func TestMalformedAuthorizationHeaders(t *testing.T) {
	h := newHarness(t)
	token := h.login(t)

	for _, header := range []string{
		token,              // no scheme
		"Basic " + token,   // wrong scheme
		"Bearer",           // no token
		"Bearer ",          // empty token
		"Bearer  " + token, // token is " <token>" after the cut, still trimmed -> valid
	} {
		req, err := http.NewRequest(http.MethodGet, h.srv.URL+"/api/auth/me", nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set(authHeaderFn, header)

		resp, err := h.srv.Client().Do(req)
		if err != nil {
			t.Fatalf("do: %v", err)
		}
		resp.Body.Close()

		// The double-space case is legitimately valid after trimming.
		want := http.StatusUnauthorized
		if header == "Bearer  "+token {
			want = http.StatusOK
		}
		if resp.StatusCode != want {
			t.Errorf("header %q: status %d, want %d", header, resp.StatusCode, want)
		}
	}
}

// The health check must stay open: it is what the container probes.
func TestHealthNeedsNoAuth(t *testing.T) {
	h := newHarness(t)

	resp := h.get(t, "/health", "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status %d, want 200", resp.StatusCode)
	}
}
