import type { Granularity } from './types'

/**
 * Formats a metric for a summary card.
 *
 * Big numbers are abbreviated because the card is the glance: "1.2M" reads
 * instantly where "1,238,411" has to be counted. The exact value stays
 * available in the element's title.
 */
export function compactNumber(n: number): string {
  if (n < 1000) return String(n)
  if (n < 10_000) return trimZero((n / 1000).toFixed(1)) + 'K'
  if (n < 1_000_000) return Math.round(n / 1000) + 'K'
  if (n < 10_000_000) return trimZero((n / 1_000_000).toFixed(1)) + 'M'
  return Math.round(n / 1_000_000) + 'M'
}

/** Full precision, grouped. For tooltips and titles. */
export function exactNumber(n: number): string {
  return n.toLocaleString()
}

function trimZero(s: string): string {
  return s.endsWith('.0') ? s.slice(0, -2) : s
}

/**
 * Formats a percent change for a delta chip.
 *
 * Null means the previous period was zero, which has no percentage. The
 * interface says "New" rather than inventing 0% or +100%.
 */
export function formatChange(change: number | null | undefined): string {
  if (change === null || change === undefined) return 'New'
  const sign = change > 0 ? '+' : ''
  return `${sign}${change}%`
}

export type ChangeDirection = 'up' | 'down' | 'flat' | 'none'

/**
 * Which way a delta points.
 *
 * Direction is about the number, not whether it is good news: for exit rates
 * "up" is bad. Zenith's metrics are all "more is better", so up is positive
 * here, but the caller decides the color.
 */
export function changeDirection(change: number | null | undefined): ChangeDirection {
  if (change === null || change === undefined) return 'none'
  if (change > 0) return 'up'
  if (change < 0) return 'down'
  return 'flat'
}

/** A chart axis label, at the precision the bucket size implies. */
export function formatBucket(iso: string, granularity: Granularity): string {
  const date = new Date(iso)

  switch (granularity) {
    case 'hour':
      return date.toLocaleTimeString(undefined, { hour: 'numeric' })
    case 'day':
      return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
    case 'week':
      return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
    case 'month':
      return date.toLocaleDateString(undefined, { month: 'short', year: '2-digit' })
  }
}

/** A full timestamp, for a tooltip where the axis label is too terse. */
export function formatBucketFull(iso: string, granularity: Granularity): string {
  const date = new Date(iso)

  if (granularity === 'hour') {
    return date.toLocaleString(undefined, {
      month: 'short',
      day: 'numeric',
      hour: 'numeric',
    })
  }
  return date.toLocaleDateString(undefined, {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })
}

/** A country code as a readable name, falling back to the code itself. */
export function countryName(code: string): string {
  try {
    const names = new Intl.DisplayNames(undefined, { type: 'region' })
    return names.of(code.toUpperCase()) ?? code
  } catch {
    return code
  }
}

/** A share of a total, for the bar behind a breakdown row. */
export function share(value: number, total: number): number {
  if (total <= 0) return 0
  return Math.min(100, (value / total) * 100)
}

/** A rate held as 0-1, shown as a whole percent. */
export function percent(rate: number): string {
  return `${Math.round(rate * 100)}%`
}

/**
 * A duration in seconds, as a person would say it.
 *
 * Seconds below a minute, minutes and seconds below an hour, hours and minutes
 * above. Never "0.0h": the point of this is to be read at a glance.
 */
export function duration(seconds: number): string {
  const total = Math.max(0, Math.round(seconds))

  if (total < 60) return `${total}s`

  const minutes = Math.floor(total / 60)
  if (minutes < 60) {
    const rest = total % 60
    return rest === 0 ? `${minutes}m` : `${minutes}m ${rest}s`
  }

  const hours = Math.floor(minutes / 60)
  const rest = minutes % 60
  return rest === 0 ? `${hours}h` : `${hours}h ${rest}m`
}

/** A small average, e.g. views per visit. Two significant-ish digits. */
export function ratio(value: number): string {
  return value.toFixed(2)
}
