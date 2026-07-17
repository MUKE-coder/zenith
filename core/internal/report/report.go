// Package report compiles and renders the monthly email report.
package report

import (
	"context"
	"fmt"
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
	TopCountries []storage.Count

	// DashboardURL is where the owner reads the full picture, on their own
	// domain. Empty if the site has no dashboard configured.
	DashboardURL string
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

// Build compiles a site's report for a period.
func Build(ctx context.Context, events storage.EventStore, site storage.Site, period string) (Data, error) {
	span, err := rangeFor(period)
	if err != nil {
		return Data{}, err
	}

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

	// The month before the one being reported, for the comparison.
	prior := query
	prior.To = span.start
	prior.From = span.start.AddDate(0, -1, 0)

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

	// "UG" means nothing to the client this email is written for. The console
	// resolves codes in the browser with Intl; an email has no JavaScript, so
	// it is resolved here.
	for i, c := range countries {
		countries[i].Label = countryName(c.Label)
	}

	return Data{
		SiteName:    site.Name,
		SiteDomain:  site.Domain,
		Period:      period,
		PeriodLabel: span.start.Format("January 2006"),
		Summary:     summary,
		Previous:    previous,
		Change: Change{
			Pageviews: percentChange(previous.Pageviews, summary.Pageviews),
			Visitors:  percentChange(previous.Visitors, summary.Visitors),
			Sessions:  percentChange(previous.Sessions, summary.Sessions),
		},
		TopPages:     pages,
		TopReferrers: referrers,
		TopCountries: countries,
	}, nil
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
