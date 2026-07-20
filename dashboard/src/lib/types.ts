/** Shapes returned by the Zenith API. */

export type Role = 'developer' | 'owner'

export type Session = {
  role: Role
  site_id?: string
  expires_at: string
}

export type LoginResponse = {
  token: string
  expires_at: string
  role: Role
  email: string
}

export type Site = {
  id: string
  name: string
  domain: string
  site_key: string
  api_key: string
  owner_email?: string

  /** Where the owner mounted their dashboard, e.g. "/analytics-dashboard". */
  dashboard_path?: string

  /** That path against the domain. Absent when no path is set. */
  dashboard_url?: string

  created_at: string
}

export type SitesResponse = { sites: Site[] }

export type Summary = {
  pageviews: number
  visitors: number
  sessions: number
  previous?: { pageviews: number; visitors: number; sessions: number }
  /** Null per metric when the previous period was zero: growth from nothing has no percentage. */
  change?: {
    pageviews: number | null
    visitors: number | null
    sessions: number | null
  }
}

export type Granularity = 'hour' | 'day' | 'week' | 'month'

export type Bucket = {
  ts: string
  pageviews: number
  visitors: number
}

export type Timeseries = {
  granularity: Granularity
  buckets: Bucket[]
  previous?: Bucket[]
}

/** One row of a breakdown: a label and the traffic it accounts for. */
export type Count = {
  label: string
  visitors: number
  pageviews: number
}

export type Pages = {
  top: Count[]
  entry: Count[]
  exit: Count[]
}

export type Referrers = {
  sources: Count[]
  utm: {
    source: Count[]
    medium: Count[]
    campaign: Count[]
    term: Count[]
    content: Count[]
  }
}

export type Geo = { countries: Count[] }

export type Tech = {
  devices: Count[]
  browsers: Count[]
  os: Count[]
}

export type EventStat = {
  name: string
  count: number
  visitors: number
}

export type PropStat = {
  key: string
  value: string
  count: number
}

export type Events = {
  events: EventStat[]
  props?: PropStat[]
}

export type Realtime = {
  visitors: number
  window_seconds: number
}

export type Settings = {
  /** Whether a Resend key is stored. The key itself never leaves the server. */
  resend_configured: boolean
  /** A fixed mask when a key is set, absent otherwise. Never the real key. */
  resend_api_key?: string
  mail_from: string
  /** Whether monthly reports can actually go out. */
  email_ready: boolean
}

export type Report = {
  period: string
  status: 'sent' | 'failed'
  sent_at?: string
  error?: string
}

export type ReportsResponse = { reports: Report[] }

export type SendReportResult = {
  status: string
  sent_to: string

  /** The month the analytics report covered, as "YYYY-MM". */
  period?: string

  analytics: boolean
  seo: boolean

  /** A partial success, e.g. the SEO report skipped for want of an audit. */
  note?: string
}

export type AuditStatus = 'queued' | 'running' | 'done' | 'failed'

export type Audit = {
  id: string
  site_id: string
  status: AuditStatus
  requested_at: string
  started_at?: string
  finished_at?: string
  score: number
  error?: string
}

export type AuditsResponse = {
  audits: Audit[]
  /** Set when jobs are queued but nothing is consuming them. */
  worker_hint?: string
}

export type AuditSeverity = 'error' | 'warning' | 'ok'

export type AuditCheck = {
  id: string
  severity: AuditSeverity
  message: string
  detail?: string
}

export type AuditVitals = {
  ttfb_ms: number
  fcp_ms: number
  lcp_ms: number
  cls: number
  dcl_ms: number
  load_ms: number
}

export type AuditPage = {
  url: string
  score: number
  checks: {
    checks: AuditCheck[]
    vitals: AuditVitals
    title?: string
    description?: string
  }
}

export type AuditDetail = {
  audit: Audit
  pages: AuditPage[]
}
