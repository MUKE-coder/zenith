import { changeDirection, compactNumber, exactNumber, formatChange } from '../lib/format'
import type { Summary } from '../lib/types'
import { Skeleton } from './Panel'
import styles from './SummaryCards.module.css'

type Props = {
  summary: Summary | undefined
  loading: boolean
}

/** The three numbers the whole product exists to report. */
export function SummaryCards({ summary, loading }: Props) {
  const metrics = [
    { key: 'visitors', label: 'Unique visitors' },
    { key: 'pageviews', label: 'Pageviews' },
    { key: 'sessions', label: 'Sessions' },
  ] as const

  return (
    <div className={styles.grid}>
      {metrics.map((metric) => (
        <Card
          key={metric.key}
          label={metric.label}
          value={summary?.[metric.key]}
          change={summary?.change?.[metric.key]}
          previous={summary?.previous?.[metric.key]}
          loading={loading}
        />
      ))}
    </div>
  )
}

type CardProps = {
  label: string
  value: number | undefined
  change: number | null | undefined
  previous: number | undefined
  loading: boolean
}

function Card({ label, value, change, previous, loading }: CardProps) {
  const direction = changeDirection(change)

  return (
    <article className={styles.card}>
      <p className="eyebrow">{label}</p>

      {loading || value === undefined ? (
        <div style={{ marginTop: 'var(--space-2)' }}>
          <Skeleton width="60%" height={36} />
        </div>
      ) : (
        // The card shows an abbreviated number; the exact one is a hover away.
        <p className={styles.value} title={exactNumber(value)}>
          {compactNumber(value)}
        </p>
      )}

      {change !== undefined && !loading && (
        <span className={`${styles.delta} ${styles[direction]}`}>
          <span aria-hidden="true">{arrow(direction)}</span>
          {formatChange(change)}
          {previous !== undefined && direction !== 'none' && (
            <span className={styles.deltaLabel}>vs {compactNumber(previous)}</span>
          )}
        </span>
      )}
    </article>
  )
}

function arrow(direction: ReturnType<typeof changeDirection>): string {
  switch (direction) {
    case 'up':
      return '↑'
    case 'down':
      return '↓'
    case 'flat':
      return '→'
    // "New" carries no direction: there was nothing to compare against.
    case 'none':
      return ''
  }
}
