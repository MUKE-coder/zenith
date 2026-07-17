// Command worker runs the Zenith SEO audit worker.
//
// It is a separate service and a separate image from core on purpose: Chromium
// is ~1GB, and an out-of-memory kill while rendering a client's site must not
// be able to take analytics ingestion down with it. It is also optional --
// a developer who does not want SEO audits simply never starts it, and
// everything else keeps working.
//
// It shares one thing with core: the SQLite database, where core enqueues jobs
// and the worker claims them. That is the whole queue. At one developer's
// scale, a broker would be infrastructure to operate for no gain.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/zenith/audit-worker/internal/audit"
	"github.com/zenith/audit-worker/internal/store"
)

// pollInterval is how often an idle worker asks for work.
//
// Audits are requested by hand, minutes apart at most, so a few seconds of
// latency is invisible -- and polling harder would just be a busier database
// for the same result.
const pollInterval = 5 * time.Second

// jobTimeout caps one whole audit.
//
// 50 pages at up to 30 seconds each, plus link checks, with room to spare. A
// job that exceeds this is stuck, and failing it with a reason beats hanging
// at "running" forever.
const jobTimeout = 45 * time.Minute

// staleAfter is when a running job is presumed abandoned.
//
// Longer than jobTimeout, so a slow-but-alive audit is never reclaimed out
// from under itself.
const staleAfter = 60 * time.Minute

// reclaimInterval is how often abandoned jobs are requeued.
const reclaimInterval = 5 * time.Minute

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	dataDir := env("ZENITH_DATA_DIR", "./data")
	dbPath := env("ZENITH_APP_DB", filepath.Join(dataDir, "zenith.sqlite"))
	chromePath := os.Getenv("ZENITH_CHROME_PATH")

	concurrency, err := strconv.Atoi(env("ZENITH_AUDIT_CONCURRENCY", "1"))
	if err != nil || concurrency < 1 {
		return errors.New("ZENITH_AUDIT_CONCURRENCY must be a positive number")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	log.Info("queue ready", "path", dbPath)

	browser, err := audit.NewBrowser(ctx, chromePath)
	if err != nil {
		return err
	}
	defer browser.Close()
	log.Info("chromium ready", "path", orDefault(chromePath, "(from PATH)"))

	auditor := audit.NewAuditor(browser, log)

	// A crashed worker leaves its job marked running, and nothing else will
	// ever claim it: the claim only looks at queued.
	go reclaimLoop(ctx, db, log)

	log.Info("audit worker started", "concurrency", concurrency, "max_pages", audit.MaxPages)

	// Workers share one browser: each renders in its own tab. The cap is what
	// bounds memory -- every concurrent audit is another set of live tabs.
	done := make(chan struct{}, concurrency)
	for i := range concurrency {
		go func(n int) {
			pollLoop(ctx, db, auditor, log.With("worker", n))
			done <- struct{}{}
		}(i + 1)
	}

	<-ctx.Done()
	log.Info("shutting down, waiting for audits in progress")

	// Let a job in flight finish and record its result, rather than leaving
	// the row at "running" for the reclaim sweep to find later.
	for range concurrency {
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			log.Warn("an audit did not stop in time")
			return nil
		}
	}

	log.Info("audit worker stopped")
	return nil
}

// pollLoop claims and runs jobs until the context ends.
func pollLoop(ctx context.Context, db *store.Store, auditor *audit.Auditor, log *slog.Logger) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		// Drain the queue before waiting again: a burst of requests should not
		// take one poll interval each.
		for {
			claimed, err := runOne(ctx, db, auditor, log)
			if err != nil || !claimed {
				break
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// runOne claims a job and audits it. It reports whether it claimed one.
func runOne(ctx context.Context, db *store.Store, auditor *audit.Auditor, log *slog.Logger) (bool, error) {
	job, err := db.Claim(ctx)
	if errors.Is(err, store.ErrNoJob) {
		return false, nil
	}
	if err != nil {
		if ctx.Err() != nil {
			return false, err
		}
		log.Error("claim failed", "err", err)
		return false, err
	}

	log.Info("audit started", "job_id", job.ID, "domain", job.SiteDomain)
	started := time.Now()

	// A background context, not the claim's: a shutdown signal must let the
	// job finish and record a result rather than abandoning a row at
	// "running" for the sweep to clean up later.
	jobCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), jobTimeout)
	defer cancel()

	result, err := auditor.Run(jobCtx, job.SiteDomain)
	if err != nil {
		reason := failureReason(err)
		log.Error("audit failed", "job_id", job.ID, "err", err)

		if failErr := db.Fail(jobCtx, job.ID, reason); failErr != nil {
			log.Error("could not record failure", "job_id", job.ID, "err", failErr)
		}
		return true, nil
	}

	rows := make([]store.PageResult, 0, len(result.Pages))
	for _, page := range result.Pages {
		checks, err := page.ChecksJSON()
		if err != nil {
			log.Error("could not encode checks", "url", page.URL, "err", err)
			continue
		}
		rows = append(rows, store.PageResult{URL: page.URL, Score: page.Score, Checks: checks})
	}

	if err := db.SaveResults(jobCtx, job.ID, rows); err != nil {
		log.Error("could not save results", "job_id", job.ID, "err", err)
		if failErr := db.Fail(jobCtx, job.ID, "The results couldn't be saved."); failErr != nil {
			log.Error("could not record failure", "job_id", job.ID, "err", failErr)
		}
		return true, nil
	}

	if err := db.Finish(jobCtx, job.ID, result.Score); err != nil {
		log.Error("could not finish job", "job_id", job.ID, "err", err)
		return true, nil
	}

	log.Info("audit finished",
		"job_id", job.ID, "pages", len(result.Pages), "score", result.Score,
		"took", time.Since(started).Round(time.Second))

	return true, nil
}

// failureReason turns an error into something the console can show.
//
// A missing sitemap already explains itself and what to do about it; anything
// else is summarized, because a raw chromedp error is not a sentence anyone
// can act on.
func failureReason(err error) string {
	var noSitemap *audit.ErrNoSitemap
	if errors.As(err, &noSitemap) {
		return noSitemap.Error()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "The audit took too long and was stopped."
	}
	return "The audit couldn't finish: " + err.Error()
}

// reclaimLoop requeues jobs left running by a worker that died.
func reclaimLoop(ctx context.Context, db *store.Store, log *slog.Logger) {
	// Sweep immediately, then on the ticker. Starting up is precisely when a
	// crash has just been resolved -- someone restarted the worker -- so it is
	// the worst possible moment to make the abandoned job wait another
	// interval before anyone looks at it.
	reclaim(ctx, db, log)

	ticker := time.NewTicker(reclaimInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reclaim(ctx, db, log)
		}
	}
}

func reclaim(ctx context.Context, db *store.Store, log *slog.Logger) {
	n, err := db.ReclaimStale(ctx, staleAfter)
	if err != nil {
		log.Warn("could not reclaim stale jobs", "err", err)
		return
	}
	if n > 0 {
		log.Info("requeued abandoned audits", "count", n)
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
