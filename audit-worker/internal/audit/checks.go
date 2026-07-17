package audit

import (
	"fmt"
	"strings"
)

// Severities, worst first.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
	SeverityOK      = "ok"
)

// Check is one finding about a page.
type Check struct {
	// ID is a stable slug, e.g. "title.missing". The UI groups on it; the
	// message is free to be rewritten without breaking anything.
	ID string `json:"id"`

	Severity string `json:"severity"`

	// Message says what is wrong and what to do, in the interface's voice.
	Message string `json:"message"`

	// Detail carries the offending value, when showing it helps.
	Detail string `json:"detail,omitempty"`
}

// Vitals are the performance numbers from the real render.
type Vitals struct {
	// TTFB, FCP, LCP in milliseconds. CLS is unitless.
	TTFB float64 `json:"ttfb_ms"`
	FCP  float64 `json:"fcp_ms"`
	LCP  float64 `json:"lcp_ms"`
	CLS  float64 `json:"cls"`

	// DOMContentLoaded and Load in milliseconds.
	DOMContentLoaded float64 `json:"dcl_ms"`
	Load             float64 `json:"load_ms"`
}

// PageChecks is everything found about one page.
type PageChecks struct {
	Checks []Check `json:"checks"`
	Vitals Vitals  `json:"vitals"`

	// Title and Description are echoed so the console can show what the page
	// actually has, not only what is wrong with it.
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

// Thresholds for the on-page checks.
//
// The title and description bounds are what search engines actually render
// before truncating; the rest is Core Web Vitals' own "good" thresholds.
const (
	titleMin = 10
	titleMax = 60

	descriptionMin = 50
	descriptionMax = 160

	goodLCP  = 2500.0
	poorLCP  = 4000.0
	goodCLS  = 0.1
	poorCLS  = 0.25
	goodTTFB = 800.0
)

// Page is what the browser extracted from one page.
type Page struct {
	URL         string
	StatusCode  int
	Title       string
	Description string
	Canonical   string
	Robots      string
	H1s         []string
	Headings    []string
	ImagesTotal int
	ImagesNoAlt int
	Links       []string
	JSONLD      []string
	Vitals      Vitals
}

// Evaluate turns a rendered page into findings.
func Evaluate(page Page, brokenLinks []BrokenLink) PageChecks {
	out := PageChecks{
		Title:       page.Title,
		Description: page.Description,
		Vitals:      page.Vitals,
		Checks:      []Check{},
	}

	add := func(c Check) { out.Checks = append(out.Checks, c) }

	add(checkTitle(page.Title))
	add(checkDescription(page.Description))
	add(checkH1(page.H1s))
	add(checkHeadingOrder(page.Headings))
	add(checkAltText(page.ImagesTotal, page.ImagesNoAlt))
	add(checkCanonical(page.Canonical, page.URL))
	add(checkRobots(page.Robots))
	add(checkStructuredData(page.JSONLD))
	add(checkBrokenLinks(brokenLinks))
	out.Checks = append(out.Checks, checkVitals(page.Vitals)...)

	return out
}

func checkTitle(title string) Check {
	trimmed := strings.TrimSpace(title)

	switch {
	case trimmed == "":
		return Check{ID: "title.missing", Severity: SeverityError,
			Message: "This page has no title. Search results will show the URL instead."}
	case len(trimmed) < titleMin:
		return Check{ID: "title.short", Severity: SeverityWarning,
			Message: fmt.Sprintf("The title is very short (%d characters). Describe the page.", len(trimmed)),
			Detail:  trimmed}
	case len(trimmed) > titleMax:
		return Check{ID: "title.long", Severity: SeverityWarning,
			Message: fmt.Sprintf("The title is %d characters and will be cut off around %d.", len(trimmed), titleMax),
			Detail:  trimmed}
	default:
		return Check{ID: "title.ok", Severity: SeverityOK, Message: "The title is a good length.", Detail: trimmed}
	}
}

func checkDescription(description string) Check {
	trimmed := strings.TrimSpace(description)

	switch {
	case trimmed == "":
		return Check{ID: "description.missing", Severity: SeverityError,
			Message: "No meta description. Search engines will invent one from the page text."}
	case len(trimmed) < descriptionMin:
		return Check{ID: "description.short", Severity: SeverityWarning,
			Message: fmt.Sprintf("The meta description is only %d characters. Aim for %d to %d.",
				len(trimmed), descriptionMin, descriptionMax)}
	case len(trimmed) > descriptionMax:
		return Check{ID: "description.long", Severity: SeverityWarning,
			Message: fmt.Sprintf("The meta description is %d characters and will be cut off around %d.",
				len(trimmed), descriptionMax)}
	default:
		return Check{ID: "description.ok", Severity: SeverityOK, Message: "The meta description is a good length."}
	}
}

func checkH1(h1s []string) Check {
	switch len(h1s) {
	case 0:
		return Check{ID: "h1.missing", Severity: SeverityError,
			Message: "This page has no H1. Give it one heading that names what the page is."}
	case 1:
		return Check{ID: "h1.ok", Severity: SeverityOK, Message: "The page has one H1.", Detail: h1s[0]}
	default:
		return Check{ID: "h1.multiple", Severity: SeverityWarning,
			Message: fmt.Sprintf("This page has %d H1s. One is clearer to readers and to crawlers.", len(h1s)),
			Detail:  strings.Join(h1s, " · ")}
	}
}

// checkHeadingOrder finds skipped levels, e.g. an H2 followed by an H4.
//
// A skipped level breaks the outline a screen reader navigates by, which is
// the same structure a crawler reads.
func checkHeadingOrder(headings []string) Check {
	previous := 0

	for _, tag := range headings {
		if len(tag) != 2 || tag[0] != 'h' {
			continue
		}
		level := int(tag[1] - '0')
		if level < 1 || level > 6 {
			continue
		}

		if previous != 0 && level > previous+1 {
			return Check{ID: "headings.skipped", Severity: SeverityWarning,
				Message: fmt.Sprintf("The headings jump from H%d to H%d. Don't skip levels.", previous, level)}
		}
		previous = level
	}

	return Check{ID: "headings.ok", Severity: SeverityOK, Message: "The heading order is sound."}
}

func checkAltText(total, missing int) Check {
	if total == 0 {
		return Check{ID: "images.none", Severity: SeverityOK, Message: "This page has no images."}
	}
	if missing == 0 {
		return Check{ID: "images.ok", Severity: SeverityOK,
			Message: fmt.Sprintf("All %d images have alt text.", total)}
	}
	return Check{ID: "images.alt_missing", Severity: SeverityError,
		Message: fmt.Sprintf("%d of %d images have no alt text. Screen readers and crawlers can't see them.",
			missing, total)}
}

func checkCanonical(canonical, pageURL string) Check {
	if strings.TrimSpace(canonical) == "" {
		return Check{ID: "canonical.missing", Severity: SeverityWarning,
			Message: "No canonical URL. Add one so duplicate URLs don't compete with each other."}
	}
	return Check{ID: "canonical.ok", Severity: SeverityOK, Message: "The page declares a canonical URL.",
		Detail: canonical}
}

// checkRobots catches a page that asks not to be indexed.
//
// This is the one check that is nearly always a mistake when it fires: a
// noindex left on after a staging deploy is invisible until traffic vanishes.
func checkRobots(robots string) Check {
	lower := strings.ToLower(robots)

	if strings.Contains(lower, "noindex") {
		return Check{ID: "robots.noindex", Severity: SeverityError,
			Message: "This page asks search engines not to index it. If that's not deliberate, remove the noindex.",
			Detail:  robots}
	}
	if strings.Contains(lower, "nofollow") {
		return Check{ID: "robots.nofollow", Severity: SeverityWarning,
			Message: "This page tells crawlers not to follow its links.", Detail: robots}
	}
	return Check{ID: "robots.ok", Severity: SeverityOK, Message: "The page is indexable."}
}

func checkStructuredData(blocks []string) Check {
	if len(blocks) == 0 {
		return Check{ID: "jsonld.missing", Severity: SeverityWarning,
			Message: "No structured data. JSON-LD helps search engines show rich results."}
	}

	for i, block := range blocks {
		if err := validJSONLD(block); err != nil {
			return Check{ID: "jsonld.invalid", Severity: SeverityError,
				Message: fmt.Sprintf("Structured data block %d is not valid: %s", i+1, err),
				Detail:  truncate(block, 200)}
		}
	}

	return Check{ID: "jsonld.ok", Severity: SeverityOK,
		Message: fmt.Sprintf("%d valid structured data block(s).", len(blocks))}
}

func checkBrokenLinks(broken []BrokenLink) Check {
	if len(broken) == 0 {
		return Check{ID: "links.ok", Severity: SeverityOK, Message: "No broken links."}
	}

	details := make([]string, 0, len(broken))
	for _, link := range broken {
		details = append(details, fmt.Sprintf("%s (%s)", link.URL, link.Reason))
	}

	return Check{ID: "links.broken", Severity: SeverityError,
		Message: fmt.Sprintf("%d broken link(s) on this page.", len(broken)),
		Detail:  strings.Join(details, "\n")}
}

// checkVitals turns the real render's numbers into findings.
//
// Thresholds are Core Web Vitals' own, so a passing page here is a passing
// page in Search Console.
func checkVitals(v Vitals) []Check {
	checks := []Check{}

	switch {
	case v.LCP == 0:
		// Not measurable, e.g. a page with no meaningful paint. Not a finding.
	case v.LCP > poorLCP:
		checks = append(checks, Check{ID: "vitals.lcp_poor", Severity: SeverityError,
			Message: fmt.Sprintf("Largest paint took %.1fs. Under %.1fs is good.", v.LCP/1000, goodLCP/1000)})
	case v.LCP > goodLCP:
		checks = append(checks, Check{ID: "vitals.lcp_slow", Severity: SeverityWarning,
			Message: fmt.Sprintf("Largest paint took %.1fs. Under %.1fs is good.", v.LCP/1000, goodLCP/1000)})
	default:
		checks = append(checks, Check{ID: "vitals.lcp_ok", Severity: SeverityOK,
			Message: fmt.Sprintf("Largest paint at %.1fs.", v.LCP/1000)})
	}

	switch {
	case v.CLS > poorCLS:
		checks = append(checks, Check{ID: "vitals.cls_poor", Severity: SeverityError,
			Message: fmt.Sprintf("The layout shifts a lot while loading (%.2f). Under %.1f is good.", v.CLS, goodCLS)})
	case v.CLS > goodCLS:
		checks = append(checks, Check{ID: "vitals.cls_slow", Severity: SeverityWarning,
			Message: fmt.Sprintf("The layout shifts while loading (%.2f). Under %.1f is good.", v.CLS, goodCLS)})
	}

	if v.TTFB > goodTTFB {
		checks = append(checks, Check{ID: "vitals.ttfb_slow", Severity: SeverityWarning,
			Message: fmt.Sprintf("The server took %.0fms to respond. Under %.0fms is good.", v.TTFB, goodTTFB)})
	}

	return checks
}

// Score turns findings into a number, 0-100.
//
// Errors cost more than warnings because they are the ones that lose traffic:
// a missing title or a stray noindex is a different order of problem from a
// long meta description. The floor is 0 -- a negative score would imply a
// page can be worse than nothing.
func Score(checks []Check) int {
	const (
		errorCost   = 15
		warningCost = 5
	)

	score := 100
	for _, check := range checks {
		switch check.Severity {
		case SeverityError:
			score -= errorCost
		case SeverityWarning:
			score -= warningCost
		}
	}

	if score < 0 {
		return 0
	}
	return score
}

// SiteScore averages the page scores.
func SiteScore(pageScores []int) int {
	if len(pageScores) == 0 {
		return 0
	}

	total := 0
	for _, score := range pageScores {
		total += score
	}
	return total / len(pageScores)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
