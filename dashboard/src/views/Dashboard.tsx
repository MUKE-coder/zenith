import { Suspense, lazy, useMemo, useState } from 'react'

import { BreakdownTable } from '../components/BreakdownTable'
import { Panel, Skeleton } from '../components/Panel'
import { SummaryCards } from '../components/SummaryCards'

// Recharts and d3 are nearly half the bundle, and this page ships to clients'
// own domains. Splitting the chart out lets the shell and the numbers paint
// first, and keeps the charting library out of every view that has no chart.
const TrafficChart = lazy(() =>
  import('../components/TrafficChart').then((m) => ({ default: m.TrafficChart })),
)
import { useAsync } from '../hooks/useAsync'
import { api } from '../lib/api'
import { countryName } from '../lib/format'
import { toParams } from '../lib/range'
import type { RangeKey } from '../lib/range'
import type { Granularity } from '../lib/types'
import styles from '../components/AppShell.module.css'
import chartStyles from '../components/TrafficChart.module.css'

type Props = {
  siteId: string | undefined

  /**
   * Whether the site to read is settled. Embedded on an owner's domain there
   * is no siteId to have: the proxy's api key names the site server-side.
   */
  ready: boolean

  range: RangeKey
  onUnauthorized: () => void
}

/** The granularities offered, and when each makes sense to offer. */
const GRANULARITIES: Granularity[] = ['hour', 'day', 'week', 'month']

export function Dashboard({ siteId, ready, range, onUnauthorized }: Props) {
  // Empty means "let the server pick one that fits the range".
  const [granularity, setGranularity] = useState<Granularity | ''>('')

  // Recomputed when the range changes, not on every render: otherwise `to`
  // moves by a few milliseconds each time and refetches forever.
  const params = useMemo(() => toParams(range), [range])

  const enabled = ready
  const base = { site: siteId, ...params }
  const opts = { enabled, onUnauthorized }

  const summary = useAsync(
    (signal) => api.summary({ ...base, compare: true }, signal),
    [siteId, params.from, params.to],
    opts,
  )

  const timeseries = useAsync(
    (signal) =>
      api.timeseries(
        { ...base, compare: true, granularity: granularity || undefined },
        signal,
      ),
    [siteId, params.from, params.to, granularity],
    opts,
  )

  const pages = useAsync(
    (signal) => api.pages(base, signal),
    [siteId, params.from, params.to],
    opts,
  )

  const referrers = useAsync(
    (signal) => api.referrers(base, signal),
    [siteId, params.from, params.to],
    opts,
  )

  const geo = useAsync(
    (signal) => api.geo(base, signal),
    [siteId, params.from, params.to],
    opts,
  )

  const tech = useAsync(
    (signal) => api.tech(base, signal),
    [siteId, params.from, params.to],
    opts,
  )

  return (
    <>
      <SummaryCards summary={summary.data} loading={summary.loading} />

      <Panel
        title="Traffic"
        action={
          <div className={chartStyles.controls}>
            <label className="visually-hidden" htmlFor="granularity">
              Bucket size
            </label>
            <select
              id="granularity"
              className={chartStyles.select}
              value={granularity}
              onChange={(e) => setGranularity(e.target.value as Granularity | '')}
            >
              <option value="">Auto</option>
              {GRANULARITIES.map((g) => (
                <option key={g} value={g}>
                  By {g}
                </option>
              ))}
            </select>
          </div>
        }
      >
        <Suspense fallback={<Skeleton width="100%" height={240} />}>
          <TrafficChart
            data={timeseries.data}
            loading={timeseries.loading}
            error={timeseries.error}
            onRetry={timeseries.reload}
          />
        </Suspense>
      </Panel>

      <div className={styles.grid2}>
        <Panel title="Top pages">
          <BreakdownTable
            rows={pages.data?.top}
            loading={pages.loading}
            error={pages.error}
            onRetry={pages.reload}
            labelHeader="Page"
            metric="pageviews"
            emptyTitle="No pageviews yet."
            emptyHint="Install the snippet to start collecting."
          />
        </Panel>

        <Panel title="Referrers">
          <BreakdownTable
            rows={referrers.data?.sources}
            loading={referrers.loading}
            error={referrers.error}
            onRetry={referrers.reload}
            labelHeader="Source"
            emptyTitle="No referrers yet."
            emptyHint="Traffic that arrives without a referrer counts as direct."
          />
        </Panel>

        <Panel title="Entry pages">
          <BreakdownTable
            rows={pages.data?.entry}
            loading={pages.loading}
            error={pages.error}
            onRetry={pages.reload}
            labelHeader="Page"
            metric="pageviews"
            emptyTitle="No sessions yet."
          />
        </Panel>

        <Panel title="Exit pages">
          <BreakdownTable
            rows={pages.data?.exit}
            loading={pages.loading}
            error={pages.error}
            onRetry={pages.reload}
            labelHeader="Page"
            metric="pageviews"
            emptyTitle="No sessions yet."
          />
        </Panel>

        <Panel title="Countries">
          <BreakdownTable
            rows={geo.data?.countries}
            loading={geo.loading}
            error={geo.error}
            onRetry={geo.reload}
            labelHeader="Country"
            formatLabel={countryName}
            emptyTitle="No country data."
            emptyHint="Set ZENITH_GEOIP_DB to resolve countries."
          />
        </Panel>

        <Panel title="Campaigns">
          <BreakdownTable
            rows={referrers.data?.utm.campaign}
            loading={referrers.loading}
            error={referrers.error}
            onRetry={referrers.reload}
            labelHeader="Campaign"
            emptyTitle="No campaigns yet."
            emptyHint="Tag links with utm_campaign to see them here."
          />
        </Panel>

        <Panel title="Browsers">
          <BreakdownTable
            rows={tech.data?.browsers}
            loading={tech.loading}
            error={tech.error}
            onRetry={tech.reload}
            labelHeader="Browser"
            emptyTitle="No visitors yet."
          />
        </Panel>

        <Panel title="Devices">
          <BreakdownTable
            rows={tech.data?.devices}
            loading={tech.loading}
            error={tech.error}
            onRetry={tech.reload}
            labelHeader="Device"
            emptyTitle="No visitors yet."
          />
        </Panel>
      </div>
    </>
  )
}
