import { useEffect, useState } from 'react'

import { useAsync } from '../hooks/useAsync'
import { api } from '../lib/api'
import styles from './Realtime.module.css'

type Props = {
  siteId: string | undefined

  /**
   * Whether the site to read is settled.
   *
   * Not the same as having a siteId: embedded on an owner's domain there is
   * none to have, because the proxy's api key names the site server-side.
   */
  ready: boolean

  onUnauthorized: () => void
}

/** How often the live count refreshes. */
const POLL_MS = 15_000

/**
 * Visitors active right now.
 *
 * Polls rather than streams: one small request every fifteen seconds is far
 * less machinery than a websocket, and this number does not need to be
 * instant to be useful.
 */
export function Realtime({ siteId, ready, onUnauthorized }: Props) {
  const [tick, setTick] = useState(0)

  useEffect(() => {
    if (!ready) return

    const id = setInterval(() => setTick((t) => t + 1), POLL_MS)
    return () => clearInterval(id)
  }, [ready, siteId])

  const { data, error } = useAsync(
    (signal) => api.realtime({ site: siteId }, signal),
    [siteId, tick],
    { enabled: ready, onUnauthorized },
  )

  // A live indicator that shows an error is worse than one that shows nothing.
  if (error || !data) return null

  const live = data.visitors > 0

  return (
    <span
      className={styles.wrap}
      title={`Visitors active in the last ${Math.round(data.window_seconds / 60)} minutes`}
    >
      <span
        className={`${styles.dot} ${live ? styles.live : styles.idle}`}
        aria-hidden="true"
      />
      <span className={styles.count}>{data.visitors}</span>
      <span className={styles.label}>
        {data.visitors === 1 ? 'visitor now' : 'visitors now'}
      </span>
    </span>
  )
}
