import { useEffect, useState } from 'react'

import { EmptyState, ErrorState, Panel, SkeletonRows } from '../components/Panel'
import { useAsync } from '../hooks/useAsync'
import { api } from '../lib/api'
import type { AuditCheck, AuditPage, AuditStatus } from '../lib/types'
import styles from './Audits.module.css'

type Props = {
  siteId: string | undefined
  ready: boolean
  onUnauthorized: () => void
}

/** How often a running audit is re-checked. */
const POLL_MS = 4000

export function Audits({ siteId, ready, onUnauthorized }: Props) {
  const [selected, setSelected] = useState<string>()
  const [tick, setTick] = useState(0)
  const [running, setRunning] = useState(false)
  const [error, setError] = useState<string>()

  const audits = useAsync(
    (signal) => api.audits(siteId ?? '', signal),
    [siteId, tick],
    { enabled: ready, onUnauthorized },
  )

  const list = audits.data?.audits ?? []
  const latest = list[0]

  // An audit in flight is the one case where the page has to keep asking:
  // a full-site render takes minutes, and the developer is watching.
  const inFlight = list.some((a) => a.status === 'queued' || a.status === 'running')

  useEffect(() => {
    if (!inFlight) return
    const id = setInterval(() => setTick((t) => t + 1), POLL_MS)
    return () => clearInterval(id)
  }, [inFlight])

  // Follow the newest audit unless the developer picked an older one.
  const showing = selected ?? latest?.id

  const detail = useAsync(
    (signal) => api.audit(showing ?? '', signal),
    [showing, tick],
    { enabled: Boolean(showing), onUnauthorized },
  )

  async function run() {
    if (!siteId) return
    setRunning(true)
    setError(undefined)

    try {
      await api.runAudit(siteId)
      setSelected(undefined)
      setTick((t) => t + 1)
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Something went wrong.')
    } finally {
      setRunning(false)
    }
  }

  return (
    <div className={styles.stack}>
      <Panel
        title="SEO audit"
        action={
          <div className={styles.runRow}>
            {inFlight && (
              <span className={styles.status}>
                <span className={styles.spinner} aria-hidden="true" />
                {latest?.status === 'running' ? 'Auditing…' : 'Queued…'}
              </span>
            )}
            <button
              type="button"
              className="button-primary"
              onClick={run}
              disabled={running || inFlight || !ready}
            >
              Run audit
            </button>
          </div>
        }
      >
        {error && <p className={styles.failed}>{error}</p>}

        {audits.data?.worker_hint && !error && (
          // A developer who never started the worker would otherwise watch
          // "queued" forever with no explanation.
          <p className={styles.failed}>{audits.data.worker_hint}</p>
        )}

        {audits.error ? (
          <ErrorState message={audits.error} onRetry={audits.reload} />
        ) : audits.loading ? (
          <SkeletonRows rows={2} />
        ) : list.length === 0 ? (
          <EmptyState
            title="No audits yet."
            hint="Run one to check titles, links, structured data, and page speed."
          />
        ) : (
          <History
            audits={list}
            showing={showing}
            onSelect={(id) => setSelected(id)}
          />
        )}
      </Panel>

      {showing && detail.data && <AuditReport detail={detail.data} />}
    </div>
  )
}

function History({
  audits,
  showing,
  onSelect,
}: {
  audits: { id: string; status: AuditStatus; requested_at: string; score: number }[]
  showing: string | undefined
  onSelect: (id: string) => void
}) {
  return (
    <div className={styles.history}>
      {audits.slice(0, 8).map((audit) => (
        <button
          key={audit.id}
          type="button"
          className={`${styles.historyRow} ${audit.id === showing ? styles.historyActive : ''}`}
          onClick={() => onSelect(audit.id)}
          aria-current={audit.id === showing}
        >
          <span style={{ flex: 1 }}>{new Date(audit.requested_at).toLocaleString()}</span>
          <span>{statusLabel(audit.status)}</span>
          {audit.status === 'done' && <ScorePill score={audit.score} />}
        </button>
      ))}
    </div>
  )
}

function statusLabel(status: AuditStatus): string {
  switch (status) {
    case 'queued':
      return 'Queued'
    case 'running':
      return 'Running'
    case 'failed':
      return 'Failed'
    default:
      return 'Done'
  }
}

function AuditReport({ detail }: { detail: { audit: { status: AuditStatus; score: number; error?: string }; pages: AuditPage[] } }) {
  const { audit, pages } = detail

  if (audit.status === 'failed') {
    return (
      <Panel title="Result">
        {/* The worker's own words: they name what to fix. */}
        <p className={styles.failed}>{audit.error ?? 'The audit failed.'}</p>
      </Panel>
    )
  }

  if (audit.status !== 'done') {
    return (
      <Panel title="Result">
        <EmptyState
          title="The audit is still running."
          hint="Rendering every page with a real browser takes a few minutes."
        />
      </Panel>
    )
  }

  return (
    <Panel title="Result">
      <div className={styles.siteScore}>
        <span className={styles.siteScoreValue}>{audit.score}</span>
        <span style={{ color: 'var(--text-muted)' }}>
          site score across {pages.length} {pages.length === 1 ? 'page' : 'pages'}
        </span>
      </div>

      <div style={{ marginTop: 'var(--space-4)' }}>
        {pages.map((page) => (
          <PageRow key={page.url} page={page} />
        ))}
      </div>
    </Panel>
  )
}

function PageRow({ page }: { page: AuditPage }) {
  // The worst pages are the reason anyone opened this, so they start open.
  const [open, setOpen] = useState(page.score < 90)

  const issues = page.checks.checks.filter((c) => c.severity !== 'ok')
  const errors = issues.filter((c) => c.severity === 'error').length
  const warnings = issues.filter((c) => c.severity === 'warning').length

  const path = pathOf(page.url)

  return (
    <div className={styles.page}>
      <button
        type="button"
        className={styles.pageHeader}
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
      >
        <span className={`${styles.chevron} ${open ? styles.chevronOpen : ''}`} aria-hidden="true">
          ▶
        </span>
        <ScorePill score={page.score} />
        <span className={styles.pageURL} title={page.url}>
          {path}
        </span>
        <span className={styles.counts}>
          {errors > 0 && <span className={styles.countError}>{errors} error{errors === 1 ? '' : 's'}</span>}
          {warnings > 0 && <span className={styles.countWarning}>{warnings} warning{warnings === 1 ? '' : 's'}</span>}
          {issues.length === 0 && <span style={{ color: 'var(--positive)' }}>No issues</span>}
        </span>
      </button>

      {open && (
        <div className={styles.issues}>
          {issues.map((check) => (
            <Issue key={check.id} check={check} />
          ))}
          {issues.length === 0 && (
            <p style={{ color: 'var(--text-muted)', fontSize: 'var(--text-label)' }}>
              Nothing to fix on this page.
            </p>
          )}
          <Vitals vitals={page.checks.vitals} />
        </div>
      )}
    </div>
  )
}

function Issue({ check }: { check: AuditCheck }) {
  const color = check.severity === 'error' ? 'var(--negative)' : 'var(--warning)'

  return (
    <div className={styles.issue}>
      {/* Severity is the semantic color and nothing else — never decorative. */}
      <span className={styles.severity} style={{ color }}>
        {check.severity === 'error' ? 'Error' : 'Warning'}
      </span>
      <div className={styles.issueBody}>
        <div>{check.message}</div>
        {check.detail && <div className={styles.issueDetail}>{check.detail}</div>}
      </div>
    </div>
  )
}

function Vitals({ vitals }: { vitals: AuditPage['checks']['vitals'] }) {
  return (
    <div className={styles.vitals}>
      <Vital label="LCP" value={ms(vitals.lcp_ms)} />
      <Vital label="FCP" value={ms(vitals.fcp_ms)} />
      <Vital label="TTFB" value={ms(vitals.ttfb_ms)} />
      <Vital label="CLS" value={vitals.cls.toFixed(3)} />
      <Vital label="Load" value={ms(vitals.load_ms)} />
    </div>
  )
}

function Vital({ label, value }: { label: string; value: string }) {
  return (
    <span className={styles.vital}>
      {label} <span className={styles.vitalValue}>{value}</span>
    </span>
  )
}

function ms(value: number): string {
  if (!value) return '—'
  if (value < 1000) return `${Math.round(value)}ms`
  return `${(value / 1000).toFixed(1)}s`
}

/** Green >= 90, amber 50-89, red < 50. */
function ScorePill({ score }: { score: number }) {
  const tone = score >= 90 ? styles.good : score >= 50 ? styles.mixed : styles.poor
  return <span className={`${styles.pill} ${tone}`}>{score}</span>
}

function pathOf(url: string): string {
  try {
    const parsed = new URL(url)
    return parsed.pathname + parsed.search
  } catch {
    return url
  }
}
