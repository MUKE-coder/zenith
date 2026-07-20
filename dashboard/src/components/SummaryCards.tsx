import {
  changeDirection,
  compactNumber,
  duration,
  exactNumber,
  formatChange,
  percent,
  ratio,
} from '../lib/format'
import type { Summary, SummaryMetric } from '../lib/types'
import { Skeleton } from './Panel'
import styles from './SummaryCards.module.css'

type Props = {
  summary: Summary | undefined
  loading: boolean
}

type Metric = {
  key: SummaryMetric
  label: string
  format: (value: number) => string
  /** The exact value on hover, where the shown one is abbreviated. */
  exact?: (value: number) => string
  /**
   * True where a smaller number is the better month. Only bounce rate: for
   * everything else up is good, and painting a rising bounce rate green would
   * congratulate the reader on losing people.
   */
  lowerIsBetter?: boolean
}

const METRICS: Metric[] = [
  { key: 'visitors', label: 'Unique visitors', format: compactNumber, exact: exactNumber },
  { key: 'pageviews', label: 'Pageviews', format: compactNumber, exact: exactNumber },
  { key: 'sessions', label: 'Sessions', format: compactNumber, exact: exactNumber },
  { key: 'views_per_visit', label: 'Views per visit', format: ratio },
  { key: 'bounce_rate', label: 'Bounce rate', format: percent, lowerIsBetter: true },
  { key: 'avg_duration', label: 'Visit duration', format: duration },
]

/** The headline numbers for the period. */
export function SummaryCards({ summary, loading }: Props) {
  return (
    <div className={styles.grid}>
      {METRICS.map((metric) => (
        <Card
          key={metric.key}
          metric={metric}
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
  metric: Metric
  value: number | undefined
  change: number | null | undefined
  previous: number | undefined
  loading: boolean
}

function Card({ metric, value, change, previous, loading }: CardProps) {
  const direction = changeDirection(change)

  // The arrow still points the way the number moved; only the colour flips.
  // An arrow that lied about the direction would be worse than an unhelpful
  // colour.
  const tone =
    metric.lowerIsBetter && (direction === 'up' || direction === 'down')
      ? direction === 'up'
        ? 'down'
        : 'up'
      : direction

  return (
    <article className={styles.card}>
      <p className="eyebrow">{metric.label}</p>

      {loading || value === undefined ? (
        <div style={{ marginTop: 'var(--space-2)' }}>
          <Skeleton width="60%" height={36} />
        </div>
      ) : (
        <p className={styles.value} title={metric.exact?.(value)}>
          {metric.format(value)}
        </p>
      )}

      {change !== undefined && !loading && (
        <span className={`${styles.delta} ${styles[tone]}`}>
          <span aria-hidden="true">{arrow(direction)}</span>
          {formatChange(change)}
          {previous !== undefined && direction !== 'none' && (
            <span className={styles.deltaLabel}>vs {metric.format(previous)}</span>
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
