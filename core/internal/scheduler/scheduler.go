// Package scheduler runs the monthly report.
package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/zenith/core/internal/email"
	"github.com/zenith/core/internal/id"
	"github.com/zenith/core/internal/report"
	"github.com/zenith/core/internal/storage"
)

// MonthlySchedule is when reports go out: 09:00 UTC on the 1st of each month.
//
// The 1st because the month being reported has only just finished. 09:00
// rather than midnight so a send lands in a working-hours inbox somewhere, and
// so a deploy at midnight is not competing with it.
const MonthlySchedule = "0 9 1 * *"

// sendTimeout caps one site's report. A hung Resend call must not stall every
// other site's report behind it.
const sendTimeout = 2 * time.Minute

// Reporter builds and sends monthly reports.
type Reporter struct {
	app    storage.AppStore
	events storage.EventStore
	log    *slog.Logger

	// now is overridable so tests can pick a month without waiting for one.
	now func() time.Time

	// sender overrides the Resend client. Set only by tests -- the
	// alternative is a suite that either mails real people or never runs the
	// code that mails people.
	sender email.Sender

	// endpoint overrides Resend's URL, so a deployment can be verified against
	// a mock. Empty means Resend itself.
	endpoint string
}

// SetEndpoint points the reporter at a Resend-compatible endpoint instead of
// Resend, for verifying a deployment without mailing clients.
func (r *Reporter) SetEndpoint(endpoint string) {
	r.endpoint = endpoint
}

// senderFor returns the sender to deliver with.
func (r *Reporter) senderFor(settings storage.Settings) (email.Sender, error) {
	if r.sender != nil {
		return r.sender, nil
	}
	return email.NewResendAt(settings.ResendAPIKey, r.endpoint)
}

// NewReporter returns a Reporter.
func NewReporter(app storage.AppStore, events storage.EventStore, log *slog.Logger) *Reporter {
	return &Reporter{app: app, events: events, log: log, now: time.Now}
}

// Scheduler runs the Reporter on a schedule.
type Scheduler struct {
	cron     *cron.Cron
	reporter *Reporter
	log      *slog.Logger
}

// New returns a Scheduler.
//
// Built in, rather than an external cron: a self-hosted product that needs a
// crontab entry to do half its job is one most people will deploy half-broken.
func New(reporter *Reporter, log *slog.Logger) *Scheduler {
	return &Scheduler{
		// UTC so the schedule does not shift under the server's timezone, or
		// fire twice / not at all across a DST boundary.
		cron:     cron.New(cron.WithLocation(time.UTC)),
		reporter: reporter,
		log:      log,
	}
}

// Start begins the schedule. It returns immediately.
func (s *Scheduler) Start() error {
	_, err := s.cron.AddFunc(MonthlySchedule, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		s.log.Info("monthly reports: starting")
		sent, failed := s.reporter.SendMonthly(ctx)
		s.log.Info("monthly reports: finished", "sent", sent, "failed", failed)
	})
	if err != nil {
		return err
	}

	s.cron.Start()
	s.log.Info("scheduler started", "schedule", MonthlySchedule, "next", s.Next())
	return nil
}

// Next reports when reports next go out.
func (s *Scheduler) Next() time.Time {
	entries := s.cron.Entries()
	if len(entries) == 0 {
		return time.Time{}
	}
	return entries[0].Next
}

// Stop halts the schedule and waits for a run in progress.
func (s *Scheduler) Stop() {
	<-s.cron.Stop().Done()
}

// SendMonthly sends last month's report to every site that has an owner. It
// returns how many were sent and how many failed.
//
// Safe to call more than once for the same month: a site whose report already
// went out is skipped. That is what makes a restart on the 1st harmless.
func (r *Reporter) SendMonthly(ctx context.Context) (sent, failed int) {
	settings, err := r.app.Settings(ctx)
	if err != nil {
		r.log.Error("monthly reports: settings", "err", err)
		return 0, 0
	}
	if !settings.Configured() {
		r.log.Warn("monthly reports: skipped, email is not configured " +
			"(set the Resend API key and MAIL FROM in settings)")
		return 0, 0
	}

	sender, err := r.senderFor(settings)
	if err != nil {
		r.log.Error("monthly reports: sender", "err", err)
		return 0, 0
	}

	sites, err := r.app.ListSites(ctx)
	if err != nil {
		r.log.Error("monthly reports: list sites", "err", err)
		return 0, 0
	}

	period := report.Period(r.now())

	for _, site := range sites {
		if site.OwnerEmail == "" {
			continue
		}

		// Already sent this month. This is the guard that matters: without it,
		// a restart or a manual re-run mails every client a second copy.
		existing, err := r.app.ReportFor(ctx, site.ID, period)
		if err == nil && existing.Status == storage.ReportSent {
			continue
		}
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			r.log.Error("monthly reports: history", "err", err, "site_id", site.ID)
			continue
		}

		if err := r.sendOne(ctx, sender, settings, site, period); err != nil {
			failed++
			continue
		}
		sent++
	}

	return sent, failed
}

// sendOne builds and sends one site's report, and records the outcome.
func (r *Reporter) sendOne(
	ctx context.Context,
	sender email.Sender,
	settings storage.Settings,
	site storage.Site,
	period string,
) error {
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	err := r.deliver(ctx, sender, settings, site, period)

	// Recorded either way. A failure that leaves no trace is a failure nobody
	// finds out about until a client asks where their report went.
	outcome := storage.Report{
		SiteID: site.ID,
		Period: period,
		SentAt: time.Now().UTC(),
		Status: storage.ReportSent,
	}
	if err != nil {
		outcome.Status = storage.ReportFailed
		outcome.Err = err.Error()
	}

	if reportID, idErr := id.New(); idErr == nil {
		outcome.ID = reportID
		if recErr := r.app.RecordReport(ctx, outcome); recErr != nil {
			r.log.Error("monthly reports: record", "err", recErr, "site_id", site.ID)
		}
	}

	if err != nil {
		r.log.Error("monthly report failed", "err", err, "site_id", site.ID, "period", period)
		return err
	}

	r.log.Info("monthly report sent", "site_id", site.ID, "period", period)
	return nil
}

// deliver builds and sends a finished month's report, without recording.
func (r *Reporter) deliver(
	ctx context.Context,
	sender email.Sender,
	settings storage.Settings,
	site storage.Site,
	period string,
) error {
	window, err := report.FullMonth(period)
	if err != nil {
		return err
	}
	return r.deliverWindow(ctx, sender, settings, site, window)
}

// deliverWindow builds and sends the analytics report for a window.
func (r *Reporter) deliverWindow(
	ctx context.Context,
	sender email.Sender,
	settings storage.Settings,
	site storage.Site,
	window report.Window,
) error {
	data, err := report.BuildWindow(ctx, r.events, site, window)
	if err != nil {
		return err
	}

	html, err := report.Render(data)
	if err != nil {
		return err
	}

	return sender.Send(ctx, email.Message{
		From:    settings.MailFrom,
		To:      site.OwnerEmail,
		Subject: report.Subject(data),
		HTML:    html,
	})
}

// deliverSEO builds and sends the SEO report from the site's latest audit.
func (r *Reporter) deliverSEO(
	ctx context.Context,
	sender email.Sender,
	settings storage.Settings,
	site storage.Site,
) error {
	data, err := report.BuildSEO(ctx, r.app, site)
	if err != nil {
		return err
	}

	html, err := report.RenderSEO(data)
	if err != nil {
		return err
	}

	return sender.Send(ctx, email.Message{
		From:    settings.MailFrom,
		To:      site.OwnerEmail,
		Subject: report.SEOSubject(data),
		HTML:    html,
	})
}

// SendOptions selects what an on-demand send includes.
type SendOptions struct {
	Analytics bool
	SEO       bool
}

// SendResult says what actually went out.
type SendResult struct {
	SentTo string

	// Period is the month the analytics report covered, as "YYYY-MM".
	Period string

	Analytics bool
	SEO       bool

	// SEONote explains why the SEO report was left out when it was asked for
	// and could not be sent -- almost always "no audit has been run yet".
	// Empty when nothing was skipped.
	SEONote string
}

// ErrNothingSelected means a send was asked for with no report chosen.
var ErrNothingSelected = errors.New("choose at least one report to send")

// SendNow sends a site's reports immediately, to its owner.
//
// The analytics report covers the month in progress rather than the last
// finished one. A developer clicking "send report" wants to show a client what
// their site is doing, and a site added this month has no finished month --
// mailing them a page of zeroes for a month that predates the site would be
// worse than not sending at all.
//
// It deliberately does not touch report_history. Recording it would mark the
// month as done, and when the 1st came around the scheduler would skip the
// real report -- the manual send would have silently replaced the thing it was
// meant to preview.
func (r *Reporter) SendNow(ctx context.Context, siteID string, opts SendOptions) (SendResult, error) {
	if !opts.Analytics && !opts.SEO {
		return SendResult{}, ErrNothingSelected
	}

	settings, err := r.app.Settings(ctx)
	if err != nil {
		return SendResult{}, err
	}
	if !settings.Configured() {
		return SendResult{}, email.ErrNotConfigured
	}

	site, err := r.app.SiteByID(ctx, siteID)
	if err != nil {
		return SendResult{}, err
	}
	if site.OwnerEmail == "" {
		return SendResult{}, errors.New("this site has no owner email set")
	}

	sender, err := r.senderFor(settings)
	if err != nil {
		return SendResult{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	result := SendResult{SentTo: site.OwnerEmail}

	if opts.Analytics {
		window := report.MonthToDate(r.now())
		if err := r.deliverWindow(ctx, sender, settings, site, window); err != nil {
			return SendResult{}, err
		}
		result.Analytics = true
		result.Period = window.Period
		r.log.Info("analytics report sent on demand", "site_id", site.ID, "period", window.Period)
	}

	if opts.SEO {
		err := r.deliverSEO(ctx, sender, settings, site)
		switch {
		case errors.Is(err, report.ErrNoAudit):
			// Not a failure. The developer has to run an audit first, and
			// saying so beats a 502 that reads like the mail broke -- but if
			// the SEO report was the only thing asked for, there is nothing to
			// report as sent.
			if !opts.Analytics {
				return SendResult{}, err
			}
			result.SEONote = "No completed audit yet, so the SEO report was skipped."
		case err != nil:
			return SendResult{}, err
		default:
			result.SEO = true
			r.log.Info("seo report sent on demand", "site_id", site.ID)
		}
	}

	return result, nil
}
