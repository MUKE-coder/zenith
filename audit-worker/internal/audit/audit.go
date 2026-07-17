// Package audit renders a site and reports what is wrong with it.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Result is a finished audit.
type Result struct {
	Pages []PageResult
	Score int
}

// PageResult is one audited page.
type PageResult struct {
	URL    string
	Score  int
	Checks PageChecks
}

// ChecksJSON renders a page's findings for storage.
func (p PageResult) ChecksJSON() (string, error) {
	raw, err := json.Marshal(p.Checks)
	if err != nil {
		return "", fmt.Errorf("audit: encode checks: %w", err)
	}
	return string(raw), nil
}

// Auditor runs audits.
type Auditor struct {
	browser *Browser
	client  *http.Client
	log     *slog.Logger
}

// NewAuditor returns an Auditor over a browser.
func NewAuditor(browser *Browser, log *slog.Logger) *Auditor {
	return &Auditor{
		browser: browser,
		client:  &http.Client{Timeout: 30 * time.Second},
		log:     log,
	}
}

// Run audits a site.
//
// Pages are rendered one at a time on purpose. Chromium is the memory hog the
// whole separate-service split exists to contain, and rendering several pages
// at once is the fastest way to prove it: the worker would be OOM-killed
// mid-audit, which is exactly the failure the architecture is meant to make
// survivable rather than to invite.
func (a *Auditor) Run(ctx context.Context, domain string) (Result, error) {
	pages, err := FetchSitemap(ctx, a.client, domain)
	if err != nil {
		return Result{}, err
	}

	a.log.Info("audit: pages found", "domain", domain, "count", len(pages))

	links := NewLinkChecker()

	results := make([]PageResult, 0, len(pages))
	scores := make([]int, 0, len(pages))

	for _, pageURL := range pages {
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		default:
		}

		result, err := a.auditPage(ctx, links, pageURL)
		if err != nil {
			// One page that will not render must not sink the audit: a site
			// with 49 good pages and one broken one is a report worth having,
			// and the broken one is itself the finding.
			a.log.Warn("audit: page failed", "url", pageURL, "err", err)

			results = append(results, PageResult{
				URL:   pageURL,
				Score: 0,
				Checks: PageChecks{Checks: []Check{{
					ID: "page.unreachable", Severity: SeverityError,
					Message: "This page couldn't be loaded.",
					Detail:  err.Error(),
				}}},
			})
			scores = append(scores, 0)
			continue
		}

		results = append(results, result)
		scores = append(scores, result.Score)
	}

	return Result{Pages: results, Score: SiteScore(scores)}, nil
}

// auditPage renders and evaluates one page.
func (a *Auditor) auditPage(ctx context.Context, links *LinkChecker, pageURL string) (PageResult, error) {
	page, err := a.browser.Render(ctx, pageURL)
	if err != nil {
		return PageResult{}, err
	}

	broken := links.Check(ctx, page.Links)
	checks := Evaluate(page, broken)

	return PageResult{
		URL:    pageURL,
		Score:  Score(checks.Checks),
		Checks: checks,
	}, nil
}
