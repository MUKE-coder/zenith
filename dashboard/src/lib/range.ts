/** The date ranges the dashboard offers, and what they mean. */

export type RangeKey = '24h' | '7d' | '30d' | '90d' | '12m'

export type Range = {
  key: RangeKey
  label: string
  /** How far back the range starts. */
  duration: number
}

const HOUR = 60 * 60 * 1000
const DAY = 24 * HOUR

export const RANGES: Range[] = [
  { key: '24h', label: 'Last 24 hours', duration: 24 * HOUR },
  { key: '7d', label: 'Last 7 days', duration: 7 * DAY },
  { key: '30d', label: 'Last 30 days', duration: 30 * DAY },
  { key: '90d', label: 'Last 90 days', duration: 90 * DAY },
  { key: '12m', label: 'Last 12 months', duration: 365 * DAY },
]

export const DEFAULT_RANGE: RangeKey = '30d'

export function rangeByKey(key: RangeKey): Range {
  return RANGES.find((r) => r.key === key) ?? RANGES[2]
}

/**
 * Resolves a range to the `from`/`to` the API expects.
 *
 * Both ends are ISO timestamps rather than bare dates: a bare `to` date means
 * "through the end of that day" to the API, which is right for a date picker
 * and wrong for "the last 24 hours".
 */
export function toParams(key: RangeKey, now = new Date()): { from: string; to: string } {
  const range = rangeByKey(key)
  return {
    from: new Date(now.getTime() - range.duration).toISOString(),
    to: now.toISOString(),
  }
}
