import { RANGES } from '../lib/range'
import type { RangeKey } from '../lib/range'
import styles from './RangePicker.module.css'

type Props = {
  value: RangeKey
  onChange: (key: RangeKey) => void
}

/** Short labels for the control; the full ones are the accessible names. */
const SHORT: Record<RangeKey, string> = {
  '24h': '24h',
  '7d': '7d',
  '30d': '30d',
  '90d': '90d',
  '12m': '12m',
}

export function RangePicker({ value, onChange }: Props) {
  return (
    <div className={styles.wrap} role="group" aria-label="Date range">
      {RANGES.map((range) => (
        <button
          key={range.key}
          type="button"
          className={`${styles.option} ${range.key === value ? styles.active : ''}`}
          aria-pressed={range.key === value}
          // The button reads "30d"; a screen reader gets "Last 30 days".
          aria-label={range.label}
          onClick={() => onChange(range.key)}
        >
          {SHORT[range.key]}
        </button>
      ))}
    </div>
  )
}
