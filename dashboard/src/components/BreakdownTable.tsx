import { exactNumber, share } from '../lib/format'
import type { Count } from '../lib/types'
import styles from './BreakdownTable.module.css'
import { EmptyState, ErrorState, SkeletonRows } from './Panel'

type Props = {
  rows: Count[] | undefined
  loading: boolean
  error?: string
  onRetry?: () => void

  /** Column header for the label, e.g. "Page" or "Source". */
  labelHeader: string

  /** Which number leads. Pages read as pageviews; sources read as visitors. */
  metric?: 'visitors' | 'pageviews'

  /** Rewrites a label for display, e.g. a country code to a country name. */
  formatLabel?: (label: string) => string

  emptyTitle: string
  emptyHint?: string
  limit?: number
}

/**
 * A dense but breathable table: hairline dividers, mono right-aligned numbers.
 *
 * Every row carries a faint share bar behind its label, so the shape of the
 * distribution reads at a glance before any number is actually read.
 */
export function BreakdownTable({
  rows,
  loading,
  error,
  onRetry,
  labelHeader,
  metric = 'visitors',
  formatLabel,
  emptyTitle,
  emptyHint,
  limit = 8,
}: Props) {
  if (error) return <ErrorState message={error} onRetry={onRetry} />
  if (loading || !rows) return <SkeletonRows rows={5} />
  if (rows.length === 0) return <EmptyState title={emptyTitle} hint={emptyHint} />

  const shown = rows.slice(0, limit)

  // The bar is a share of the busiest row, not of the total: with a long tail,
  // shares of the total are all hairlines and the panel says nothing.
  const max = Math.max(...shown.map((r) => r[metric]), 1)

  const secondary = metric === 'visitors' ? 'pageviews' : 'visitors'

  return (
    <table className={styles.table}>
      <thead>
        <tr className={styles.headRow}>
          <th scope="col">{labelHeader}</th>
          <th scope="col" className={styles.numeric}>
            {metric === 'visitors' ? 'Visitors' : 'Views'}
          </th>
          <th scope="col" className={styles.numeric}>
            {secondary === 'visitors' ? 'Visitors' : 'Views'}
          </th>
        </tr>
      </thead>
      <tbody>
        {shown.map((row) => (
          <tr key={row.label} className={styles.row}>
            <td className={styles.labelCell}>
              <span
                className={styles.bar}
                style={{ width: `calc(${share(row[metric], max)}% + var(--space-2))` }}
                aria-hidden="true"
              />
              <span className={styles.label} title={row.label}>
                {formatLabel ? formatLabel(row.label) : row.label}
              </span>
            </td>
            <td className={styles.numeric}>{exactNumber(row[metric])}</td>
            <td className={`${styles.numeric} ${styles.secondary}`}>
              {exactNumber(row[secondary])}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
