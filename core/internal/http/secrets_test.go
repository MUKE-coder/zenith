package http_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
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

// The secrets a running deployment holds. If any of these ever reaches a log
// line, it is in the operator's log aggregator, their terminal scrollback, and
// whatever screenshot they paste into an issue.
const (
	secretPassword = "correct horse battery staple"
	secretJWT      = "a-test-signing-secret-long-enough-to-pass"
	secretResend   = "re_live_this_must_never_be_logged"
	secretAPIKey   = "zk_sec_this_must_never_be_logged"
)

// safeBuffer is a concurrency-safe log sink.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// Drive every path that touches a secret, then read the log back.
//
// This is the test the "never log secrets" rule actually needs: a code review
// catches the obvious `log.Info("key", key)`, but not a wrapped error that
// carries a request, or a struct logged with %+v three call frames away.
func TestNoSecretReachesTheLog(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	app, err := sqlite.Open(ctx, filepath.Join(dir, "app.sqlite"))
	if err != nil {
		t.Fatalf("open app: %v", err)
	}
	defer app.Close()
	if err := app.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	events, err := duckdb.Open(ctx, filepath.Join(dir, "events.duckdb"))
	if err != nil {
		t.Fatalf("open events: %v", err)
	}
	defer events.Close()

	if _, err := account.Provision(ctx, app, adminEmail, secretPassword); err != nil {
		t.Fatalf("provision: %v", err)
	}

	err = app.CreateSite(ctx, storage.Site{
		ID: "site-1", Name: "Client", Domain: "client.com",
		SiteKey: "zk_pub_public", APIKey: secretAPIKey, OwnerEmail: "owner@client.com",
	})
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	issuer, err := auth.NewIssuer(secretJWT, time.Hour)
	if err != nil {
		t.Fatalf("issuer: %v", err)
	}
	visitors, err := visitor.NewHasher()
	if err != nil {
		t.Fatalf("hasher: %v", err)
	}

	// Debug level: whatever the noisiest deployment would print.
	sink := &safeBuffer{}
	log := slog.New(slog.NewTextHandler(sink, &slog.HandlerOptions{Level: slog.LevelDebug}))

	srv := httptest.NewServer(zhttp.New(zhttp.Deps{
		Events:   events,
		App:      app,
		Issuer:   issuer,
		Visitors: visitors,
		Log:      log,
		Reporter: scheduler.NewReporter(app, events, log),
	}).Routes())
	defer srv.Close()

	h := &harness{srv: srv, app: app, issuer: issuer}

	// Every flow that handles a secret.
	token := h.login(t)          // right password
	h.post(t, "/api/auth/login", // wrong password
		`{"email":"`+adminEmail+`","password":"`+secretPassword+`X"}`, "").Body.Close()
	h.post(t, "/api/auth/login", // unknown account
		`{"email":"nobody@example.com","password":"`+secretPassword+`"}`, "").Body.Close()
	h.put(t, "/api/settings", // store the Resend key
		`{"resend_api_key":"`+secretResend+`","mail_from":"a@example.com"}`, token).Body.Close()
	h.get(t, "/api/settings", token).Body.Close()                       // read it back
	h.get(t, "/api/sites", token).Body.Close()                          // lists api keys
	h.apiKeyGet(t, "/api/stats/summary", secretAPIKey).Body.Close()     // authenticate with one
	h.apiKeyGet(t, "/api/stats/summary", "zk_sec_wrong").Body.Close()   // and fail with one
	h.post(t, "/api/sites/site-1/reports/test", "", token).Body.Close() // try to send email
	h.post(t, "/api/auth/logout", "", token).Body.Close()

	logged := sink.String()
	if logged == "" {
		t.Fatal("nothing was logged, so this proves nothing")
	}

	for name, secret := range map[string]string{
		"the account password": secretPassword,
		"the signing secret":   secretJWT,
		"the Resend API key":   secretResend,
		"the site's api key":   secretAPIKey,
		"a session token":      token,
	} {
		if strings.Contains(logged, secret) {
			t.Errorf("%s reached the log", name)
		}
	}
}

// A password hash is not as bad as a password, but it is still the thing an
// offline attack runs against.
func TestNoPasswordHashReachesTheLog(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	app, err := sqlite.Open(ctx, filepath.Join(dir, "app.sqlite"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer app.Close()
	if err := app.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	sink := &safeBuffer{}
	log := slog.New(slog.NewTextHandler(sink, &slog.HandlerOptions{Level: slog.LevelDebug}))

	if _, err := account.Provision(ctx, app, adminEmail, secretPassword); err != nil {
		t.Fatalf("provision: %v", err)
	}

	user, err := app.UserByEmail(ctx, adminEmail)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}

	events, err := duckdb.Open(ctx, filepath.Join(dir, "events.duckdb"))
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	defer events.Close()

	issuer, _ := auth.NewIssuer(secretJWT, time.Hour)
	visitors, _ := visitor.NewHasher()

	srv := httptest.NewServer(zhttp.New(zhttp.Deps{
		Events: events, App: app, Issuer: issuer, Visitors: visitors, Log: log,
	}).Routes())
	defer srv.Close()

	h := &harness{srv: srv, app: app, issuer: issuer}
	h.login(t)
	h.post(t, "/api/auth/login", `{"email":"`+adminEmail+`","password":"wrong"}`, "").Body.Close()

	if strings.Contains(sink.String(), user.PasswordHash) {
		t.Error("a bcrypt hash reached the log")
	}
}
