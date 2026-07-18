import { useMemo, useState } from 'react'

import { BreakdownTable } from '../components/BreakdownTable'
import { IconCursor, IconEvents } from '../components/icons'
import { EmptyState, ErrorState, Panel, SkeletonRows } from '../components/Panel'
import { useAsync } from '../hooks/useAsync'
import { api } from '../lib/api'
import { toParams } from '../lib/range'
import type { RangeKey } from '../lib/range'
import type { Count } from '../lib/types'
import styles from '../components/AppShell.module.css'

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

export function Events({ siteId, ready, range, onUnauthorized }: Props) {
  const [selected, setSelected] = useState<string>()

  const params = useMemo(() => toParams(range), [range])
  const enabled = ready

  const events = useAsync(
    (signal) => api.events({ site: siteId, ...params }, signal),
    [siteId, params.from, params.to],
    { enabled, onUnauthorized },
  )

  const props = useAsync(
    (signal) => api.events({ site: siteId, ...params, name: selected }, signal),
    [siteId, params.from, params.to, selected],
    { enabled: enabled && Boolean(selected), onUnauthorized },
  )

  // Custom events counts reuse the breakdown table by mapping to its shape:
  // "how many times" is the lead number, "how many people" the secondary.
  const eventRows: Count[] | undefined = events.data?.events.map((e) => ({
    label: e.name,
    pageviews: e.count,
    visitors: e.visitors,
  }))

  const propRows: Count[] | undefined = props.data?.props?.map((p) => ({
    label: `${p.key}: ${p.value}`,
    pageviews: p.count,
    visitors: p.count,
  }))

  return (
    <div className={styles.grid2}>
      <Panel title="Custom events">
        {events.error ? (
          <ErrorState message={events.error} onRetry={events.reload} />
        ) : events.loading ? (
          <SkeletonRows rows={4} />
        ) : !eventRows || eventRows.length === 0 ? (
          <EmptyState
            icon={<IconEvents />}
            title="No custom events yet"
            hint="Pageviews arrive on their own. Custom events are yours to name — call track('signup') anywhere in your app and it shows up here."
          />
        ) : (
          <EventList
            rows={eventRows}
            selected={selected}
            onSelect={(name) => setSelected(name === selected ? undefined : name)}
          />
        )}
      </Panel>

      <Panel title={selected ? `Properties of “${selected}”` : 'Properties'}>
        {!selected ? (
          <EmptyState
            icon={<IconCursor />}
            title="Select an event"
            hint="Pick one on the left to see the properties it carried and how often each value appeared."
          />
        ) : (
          <BreakdownTable
            rows={propRows}
            loading={props.loading}
            error={props.error}
            onRetry={props.reload}
            labelHeader="Property"
            metric="pageviews"
            emptyTitle="This event carries no properties."
            emptyHint="Pass an object to zenith.track to add some."
            limit={12}
          />
        )}
      </Panel>
    </div>
  )
}

/** The event list doubles as a selector for the properties panel. */
function EventList({
  rows,
  selected,
  onSelect,
}: {
  rows: Count[]
  selected: string | undefined
  onSelect: (name: string) => void
}) {
  return (
    <table style={{ width: '100%', borderCollapse: 'collapse' }}>
      <tbody>
        {rows.map((row) => (
          <tr key={row.label}>
            <td style={{ padding: 0 }}>
              <button
                type="button"
                onClick={() => onSelect(row.label)}
                aria-pressed={row.label === selected}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                  gap: 'var(--space-4)',
                  width: '100%',
                  padding: 'var(--space-3) var(--space-2)',
                  background: row.label === selected ? 'var(--surface-2)' : 'transparent',
                  border: 'none',
                  borderRadius: 'var(--radius-button)',
                  cursor: 'pointer',
                  fontFamily: 'inherit',
                  fontSize: 'var(--text-body)',
                  color: row.label === selected ? 'var(--accent)' : 'var(--text)',
                  textAlign: 'left',
                }}
              >
                <span>{row.label}</span>
                <span
                  className="mono"
                  style={{ color: 'var(--text-muted)', fontSize: 'var(--text-data)' }}
                >
                  {row.pageviews.toLocaleString()}
                </span>
              </button>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
