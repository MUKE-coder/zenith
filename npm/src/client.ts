/**
 * Browser entry point.
 *
 * Safe to import into client code: it touches no config and holds no secrets.
 * It is a thin wrapper over the global the tracking snippet installs.
 */

/** Values a custom event property may hold. */
export type PropValue = string | number | boolean | null

type ZenithGlobal = {
  track: (name: string, props?: Record<string, PropValue>) => void
  q?: Array<[string, Record<string, PropValue>?]>
}

declare global {
  interface Window {
    zenith?: ZenithGlobal
  }
}

/**
 * Records a custom event.
 *
 *     import { track } from 'zenith-analytics/client'
 *     track('signup', { plan: 'pro' })
 *
 * Calls made before the snippet has loaded are queued and sent once it does,
 * so a click handler that fires during hydration is not silently dropped.
 *
 * It never throws. Analytics must not be able to break the page it measures --
 * a failed signup metric is a lost number; a thrown error is a lost customer.
 */
export function track(name: string, props?: Record<string, PropValue>): void {
  if (typeof window === 'undefined') return
  if (!name) return

  try {
    if (window.zenith?.track) {
      window.zenith.track(name, props)
      return
    }

    // The snippet has not run yet. Queue for it to drain.
    const queue = (window.zenith?.q ?? []) as Array<[string, Record<string, PropValue>?]>
    queue.push([name, props])
    window.zenith = { ...(window.zenith ?? {}), q: queue } as ZenithGlobal
  } catch {
    // Nothing an analytics call can fail at is worth breaking a page over.
  }
}
