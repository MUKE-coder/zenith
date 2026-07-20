// Package storage defines the persistence seams for Zenith.
//
// Analytics events and application data live in different stores because they
// have different shapes: events are append-only and queried with wide
// aggregations, app data is transactional and queried by key. EventStore and
// AppStore keep that split explicit so the hosted tier can swap
// DuckDB->ClickHouse and SQLite->Postgres without business logic changing.
package storage

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned by lookups that match no row.
var ErrNotFound = errors.New("storage: not found")

// ErrConflict is returned when a write collides with an existing row, such as a
// duplicate email or site key.
var ErrConflict = errors.New("storage: already exists")

// Event is a single analytics datapoint: a pageview or a custom event.
//
// VisitorHash is the cookieless daily-rotating identity. It is derived from the
// raw IP and user agent, which are never themselves persisted.
type Event struct {
	SiteID      string
	TS          time.Time
	Type        string // "pageview" | "event"
	Path        string
	Referrer    string
	UTMSource   string
	UTMMedium   string
	UTMCampaign string
	UTMTerm     string
	UTMContent  string
	Country     string // ISO-3166-1 alpha-2, coarse; never finer than country
	Device      string
	Browser     string
	OS          string
	VisitorHash string
	EventName   string
	Props       map[string]any
}

// SessionGap is how long a visitor may be idle before their next event starts
// a new session.
//
// Thirty minutes is the industry convention, which matters more than being
// optimal: it makes Zenith's session counts comparable to everyone else's.
const SessionGap = 30 * time.Minute

// RealtimeWindow is how recently a visitor must have been seen to count as
// active "now".
const RealtimeWindow = 5 * time.Minute

// Query scopes an analytics question to one site and a time range.
//
// SiteID is not optional and there is no query without it. That is the whole
// defense against one client's dashboard showing another's traffic: the store
// cannot express a cross-site question, so no forgotten authorization check
// can ask one.
type Query struct {
	SiteID string

	// From is inclusive, To is exclusive. Half-open ranges mean adjacent
	// periods tile exactly, with no event counted in both or missed between.
	From time.Time
	To   time.Time

	// Limit caps returned rows. Zero means a sensible default.
	Limit int

	// ExcludeReferrer drops a hostname from referrer breakdowns. Set to the
	// site's own domain: internal navigation sets a referrer, and without this
	// a site's top referrer is always itself.
	ExcludeReferrer string
}

// Granularity is a timeseries bucket size.
type Granularity string

// Bucket sizes for the timeseries chart.
const (
	GranularityHour  Granularity = "hour"
	GranularityDay   Granularity = "day"
	GranularityWeek  Granularity = "week"
	GranularityMonth Granularity = "month"
)

// Valid reports whether g is a supported bucket size.
//
// Callers must check this before the value reaches a query: granularity is
// interpolated into SQL as a date_trunc unit, which no bound parameter can
// carry, so the whitelist is what stands between a URL parameter and an
// injection.
func (g Granularity) Valid() bool {
	switch g {
	case GranularityHour, GranularityDay, GranularityWeek, GranularityMonth:
		return true
	}
	return false
}

// Dimension is a column a breakdown can group by.
type Dimension string

// The columns a breakdown may group by.
const (
	DimPath        Dimension = "path"
	DimReferrer    Dimension = "referrer"
	DimCountry     Dimension = "country"
	DimDevice      Dimension = "device"
	DimBrowser     Dimension = "browser"
	DimOS          Dimension = "os"
	DimUTMSource   Dimension = "utm_source"
	DimUTMMedium   Dimension = "utm_medium"
	DimUTMCampaign Dimension = "utm_campaign"
	DimUTMTerm     Dimension = "utm_term"
	DimUTMContent  Dimension = "utm_content"
)

// Valid reports whether d is a column a breakdown may group by.
//
// Same reasoning as Granularity: the dimension becomes a column name in the
// SQL, so only these exact values may ever reach a query.
func (d Dimension) Valid() bool {
	switch d {
	case DimPath, DimReferrer, DimCountry, DimDevice, DimBrowser, DimOS,
		DimUTMSource, DimUTMMedium, DimUTMCampaign, DimUTMTerm, DimUTMContent:
		return true
	}
	return false
}

// Summary is the headline numbers for a period.
type Summary struct {
	Pageviews int64
	Visitors  int64
	Sessions  int64
}

// Bucket is one point on the timeseries chart.
type Bucket struct {
	TS        time.Time
	Pageviews int64
	Visitors  int64
}

// Count is one row of a breakdown: a label and how much traffic it accounts
// for.
type Count struct {
	Label     string
	Visitors  int64
	Pageviews int64
}

// EventStat is one custom event and how often it fired.
type EventStat struct {
	Name     string
	Count    int64
	Visitors int64
}

// PropStat is one custom event property value and how often it appeared.
type PropStat struct {
	Key   string
	Value string
	Count int64
}

// EventStore is the append-only analytics event store, backed by DuckDB.
//
// Every read is scoped by site: there is no method here that can return events
// across site boundaries, so a missing authorization check cannot leak one
// client's traffic into another's dashboard.
type EventStore interface {
	// Insert appends events. Implementations may batch.
	Insert(ctx context.Context, events ...Event) error

	// Summary returns pageviews, unique visitors, and sessions for a period.
	Summary(ctx context.Context, q Query) (Summary, error)

	// Timeseries returns traffic bucketed by g, with empty buckets included as
	// zeroes so a chart shows a gap rather than closing over it.
	Timeseries(ctx context.Context, q Query, g Granularity) ([]Bucket, error)

	// Breakdown groups traffic by one dimension, busiest first.
	Breakdown(ctx context.Context, q Query, d Dimension) ([]Count, error)

	// EntryPages returns the pages sessions started on.
	EntryPages(ctx context.Context, q Query) ([]Count, error)

	// ExitPages returns the pages sessions ended on.
	ExitPages(ctx context.Context, q Query) ([]Count, error)

	// Events returns custom event counts.
	Events(ctx context.Context, q Query) ([]EventStat, error)

	// EventProps returns the property breakdown for one custom event.
	EventProps(ctx context.Context, q Query, name string) ([]PropStat, error)

	// Realtime returns visitors active within the last RealtimeWindow.
	Realtime(ctx context.Context, siteID string) (int64, error)

	// DeleteSite removes every event for a site. It is the event-store half of
	// deleting a site: the app store cascades its own rows, but events are
	// here, and a deleted site's traffic must not linger in aggregate queries.
	DeleteSite(ctx context.Context, siteID string) error

	// Ping verifies the store is reachable.
	Ping(ctx context.Context) error

	// Close releases the underlying handle.
	Close() error
}

// Role is what an account is allowed to see.
type Role string

const (
	// RoleDeveloper can see and manage every site.
	RoleDeveloper Role = "developer"
	// RoleOwner can read exactly one site.
	RoleOwner Role = "owner"
)

// Valid reports whether r is a role the schema will accept.
func (r Role) Valid() bool {
	return r == RoleDeveloper || r == RoleOwner
}

// User is a Zenith account.
type User struct {
	ID           string
	Email        string
	PasswordHash string
	Role         Role
	CreatedAt    time.Time
}

// Site is one tracked website.
//
// The two keys are not interchangeable, and the difference is the security
// model. SiteKey is public -- it ships inside the tracking snippet, so treat it
// as readable by anyone -- and it authorizes writing events to this site only.
// APIKey is secret, lives server-side in the owner's zenith.config.js, and
// authorizes reading this site's analytics. Never send APIKey to a browser.
type Site struct {
	ID         string
	Name       string
	Domain     string
	SiteKey    string
	APIKey     string
	OwnerEmail string

	// DashboardPath is where the owner mounted their dashboard, e.g.
	// "/analytics-dashboard". Only the owner's app knows it, so it is recorded
	// here for the report email's link. Empty means no dashboard, and the link
	// is omitted rather than guessed.
	DashboardPath string

	CreatedAt time.Time
}

// DashboardURL is where this site's owner reads their analytics, or "" when no
// dashboard path is recorded.
func (s Site) DashboardURL() string {
	if s.Domain == "" || s.DashboardPath == "" {
		return ""
	}
	return "https://" + s.Domain + s.DashboardPath
}

// Settings is the deployment's global configuration.
//
// Global, not per-site: v1 sends every site's report through one Resend
// account, because one developer owns every site in a deployment.
type Settings struct {
	// ResendAPIKey is a secret. It is never returned to a client unmasked and
	// never logged.
	ResendAPIKey string

	// MailFrom is the From address, e.g. "Zenith <reports@example.com>".
	MailFrom string

	UpdatedAt time.Time
}

// Configured reports whether email can actually be sent.
func (s Settings) Configured() bool {
	return s.ResendAPIKey != "" && s.MailFrom != ""
}

// Report statuses.
const (
	ReportSent   = "sent"
	ReportFailed = "failed"
)

// Report is one monthly report send.
type Report struct {
	ID     string
	SiteID string

	// Period is the month reported on, as "YYYY-MM".
	Period string

	SentAt time.Time
	Status string

	// Err is why a failed send failed, for the console to show.
	Err string
}

// Audit job statuses.
const (
	AuditQueued  = "queued"
	AuditRunning = "running"
	AuditDone    = "done"
	AuditFailed  = "failed"
)

// AuditJob is one requested site audit.
type AuditJob struct {
	ID     string
	SiteID string
	Status string

	RequestedAt time.Time

	// StartedAt is set when a worker claims the job. It is also how a crashed
	// worker's job is found: still running, started long ago, never finished.
	StartedAt  time.Time
	FinishedAt time.Time

	// Score is the site-wide score, 0-100. Meaningful once Status is done.
	Score int

	// Err is why a failed audit failed, in the interface's voice.
	Err string
}

// AuditResult is one audited page.
type AuditResult struct {
	ID      string
	JobID   string
	PageURL string

	// Checks is the page's findings as JSON. A column per check would mean a
	// migration per check, and the check set is expected to grow.
	Checks string

	Score int
}

// AppStore is the transactional application store, backed by SQLite.
type AppStore interface {
	// Migrate brings the schema up to the current version. It is idempotent.
	Migrate(ctx context.Context) error

	// CreateUser stores a new account. It returns ErrConflict if the email is
	// already registered. PasswordHash must already be hashed.
	CreateUser(ctx context.Context, u User) error

	// UserByEmail looks up an account by email, case-insensitively. It returns
	// ErrNotFound if no account matches.
	UserByEmail(ctx context.Context, email string) (User, error)

	// CountUsers returns the number of accounts. Used to decide whether a
	// deployment still needs its first admin.
	CountUsers(ctx context.Context) (int, error)

	// CreateSite stores a new site. It returns ErrConflict if either key is
	// already taken.
	CreateSite(ctx context.Context, s Site) error

	// ListSites returns every site, oldest first. Developer scope only: there
	// is no caller who should see this list except the person who owns them
	// all.
	ListSites(ctx context.Context) ([]Site, error)

	// UpdateSite changes a site's name, domain, and owner email. The keys are
	// not editable: rotating one is a different operation with different
	// consequences, and it must not be something a typo in a form can do.
	UpdateSite(ctx context.Context, s Site) error

	// DeleteSite removes a site and everything that references it in the app
	// store -- audits, results, reports -- via ON DELETE CASCADE. The site's
	// analytics events live in the event store and must be deleted there
	// separately; see EventStore.DeleteSite.
	DeleteSite(ctx context.Context, id string) error

	// Settings returns the global settings. The row always exists.
	Settings(ctx context.Context) (Settings, error)

	// UpdateSettings replaces the global settings.
	UpdateSettings(ctx context.Context, s Settings) error

	// RecordReport stores the outcome of a monthly send, replacing any earlier
	// outcome for the same site and period. That is what lets a failed send be
	// retried without ever becoming a second email.
	RecordReport(ctx context.Context, r Report) error

	// ReportFor returns a site's report for a period, or ErrNotFound.
	ReportFor(ctx context.Context, siteID, period string) (Report, error)

	// ReportsForSite returns a site's report history, newest first.
	ReportsForSite(ctx context.Context, siteID string) ([]Report, error)

	// CreateAuditJob enqueues an audit. The worker picks it up; this returns
	// immediately, because a full-site render takes minutes and no HTTP
	// request should wait for it.
	CreateAuditJob(ctx context.Context, job AuditJob) error

	// AuditJobByID returns one job, or ErrNotFound.
	AuditJobByID(ctx context.Context, id string) (AuditJob, error)

	// AuditJobsForSite returns a site's audits, newest first.
	AuditJobsForSite(ctx context.Context, siteID string) ([]AuditJob, error)

	// AuditResultsForJob returns a job's per-page findings, worst first: the
	// pages that need work are the reason anyone opened the report.
	AuditResultsForJob(ctx context.Context, jobID string) ([]AuditResult, error)

	// ActiveAuditForSite returns a site's queued or running audit, or
	// ErrNotFound. Used to refuse a second audit of the same site.
	ActiveAuditForSite(ctx context.Context, siteID string) (AuditJob, error)

	// SiteByID looks up a site. It returns ErrNotFound if there is no such
	// site.
	SiteByID(ctx context.Context, id string) (Site, error)

	// SiteBySiteKey resolves a public site key to its site. It returns
	// ErrNotFound for an unknown key.
	//
	// This is on the ingestion hot path: every event resolves a key here.
	SiteBySiteKey(ctx context.Context, siteKey string) (Site, error)

	// SiteByAPIKey resolves a secret api key to its site. It returns
	// ErrNotFound for an unknown key.
	//
	// This is how the domain-native proxy reads a site's analytics without a
	// user session. The key authorizes reading exactly one site.
	SiteByAPIKey(ctx context.Context, apiKey string) (Site, error)

	// RevokeToken adds a token id to the denylist until expiresAt, which is
	// when the token would have expired on its own. Revoking an already
	// revoked token is not an error.
	RevokeToken(ctx context.Context, jti, userID string, expiresAt time.Time) error

	// IsTokenRevoked reports whether a token id has been revoked.
	IsTokenRevoked(ctx context.Context, jti string) (bool, error)

	// DeleteExpiredRevokedTokens drops denylist rows past their expiry, which
	// the signature check would reject anyway. Returns the number removed.
	DeleteExpiredRevokedTokens(ctx context.Context) (int64, error)

	// Ping verifies the store is reachable.
	Ping(ctx context.Context) error

	// Close releases the underlying handle.
	Close() error
}
