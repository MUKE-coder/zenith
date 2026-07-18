import { useId } from 'react'
import {
  Area,
  CartesianGrid,
  ComposedChart,
  Line,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'

import { useThemeColors } from '../hooks/useThemeColors'
import { compactNumber, exactNumber, formatBucket, formatBucketFull } from '../lib/format'
import type { Granularity, Timeseries } from '../lib/types'
import { IconChart } from './icons'
import { EmptyState, ErrorState, Skeleton } from './Panel'
import styles from './TrafficChart.module.css'

type Props = {
  data: Timeseries | undefined
  loading: boolean
  error: string | undefined
  onRetry: () => void
}

type Point = {
  ts: string
  visitors: number
  pageviews: number
  previous?: number
  isCurrent?: boolean
}

/**
 * The daily traffic ridgeline: the signature.
 *
 * A crisp single-hue line with a faint accent glow beneath, reading like an
 * altitude profile climbing toward a peak. Everything around it stays quiet,
 * which is what lets this one element carry the product's character.
 */
export function TrafficChart({ data, loading, error, onRetry }: Props) {
  const colors = useThemeColors()

  // Unique per instance: two charts on one page must not share a gradient id.
  const glowId = useId()

  if (error) return <ErrorState message={error} onRetry={onRetry} />

  if (loading || !data) {
    return (
      <div className={styles.wrap} style={{ display: 'grid', placeItems: 'center' }}>
        <Skeleton width="100%" height={240} />
      </div>
    )
  }

  const points = toPoints(data)

  if (points.every((p) => p.pageviews === 0 && p.visitors === 0)) {
    return (
      <EmptyState
        icon={<IconChart />}
        title="No traffic in this period"
        hint="Nothing was recorded in this range. Try a wider one — or if this site is newly added, check the snippet is live on the page."
      />
    )
  }

  const current = points.find((p) => p.isCurrent)

  return (
    <div className={styles.wrap}>
      <ResponsiveContainer width="100%" height="100%">
        <ComposedChart data={points} margin={{ top: 8, right: 8, bottom: 0, left: -16 }}>
          <defs>
            {/* The glow: a single hue fading to nothing, never a heavy fill. */}
            <linearGradient id={glowId} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={colors.accent} stopOpacity={0.18} />
              <stop offset="100%" stopColor={colors.accent} stopOpacity={0} />
            </linearGradient>
          </defs>

          {/* Faint horizontals only. Vertical gridlines are noise on a
              time series the axis already labels. */}
          <CartesianGrid stroke={colors.border} strokeDasharray="0" vertical={false} />

          <XAxis
            dataKey="ts"
            tickFormatter={(ts: string) => formatBucket(ts, data.granularity)}
            stroke={colors.subtle}
            tick={{ fill: colors.muted, fontSize: 12 }}
            tickLine={false}
            axisLine={false}
            minTickGap={24}
          />
          <YAxis
            stroke={colors.subtle}
            tick={{ fill: colors.muted, fontSize: 12 }}
            tickLine={false}
            axisLine={false}
            width={48}
            tickFormatter={compactNumber}
            allowDecimals={false}
          />

          <Tooltip
            content={<ChartTooltip granularity={data.granularity} colors={colors} />}
            cursor={{ stroke: colors.border, strokeWidth: 1 }}
          />

          {/* The comparison series recedes: muted, dashed, behind the primary. */}
          {data.previous && (
            <Line
              type="monotone"
              dataKey="previous"
              stroke={colors.subtle}
              strokeWidth={1.5}
              strokeDasharray="3 3"
              dot={false}
              activeDot={false}
              isAnimationActive={false}
            />
          )}

          <Area
            type="monotone"
            dataKey="visitors"
            stroke={colors.accent}
            strokeWidth={2}
            fill={`url(#${glowId})`}
            dot={false}
            activeDot={{ r: 3, fill: colors.accent, stroke: colors.surface, strokeWidth: 2 }}
            isAnimationActive={false}
          />

          {/* The current bucket, marked with a subtle vertical tick. */}
          {current && (
            <ReferenceLine
              x={current.ts}
              stroke={colors.accent}
              strokeOpacity={0.35}
              strokeWidth={1}
            />
          )}
        </ComposedChart>
      </ResponsiveContainer>
    </div>
  )
}

/**
 * Aligns the two series by position, not by timestamp.
 *
 * The previous period's buckets carry their own (earlier) timestamps, so they
 * are matched to the current period index by index: the nth bucket of each.
 * That is what makes "this Monday vs last Monday" line up on one axis.
 */
function toPoints(data: Timeseries): Point[] {
  const now = Date.now()

  // The current bucket is the last one that has actually started.
  let currentIndex = -1
  for (let i = 0; i < data.buckets.length; i++) {
    if (new Date(data.buckets[i].ts).getTime() <= now) currentIndex = i
  }

  return data.buckets.map((bucket, i) => ({
    ts: bucket.ts,
    visitors: bucket.visitors,
    pageviews: bucket.pageviews,
    previous: data.previous?.[i]?.visitors,
    isCurrent: i === currentIndex,
  }))
}

type TooltipProps = {
  active?: boolean
  payload?: Array<{ payload: Point }>
  granularity: Granularity
  colors: ReturnType<typeof useThemeColors>
}

function ChartTooltip({ active, payload, granularity, colors }: TooltipProps) {
  if (!active || !payload?.length) return null

  const point = payload[0].payload

  return (
    <div className={styles.tooltip}>
      <p className={styles.tooltipLabel}>{formatBucketFull(point.ts, granularity)}</p>

      <div className={styles.tooltipRow}>
        <span className={styles.tooltipKey}>
          <span className={styles.swatch} style={{ background: colors.accent }} />
          Visitors
        </span>
        <span className={styles.tooltipValue}>{exactNumber(point.visitors)}</span>
      </div>

      <div className={styles.tooltipRow}>
        <span className={styles.tooltipKey}>Pageviews</span>
        <span className={styles.tooltipValue}>{exactNumber(point.pageviews)}</span>
      </div>

      {point.previous !== undefined && (
        <div className={styles.tooltipRow}>
          <span className={styles.tooltipKey}>
            <span className={styles.swatch} style={{ background: colors.subtle }} />
            Previous
          </span>
          <span className={styles.tooltipValue}>{exactNumber(point.previous)}</span>
        </div>
      )}
    </div>
  )
}
