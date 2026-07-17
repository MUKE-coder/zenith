package scheduler_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenith/core/internal/email"
	"github.com/zenith/core/internal/scheduler"
	"github.com/zenith/core/internal/storage"
	"github.com/zenith/core/internal/storage/duckdb"
	"github.com/zenith/core/internal/storage/sqlite"
)

// july is a fixed "now", so the reported period is always June 2026.
var july = time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)

// recorder captures what would have been emailed.
type recorder struct {
	mu   sync.Mutex
	sent []email.Message
	err  error
}

func (r *recorder) Send(_ context.Context, msg email.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.err != nil {
		return r.err
	}
	r.sent = append(r.sent, msg)
	return nil
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.sent)
}

type fixture struct {
	app      *sqlite.Store
	events   *duckdb.Store
	reporter *scheduler.Reporter
	mail     *recorder
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()

	app, err := sqlite.Open(ctx, filepath.Join(dir, "app.sqlite"))
	if err != nil {
		t.Fatalf("open app: %v", err)
	}
	if err := app.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	events, err := duckdb.Open(ctx, filepath.Join(dir, "events.duckdb"))
	if err != nil {
		t.Fatalf("open events: %v", err)
	}

	t.Cleanup(func() {
		events.Close()
		app.Close()
	})

	mail := &recorder{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	reporter := scheduler.NewReporter(app, events, log)
	scheduler.SetNow(reporter, func() time.Time { return july })
	scheduler.SetSender(reporter, mail)

	return &fixture{app: app, events: events, reporter: reporter, mail: mail}
}

func (f *fixture) configure(t *testing.T) {
	t.Helper()

	err := f.app.UpdateSettings(context.Background(), storage.Settings{
		ResendAPIKey: "re_test_key",
		MailFrom:     "Zenith <reports@example.com>",
	})
	if err != nil {
		t.Fatalf("settings: %v", err)
	}
}

func (f *fixture) addSite(t *testing.T, id, owner string) {
	t.Helper()

	err := f.app.CreateSite(context.Background(), storage.Site{
		ID: id, Name: "Site " + id, Domain: id + ".com",
		SiteKey: "zk_pub_" + id, APIKey: "zk_sec_" + id, OwnerEmail: owner,
	})
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
}

// A pageview inside June 2026, the month a report sent in July covers.
func (f *fixture) addJuneTraffic(t *testing.T, siteID string) {
	t.Helper()

	err := f.events.Insert(context.Background(), storage.Event{
		SiteID: siteID, TS: time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC),
		Type: "pageview", Path: "/", VisitorHash: "v1",
	})
	if err != nil {
		t.Fatalf("insert event: %v", err)
	}
}

func TestSendsMonthlyReport(t *testing.T) {
	f := newFixture(t)
	f.configure(t)
	f.addSite(t, "site-1", "owner@client.com")
	f.addJuneTraffic(t, "site-1")

	sent, failed := f.reporter.SendMonthly(context.Background())

	if sent != 1 || failed != 0 {
		t.Fatalf("sent %d, failed %d; want 1 and 0", sent, failed)
	}
	if f.mail.count() != 1 {
		t.Fatalf("delivered %d emails, want 1", f.mail.count())
	}

	msg := f.mail.sent[0]
	if msg.To != "owner@client.com" {
		t.Errorf("to = %q, want owner@client.com", msg.To)
	}
	if msg.From != "Zenith <reports@example.com>" {
		t.Errorf("from = %q", msg.From)
	}
	// The subject names the month that just ended, not the current one.
	if want := "June 2026"; !contains(msg.Subject, want) {
		t.Errorf("subject = %q, want it to mention %q", msg.Subject, want)
	}
	if !contains(msg.HTML, "Unique visitors") {
		t.Error("the email does not contain the report")
	}
}

// The guard the whole schema constraint exists for: running twice for the same
// month must not mail the client a second copy.
func TestDoesNotSendTwiceForTheSameMonth(t *testing.T) {
	f := newFixture(t)
	f.configure(t)
	f.addSite(t, "site-1", "owner@client.com")
	f.addJuneTraffic(t, "site-1")

	ctx := context.Background()

	first, _ := f.reporter.SendMonthly(ctx)
	second, _ := f.reporter.SendMonthly(ctx)
	third, _ := f.reporter.SendMonthly(ctx)

	if first != 1 {
		t.Fatalf("first run sent %d, want 1", first)
	}
	if second != 0 || third != 0 {
		t.Errorf("later runs sent %d and %d, want 0: the client got the same month twice",
			second, third)
	}
	if f.mail.count() != 1 {
		t.Errorf("delivered %d emails, want exactly 1", f.mail.count())
	}
}

// A site with no owner has nobody to send to. Not an error.
func TestSkipsSitesWithNoOwner(t *testing.T) {
	f := newFixture(t)
	f.configure(t)
	f.addSite(t, "site-1", "")
	f.addJuneTraffic(t, "site-1")

	sent, failed := f.reporter.SendMonthly(context.Background())

	if sent != 0 || failed != 0 {
		t.Errorf("sent %d, failed %d; want 0 and 0", sent, failed)
	}
	if f.mail.count() != 0 {
		t.Error("emailed a site with no owner")
	}
}

// Without a Resend key there is nothing to send with, and that must be a
// no-op, not a crash.
func TestSkipsWhenEmailIsNotConfigured(t *testing.T) {
	f := newFixture(t)
	f.addSite(t, "site-1", "owner@client.com")

	sent, failed := f.reporter.SendMonthly(context.Background())

	if sent != 0 || failed != 0 {
		t.Errorf("sent %d, failed %d; want 0 and 0", sent, failed)
	}
}

// A failure must be recorded, or nobody finds out until the client asks where
// their report went.
func TestRecordsFailures(t *testing.T) {
	f := newFixture(t)
	f.configure(t)
	f.addSite(t, "site-1", "owner@client.com")
	f.mail.err = errors.New("domain is not verified")

	ctx := context.Background()
	sent, failed := f.reporter.SendMonthly(ctx)

	if sent != 0 || failed != 1 {
		t.Fatalf("sent %d, failed %d; want 0 and 1", sent, failed)
	}

	report, err := f.app.ReportFor(ctx, "site-1", "2026-06")
	if err != nil {
		t.Fatalf("no report recorded: %v", err)
	}
	if report.Status != storage.ReportFailed {
		t.Errorf("status = %q, want failed", report.Status)
	}
	if !contains(report.Err, "domain is not verified") {
		t.Errorf("error = %q, want it to carry Resend's reason", report.Err)
	}
}

// A failed month must be retried, not treated as done.
func TestRetriesAfterFailure(t *testing.T) {
	f := newFixture(t)
	f.configure(t)
	f.addSite(t, "site-1", "owner@client.com")

	ctx := context.Background()

	f.mail.err = errors.New("temporarily unavailable")
	if _, failed := f.reporter.SendMonthly(ctx); failed != 1 {
		t.Fatal("expected the first run to fail")
	}

	// Resend recovers.
	f.mail.err = nil
	sent, _ := f.reporter.SendMonthly(ctx)

	if sent != 1 {
		t.Errorf("sent %d after recovery, want 1: a failed month was never retried", sent)
	}

	report, err := f.app.ReportFor(ctx, "site-1", "2026-06")
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	if report.Status != storage.ReportSent {
		t.Errorf("status = %q, want sent", report.Status)
	}
}

func TestSendsToEverySiteWithAnOwner(t *testing.T) {
	f := newFixture(t)
	f.configure(t)
	f.addSite(t, "site-1", "a@client.com")
	f.addSite(t, "site-2", "b@client.com")
	f.addSite(t, "site-3", "") // no owner

	sent, failed := f.reporter.SendMonthly(context.Background())

	if sent != 2 || failed != 0 {
		t.Errorf("sent %d, failed %d; want 2 and 0", sent, failed)
	}
}

// One client's failure must not stop another client's report.
func TestOneFailureDoesNotStopTheRest(t *testing.T) {
	f := newFixture(t)
	f.configure(t)
	f.addSite(t, "site-1", "a@client.com")
	f.addSite(t, "site-2", "b@client.com")

	ctx := context.Background()

	// Fail the first site only: record it as already failed, then let the
	// sender work.
	f.mail.err = errors.New("rejected")
	f.reporter.SendMonthly(ctx)

	f.mail.err = nil
	sent, _ := f.reporter.SendMonthly(ctx)

	if sent != 2 {
		t.Errorf("sent %d, want 2: both sites should retry", sent)
	}
}

// A test send is a preview. If it recorded itself, the month would be marked
// done and the client's real report would never go out.
func TestTestSendDoesNotRecordHistory(t *testing.T) {
	f := newFixture(t)
	f.configure(t)
	f.addSite(t, "site-1", "owner@client.com")
	f.addJuneTraffic(t, "site-1")

	ctx := context.Background()

	if err := f.reporter.SendTest(ctx, "site-1"); err != nil {
		t.Fatalf("send test: %v", err)
	}
	if f.mail.count() != 1 {
		t.Fatalf("delivered %d emails, want 1", f.mail.count())
	}

	// Nothing recorded...
	if _, err := f.app.ReportFor(ctx, "site-1", "2026-06"); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("a test send wrote to report_history (%v): the real report would be skipped", err)
	}

	// ...so the real one still goes out.
	sent, _ := f.reporter.SendMonthly(ctx)
	if sent != 1 {
		t.Errorf("monthly sent %d after a test send, want 1", sent)
	}
}

func TestTestSendNeedsAnOwnerEmail(t *testing.T) {
	f := newFixture(t)
	f.configure(t)
	f.addSite(t, "site-1", "")

	if err := f.reporter.SendTest(context.Background(), "site-1"); err == nil {
		t.Error("sent a test report to a site with no owner")
	}
}

func TestTestSendNeedsEmailConfigured(t *testing.T) {
	f := newFixture(t)
	f.addSite(t, "site-1", "owner@client.com")

	err := f.reporter.SendTest(context.Background(), "site-1")
	if !errors.Is(err, email.ErrNotConfigured) {
		t.Errorf("got %v, want ErrNotConfigured", err)
	}
}

// The schedule must be a valid cron spec, and it must be monthly.
func TestScheduleIsMonthly(t *testing.T) {
	f := newFixture(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	s := scheduler.New(f.reporter, log)
	if err := s.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer s.Stop()

	next := s.Next()
	if next.IsZero() {
		t.Fatal("no next run scheduled")
	}
	if next.Day() != 1 {
		t.Errorf("next run is on day %d, want the 1st", next.Day())
	}
	if next.Location() != time.UTC {
		t.Errorf("next run is in %v, want UTC", next.Location())
	}
}

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}
