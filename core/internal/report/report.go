// Package report compiles and renders the monthly email report.
package report

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zenith/core/internal/storage"
)

// Rows is how many entries each table in the report shows.
//
// An email is a glance, not a console. The top few lines answer "how did last
// month go"; the rest is what the dashboard is for.
const Rows = 5

// Data is everything the template renders.
type Data struct {
	SiteName   string
	SiteDomain string

	// Period is the month reported on, as "YYYY-MM".
	Period string

	// PeriodLabel is that month for a human, e.g. "July 2026".
	PeriodLabel string

	Summary  storage.Summary
	Previous storage.Summary

	// Change is the percent change per metric, nil where the previous month
	// was zero and there is no percentage to state.
	Change Change

	TopPages     []storage.Count
	TopReferrers []storage.Count
	TopCountries []CountryRow
	TopDevices   []storage.Count

	// CompareLabel names what Change is measured against, e.g. "last month".
	// A month-to-date report compares against the same days of the previous
	// month, and saying "vs last month" there would overstate it.
	CompareLabel string

	// SentBy is the domain the mail went out from, for the footer. A client
	// who half-remembers the email looks for who sent it, not for Zenith.
	SentBy string

	// DashboardURL is where the owner reads the full picture, on their own
	// domain. Empty if the site has no dashboard path recorded.
	DashboardURL string
}

// CountryRow is one country's traffic.
//
// The code is kept alongside the resolved name because the email shows both:
// "UG" is meaningless to the client, but it makes a scanned list line up and
// tells a reader who does know the codes what they are looking at.
type CountryRow struct {
	Code     string
	Name     string
	Visitors int64
}

// Change is the month-over-month movement.
type Change struct {
	Pageviews *float64
	Visitors  *float64
	Sessions  *float64
}

// Period returns the "YYYY-MM" of the month before now.
//
// That is what a report sent on the 1st covers: the month that just finished.
func Period(now time.Time) string {
	return periodRange(now).start.Format("2006-01")
}

type monthRange struct {
	start time.Time
	end   time.Time
}

// periodRange returns the half-open range covering the month before now.
func periodRange(now time.Time) monthRange {
	utc := now.UTC()

	// The first instant of this month, then step back one month. Going via the
	// first of the month avoids the classic AddDate(0,-1,0) trap: on the 31st,
	// "one month ago" is March 3rd, not February.
	thisMonth := time.Date(utc.Year(), utc.Month(), 1, 0, 0, 0, 0, time.UTC)
	return monthRange{start: thisMonth.AddDate(0, -1, 0), end: thisMonth}
}

// rangeFor returns the half-open range for a "YYYY-MM" period.
func rangeFor(period string) (monthRange, error) {
	start, err := time.Parse("2006-01", period)
	if err != nil {
		return monthRange{}, fmt.Errorf("report: bad period %q, want YYYY-MM", period)
	}
	start = start.UTC()
	return monthRange{start: start, end: start.AddDate(0, 1, 0)}, nil
}

// Window is the stretch of time a report covers.
//
// Two shapes exist. The scheduled report covers a whole finished month. A
// report sent on demand usually wants the month in progress, which is not a
// month yet -- so the window carries its own label and comparison wording
// rather than every caller re-deriving them.
type Window struct {
	// Period is the "YYYY-MM" this belongs to, and the key report history is
	// recorded under.
	Period string

	// Label is the period for a human, e.g. "July 2026" or "July 2026 so far".
	Label string

	// CompareLabel names what the previous window is, for the delta captions.
	CompareLabel string

	span monthRange
}

// FullMonth is the window for a finished month, given as "YYYY-MM".
func FullMonth(period string) (Window, error) {
	span, err := rangeFor(period)
	if err != nil {
		return Window{}, err
	}
	return Window{
		Period:       period,
		Label:        span.start.Format("January 2006"),
		CompareLabel: "last month",
		span:         span,
	}, nil
}

// MonthToDate is the window for the month in progress, up to now.
//
// This is what an on-demand send wants: a site added this month has no
// finished month to report on, and mailing a client a page of zeroes for a
// month that predates their site is worse than useless.
func MonthToDate(now time.Time) Window {
	utc := now.UTC()
	start := time.Date(utc.Year(), utc.Month(), 1, 0, 0, 0, 0, time.UTC)

	return Window{
		Period:       start.Format("2006-01"),
		Label:        start.Format("January 2006") + " so far",
		CompareLabel: "the same days last month",
		span:         monthRange{start: start, end: utc},
	}
}

// Build compiles a site's report for a finished month, given as "YYYY-MM".
func Build(ctx context.Context, events storage.EventStore, site storage.Site, period string) (Data, error) {
	window, err := FullMonth(period)
	if err != nil {
		return Data{}, err
	}
	return BuildWindow(ctx, events, site, window)
}

// BuildWindow compiles a site's report for an arbitrary window.
func BuildWindow(ctx context.Context, events storage.EventStore, site storage.Site, w Window) (Data, error) {
	span := w.span

	query := storage.Query{
		SiteID:          site.ID,
		From:            span.start,
		To:              span.end,
		Limit:           Rows,
		ExcludeReferrer: site.Domain,
	}

	summary, err := events.Summary(ctx, query)
	if err != nil {
		return Data{}, fmt.Errorf("report: summary: %w", err)
	}

	// The equivalent stretch a month earlier. Shifting both ends by one month
	// gives the previous whole month for a full-month window, and the same run
	// of days for a month-to-date one -- comparing the first 20 days of July
	// against the first 20 of June, rather than against all of June.
	prior := query
	prior.From = span.start.AddDate(0, -1, 0)
	prior.To = span.end.AddDate(0, -1, 0)

	// AddDate normalizes overflow, so the 31st of a month following a 30-day
	// one would spill into the reported month itself. Clamp it back.
	if limit := prior.From.AddDate(0, 1, 0); prior.To.After(limit) {
		prior.To = limit
	}

	previous, err := events.Summary(ctx, prior)
	if err != nil {
		return Data{}, fmt.Errorf("report: previous summary: %w", err)
	}

	pages, err := events.Breakdown(ctx, query, storage.DimPath)
	if err != nil {
		return Data{}, fmt.Errorf("report: pages: %w", err)
	}
	referrers, err := events.Breakdown(ctx, query, storage.DimReferrer)
	if err != nil {
		return Data{}, fmt.Errorf("report: referrers: %w", err)
	}
	countries, err := events.Breakdown(ctx, query, storage.DimCountry)
	if err != nil {
		return Data{}, fmt.Errorf("report: countries: %w", err)
	}
	devices, err := events.Breakdown(ctx, query, storage.DimDevice)
	if err != nil {
		return Data{}, fmt.Errorf("report: devices: %w", err)
	}

	// "UG" means nothing to the client this email is written for. The console
	// resolves codes in the browser with Intl; an email has no JavaScript, so
	// it is resolved here -- keeping the code, which the layout shows beside
	// the name.
	countryRows := make([]CountryRow, 0, len(countries))
	for _, c := range countries {
		countryRows = append(countryRows, CountryRow{
			Code:     c.Label,
			Name:     countryName(c.Label),
			Visitors: c.Visitors,
		})
	}

	return Data{
		SiteName:    site.Name,
		SiteDomain:  site.Domain,
		Period:      w.Period,
		PeriodLabel: w.Label,
		Summary:     summary,
		Previous:    previous,
		Change: Change{
			Pageviews: percentChange(previous.Pageviews, summary.Pageviews),
			Visitors:  percentChange(previous.Visitors, summary.Visitors),
			Sessions:  percentChange(previous.Sessions, summary.Sessions),
		},
		TopPages:     pages,
		TopReferrers: referrers,
		TopCountries: countryRows,
		TopDevices:   devices,
		CompareLabel: w.CompareLabel,
		DashboardURL: site.DashboardURL(),
	}, nil
}

// SenderDomain reduces a MAIL FROM to the domain it sends from.
//
// The footer names who sent the mail, and a client half-remembering it looks
// for the domain they recognise, not for Zenith. Handles both bare addresses
// and the "Name <a@b.c>" form; anything unparseable yields "", and the footer
// falls back to naming Zenith.
func SenderDomain(mailFrom string) string {
	from := strings.TrimSpace(mailFrom)

	if i := strings.LastIndex(from, "<"); i != -1 {
		from = strings.TrimSuffix(from[i+1:], ">")
	}

	_, domain, found := strings.Cut(strings.TrimSpace(from), "@")
	if !found {
		return ""
	}
	return strings.TrimSpace(domain)
}

// percentChange returns the change from before to now, or nil when there is no
// meaningful percentage -- growth from zero is not a percentage.
func percentChange(before, now int64) *float64 {
	if before == 0 {
		return nil
	}
	change := (float64(now) - float64(before)) / float64(before) * 100
	rounded := float64(int64(change*10)) / 10
	return &rounded
}
