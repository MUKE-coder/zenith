package report

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/zenith/core/internal/storage"
)

// Issues is how many findings the SEO email lists.
//
// Same reasoning as Rows: the email is the nudge, the console is the tool. A
// client who reads five real problems will fix one; a client who receives
// forty will read none.
const Issues = 6

// AuditStore is the slice of the app store the SEO report needs.
//
// Declared here rather than taking the whole AppStore so the dependency is
// visible: this package reads audits, and writes nothing.
type AuditStore interface {
	AuditJobsForSite(ctx context.Context, siteID string) ([]storage.AuditJob, error)
	AuditResultsForJob(ctx context.Context, jobID string) ([]storage.AuditResult, error)
}

// SEOData is everything the SEO email renders.
type SEOData struct {
	SiteName   string
	SiteDomain string

	// AuditedLabel is when the audit ran, for a human.
	AuditedLabel string

	// Score is the site-wide score, 0-100.
	Score int

	// ScoreLabel reads the score for someone who does not know the scale.
	ScoreLabel string

	// ScoreColor is the swatch the template paints the score in.
	ScoreColor string

	Pages  []SEOPage
	Issues []SEOIssue

	// PagesAudited is the true page count, which may exceed len(Pages).
	PagesAudited int

	// Errors and Warnings count distinct findings across the whole audit, not
	// only the few listed. The summary tiles state the size of the problem;
	// the list only shows where to start.
	Errors   int
	Warnings int

	// SentBy is the domain the mail went out from, for the footer.
	SentBy string

	// DashboardURL is the owner's own dashboard, whose SEO tab holds the full
	// audit. Empty if the site has no dashboard path recorded.
	DashboardURL string
}

// SEOPage is one audited page's score.
type SEOPage struct {
	// Path is the URL's path, which is what identifies a page to its owner.
	// The domain is already the subject of the email.
	Path  string
	URL   string
	Score int
}

// SEOIssue is one finding, aggregated across the pages that share it.
type SEOIssue struct {
	Severity string

	// Title is the finding at a glance, and Detail is what to do about it.
	// The worker writes both as one message, so they are split here: the email
	// leads with the problem and sets the advice underneath, where a reader
	// scanning a list can skip it.
	Title  string
	Detail string

	// Pages is how many audited pages have this finding.
	Pages int

	// Example is one page's path, so the reader knows where to look.
	Example string
}

// splitMessage divides a finding into its headline and its advice at the first
// sentence boundary. A message with only one sentence is all headline.
func splitMessage(message string) (title, detail string) {
	message = strings.TrimSpace(message)

	if i := strings.Index(message, ". "); i != -1 {
		return strings.TrimSuffix(message[:i], "."), strings.TrimSpace(message[i+2:])
	}
	return strings.TrimSuffix(message, "."), ""
}

// checksPayload mirrors the audit worker's per-page JSON. Only the fields the
// email uses are declared; the rest is ignored on purpose, so a new check or
// vital in the worker cannot break a send.
type checksPayload struct {
	Checks []struct {
		ID       string `json:"id"`
		Severity string `json:"severity"`
		Message  string `json:"message"`
	} `json:"checks"`
}

// Audit severities, as the worker writes them.
const (
	severityError   = "error"
	severityWarning = "warning"
)

// SEOSubject returns the SEO email's subject line.
func SEOSubject(d SEOData) string {
	return fmt.Sprintf("%s — SEO audit, score %d/100", d.SiteName, d.Score)
}

// ErrNoAudit means the site has never completed an audit, so there is nothing
// to report. The caller turns this into a message, not a failure: it is a
// thing the developer must do, not a thing that went wrong.
var ErrNoAudit = fmt.Errorf("report: this site has no completed audit yet")

// BuildSEO compiles a site's SEO report from its most recent finished audit.
func BuildSEO(ctx context.Context, store AuditStore, site storage.Site) (SEOData, error) {
	jobs, err := store.AuditJobsForSite(ctx, site.ID)
	if err != nil {
		return SEOData{}, fmt.Errorf("report: audits: %w", err)
	}

	// Newest first, so the first finished one is the latest. A queued or
	// running audit is skipped rather than waited for -- the report is about
	// what is known now.
	var job storage.AuditJob
	for _, j := range jobs {
		if j.Status == storage.AuditDone {
			job = j
			break
		}
	}
	if job.ID == "" {
		return SEOData{}, ErrNoAudit
	}

	results, err := store.AuditResultsForJob(ctx, job.ID)
	if err != nil {
		return SEOData{}, fmt.Errorf("report: audit results: %w", err)
	}

	pages := make([]SEOPage, 0, len(results))
	for _, r := range results {
		pages = append(pages, SEOPage{
			Path:  pathOf(r.PageURL),
			URL:   r.PageURL,
			Score: r.Score,
		})
	}

	// Worst first: the pages that need attention are the point of the email.
	sort.SliceStable(pages, func(i, j int) bool { return pages[i].Score < pages[j].Score })

	audited := len(pages)
	if len(pages) > Rows {
		pages = pages[:Rows]
	}

	issues, errorCount, warningCount := topIssues(results)

	return SEOData{
		SiteName:     site.Name,
		SiteDomain:   site.Domain,
		AuditedLabel: auditedLabel(job),
		Score:        job.Score,
		ScoreLabel:   scoreLabel(job.Score),
		ScoreColor:   scoreColor(job.Score),
		Pages:        pages,
		Issues:       issues,
		PagesAudited: audited,
		Errors:       errorCount,
		Warnings:     warningCount,
		DashboardURL: site.DashboardURL(),
	}, nil
}

// topIssues aggregates every page's findings into the worst few.
//
// Grouped by message rather than listed per page: "12 images are missing alt
// text" across four pages is one problem to fix, and four identical lines
// would bury the other three findings.
// It also returns how many distinct error and warning findings the audit has
// in total, which the summary tiles state.
func topIssues(results []storage.AuditResult) ([]SEOIssue, int, int) {
	type agg struct {
		severity string
		message  string
		pages    int
		example  string
		order    int
	}

	byID := make(map[string]*agg)
	seen := 0

	for _, r := range results {
		var payload checksPayload
		if err := json.Unmarshal([]byte(r.Checks), &payload); err != nil {
			// One unreadable page must not cost the whole report. The console
			// still has the raw findings.
			continue
		}

		for _, c := range payload.Checks {
			if c.Severity != severityError && c.Severity != severityWarning {
				continue
			}

			entry, ok := byID[c.ID]
			if !ok {
				seen++
				entry = &agg{
					severity: c.Severity,
					message:  c.Message,
					example:  pathOf(r.PageURL),
					order:    seen,
				}
				byID[c.ID] = entry
			}
			entry.pages++
		}
	}

	out := make([]SEOIssue, 0, len(byID))
	for _, e := range byID {
		title, detail := splitMessage(e.message)
		out = append(out, SEOIssue{
			Severity: e.severity,
			Title:    title,
			Detail:   detail,
			Pages:    e.pages,
			Example:  e.example,
		})
	}

	// Errors before warnings, then by how many pages are affected. Ties fall
	// back to first-seen so the same audit always renders the same email.
	order := map[string]int{severityError: 0, severityWarning: 1}
	sort.SliceStable(out, func(i, j int) bool {
		if order[out[i].Severity] != order[out[j].Severity] {
			return order[out[i].Severity] < order[out[j].Severity]
		}
		return out[i].Pages > out[j].Pages
	})

	// Counted over everything found, not over the few listed: the tiles state
	// the size of the problem and the list only says where to start.
	var errors, warnings int
	for _, issue := range out {
		if issue.Severity == severityError {
			errors++
		} else {
			warnings++
		}
	}

	if len(out) > Issues {
		out = out[:Issues]
	}
	return out, errors, warnings
}

// pathOf reduces a page URL to its path, or returns it whole if it will not
// parse. "/pricing" identifies a page to its owner; the origin is the email's
// subject already.
func pathOf(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Path == "" {
		return raw
	}
	path := parsed.Path
	if parsed.RawQuery != "" {
		path += "?" + parsed.RawQuery
	}
	return path
}

func auditedLabel(job storage.AuditJob) string {
	when := job.FinishedAt
	if when.IsZero() {
		when = job.RequestedAt
	}
	if when.IsZero() {
		return ""
	}
	return when.UTC().Format("2 January 2006")
}

// scoreLabel reads a score for someone who has never seen the scale.
func scoreLabel(score int) string {
	switch {
	case score >= 90:
		return "Good"
	case score >= 70:
		return "Needs work"
	default:
		return "Poor"
	}
}

func scoreColor(score int) string {
	switch {
	case score >= 90:
		return colorPositive
	case score >= 70:
		return colorWarning
	default:
		return colorNegative
	}
}

// scoreTint is the pill background behind the score's label.
func scoreTint(score int) string {
	switch {
	case score >= 90:
		return tintPositive
	case score >= 70:
		return tintWarning
	default:
		return tintNegative
	}
}

// severityTint is the pill background behind a finding's severity.
func severityTint(severity string) string {
	if severity == severityError {
		return tintNegative
	}
	return tintWarning
}

// severityLabel names a severity for the email.
func severityLabel(severity string) string {
	if severity == severityError {
		return "Error"
	}
	return "Warning"
}

// severityColor is the swatch a severity is painted in.
func severityColor(severity string) string {
	if severity == severityError {
		return colorNegative
	}
	return colorWarning
}
