package report_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zenith/core/internal/report"
	"github.com/zenith/core/internal/storage"
)

// auditStore is a stand-in for the app store, holding one site's audits.
type auditStore struct {
	jobs    []storage.AuditJob
	results map[string][]storage.AuditResult
}

func (a auditStore) AuditJobsForSite(context.Context, string) ([]storage.AuditJob, error) {
	return a.jobs, nil
}

func (a auditStore) AuditResultsForJob(_ context.Context, jobID string) ([]storage.AuditResult, error) {
	return a.results[jobID], nil
}

func done(id string, score int, at time.Time) storage.AuditJob {
	return storage.AuditJob{ID: id, SiteID: "site-1", Status: storage.AuditDone, Score: score, FinishedAt: at}
}

func page(url string, score int, checks string) storage.AuditResult {
	return storage.AuditResult{PageURL: url, Score: score, Checks: checks}
}

const (
	altMissing  = `{"id":"images.alt_missing","severity":"error","message":"Images are missing alt text."}`
	titleLong   = `{"id":"title.long","severity":"warning","message":"The title is too long."}`
	titleOK     = `{"id":"title.ok","severity":"ok","message":"The title is a good length."}`
	descMissing = `{"id":"description.missing","severity":"error","message":"No meta description."}`
)

func checks(items ...string) string {
	return `{"checks":[` + strings.Join(items, ",") + `],"vitals":{}}`
}

// The report is about the newest finished audit. A queued one that has not run
// has nothing to say, and must not hide the last real result.
func TestBuildSEOUsesTheLatestFinishedAudit(t *testing.T) {
	store := auditStore{
		jobs: []storage.AuditJob{
			{ID: "job-3", Status: storage.AuditQueued},
			done("job-2", 82, time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)),
			done("job-1", 40, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)),
		},
		results: map[string][]storage.AuditResult{
			"job-2": {page("https://acme.com/", 82, checks(titleOK))},
		},
	}

	data, err := report.BuildSEO(context.Background(), store, site)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if data.Score != 82 {
		t.Errorf("score = %d, want 82 (the newest finished audit)", data.Score)
	}
	if data.AuditedLabel != "2 July 2026" {
		t.Errorf("audited = %q, want 2 July 2026", data.AuditedLabel)
	}
}

// A site that has never been audited is a thing to do, not a failure to report.
func TestBuildSEOWithoutAnAudit(t *testing.T) {
	cases := map[string]auditStore{
		"no audits at all": {},
		"none finished": {
			jobs: []storage.AuditJob{{ID: "job-1", Status: storage.AuditRunning}},
		},
	}

	for name, store := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := report.BuildSEO(context.Background(), store, site)
			if !errors.Is(err, report.ErrNoAudit) {
				t.Errorf("got %v, want ErrNoAudit", err)
			}
		})
	}
}

// One problem across four pages is one line, not four -- and it counts the
// pages so the reader knows the size of it.
func TestSEOIssuesAggregateAcrossPages(t *testing.T) {
	store := auditStore{
		jobs: []storage.AuditJob{done("job-1", 60, time.Now())},
		results: map[string][]storage.AuditResult{
			"job-1": {
				page("https://acme.com/", 60, checks(altMissing, titleOK)),
				page("https://acme.com/pricing", 55, checks(altMissing)),
				page("https://acme.com/blog", 70, checks(altMissing, titleLong)),
			},
		},
	}

	data, err := report.BuildSEO(context.Background(), store, site)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if len(data.Issues) != 2 {
		t.Fatalf("got %d issues, want 2 (alt text and title, deduplicated)", len(data.Issues))
	}
	if data.Issues[0].Pages != 3 {
		t.Errorf("alt text affects %d pages, want 3", data.Issues[0].Pages)
	}
}

// Errors first, then whatever affects the most pages: the email is a to-do
// list and its order is the advice.
func TestSEOIssuesAreWorstFirst(t *testing.T) {
	store := auditStore{
		jobs: []storage.AuditJob{done("job-1", 50, time.Now())},
		results: map[string][]storage.AuditResult{
			"job-1": {
				page("https://acme.com/a", 50, checks(titleLong)),
				page("https://acme.com/b", 50, checks(titleLong)),
				page("https://acme.com/c", 50, checks(descMissing)),
			},
		},
	}

	data, err := report.BuildSEO(context.Background(), store, site)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if data.Issues[0].Severity != "error" {
		t.Errorf("first issue is %q, want the error ahead of the more common warning", data.Issues[0].Severity)
	}
}

// "ok" is a passing check. Mailing a client a list of things that are fine,
// under the heading "what to fix first", would be nonsense.
func TestSEOIssuesExcludePassingChecks(t *testing.T) {
	store := auditStore{
		jobs: []storage.AuditJob{done("job-1", 100, time.Now())},
		results: map[string][]storage.AuditResult{
			"job-1": {page("https://acme.com/", 100, checks(titleOK))},
		},
	}

	data, err := report.BuildSEO(context.Background(), store, site)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(data.Issues) != 0 {
		t.Errorf("got %d issues from a clean audit, want 0", len(data.Issues))
	}
}

// The pages listed are the ones that need work, worst first.
func TestSEOPagesAreLowestScoringFirst(t *testing.T) {
	store := auditStore{
		jobs: []storage.AuditJob{done("job-1", 70, time.Now())},
		results: map[string][]storage.AuditResult{
			"job-1": {
				page("https://acme.com/good", 95, checks()),
				page("https://acme.com/bad?ref=x", 30, checks()),
				page("https://acme.com/mid", 60, checks()),
			},
		},
	}

	data, err := report.BuildSEO(context.Background(), store, site)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if data.Pages[0].Score != 30 {
		t.Errorf("first page scores %d, want the worst (30)", data.Pages[0].Score)
	}
	// The domain is the subject of the email; the path is what identifies a page.
	if data.Pages[0].Path != "/bad?ref=x" {
		t.Errorf("path = %q, want /bad?ref=x", data.Pages[0].Path)
	}
}

// A page whose findings will not parse must cost that page, not the send.
func TestSEOSurvivesUnreadableChecks(t *testing.T) {
	store := auditStore{
		jobs: []storage.AuditJob{done("job-1", 60, time.Now())},
		results: map[string][]storage.AuditResult{
			"job-1": {
				page("https://acme.com/broken", 60, "not json at all"),
				page("https://acme.com/fine", 60, checks(altMissing)),
			},
		},
	}

	data, err := report.BuildSEO(context.Background(), store, site)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(data.Issues) != 1 {
		t.Errorf("got %d issues, want the one readable page's", len(data.Issues))
	}
	if data.PagesAudited != 2 {
		t.Errorf("audited = %d, want both pages counted", data.PagesAudited)
	}
}

// The subject has to be identifiable in an inbox without being opened.
func TestSEOSubjectNamesTheSiteAndScore(t *testing.T) {
	got := report.SEOSubject(report.SEOData{SiteName: "Acme", Score: 82})
	if !strings.Contains(got, "Acme") || !strings.Contains(got, "82") {
		t.Errorf("subject = %q, want the site and the score", got)
	}
}

// The email is HTML built by hand, so a site name with markup in it must not
// be able to inject any.
func TestRenderSEOEscapesTheSiteName(t *testing.T) {
	html, err := report.RenderSEO(report.SEOData{
		SiteName: `<script>alert(1)</script>`,
		Score:    50,
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(html, "<script>alert(1)</script>") {
		t.Error("the site name was rendered as live markup")
	}
}

// Without a dashboard path there is nowhere to send the reader, and a button
// pointing at a page that is not there is worse than no button.
func TestRenderSEOOmitsTheLinkWithoutADashboard(t *testing.T) {
	html, err := report.RenderSEO(report.SEOData{SiteName: "Acme", Score: 90})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(html, "View the full audit") {
		t.Error("rendered a dashboard button with no dashboard URL")
	}
}

// The counterpart to the omission test: with a path recorded, the button is
// there and points at the client's own domain. This is the whole reason the
// path is stored, so it is worth asserting in both directions.
func TestRenderSEOLinksToTheDashboard(t *testing.T) {
	html, err := report.RenderSEO(report.SEOData{
		SiteName: "Acme", Score: 69,
		DashboardURL: "https://acme.com/zenith",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(html, "View full report") {
		t.Error("no call to action, even with a dashboard to send the reader to")
	}
	if !strings.Contains(html, "https://acme.com/zenith") {
		t.Error("the button does not point at the client's dashboard")
	}
}

// The footer credits Zenith, not the domain the mail happened to leave from.
// That domain is the developer's own sending infrastructure, and putting it in
// front of their client named a stranger's host in a client-facing email.
func TestFootersCreditZenith(t *testing.T) {
	seo, err := report.RenderSEO(report.SEOData{SiteName: "Acme", Score: 90})
	if err != nil {
		t.Fatalf("render seo: %v", err)
	}

	s := store(t)
	data, err := report.Build(context.Background(), s, site, "2026-06")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	analytics, err := report.Render(data)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	for name, html := range map[string]string{"seo": seo, "analytics": analytics} {
		if !strings.Contains(html, "Powered by") {
			t.Errorf("%s: footer does not credit Zenith", name)
		}
		if !strings.Contains(html, "https://zenith.gritframework.dev") {
			t.Errorf("%s: footer does not link to Zenith", name)
		}
		if strings.Contains(html, "Sent by") {
			t.Errorf("%s: footer still names the sending domain", name)
		}
	}
}
