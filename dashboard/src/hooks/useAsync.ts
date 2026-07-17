import { useCallback, useEffect, useState } from 'react'

import { ApiError } from '../lib/api'

export type AsyncState<T> = {
  data: T | undefined
  error: string | undefined
  /** True until the first result arrives; a refetch does not flip it back. */
  loading: boolean
  reload: () => void
}

/**
 * Runs an async function and tracks its state.
 *
 * Deliberately small: the dashboard reads a handful of endpoints on a page it
 * already knows how to refresh, which does not warrant a caching library.
 *
 * Every request gets an AbortSignal and is cancelled on unmount or when the
 * dependencies change, so switching sites quickly cannot let a stale response
 * land after the one the user is waiting for.
 */
export function useAsync<T>(
  fn: (signal: AbortSignal) => Promise<T>,
  deps: unknown[],
  options: { enabled?: boolean; onUnauthorized?: () => void } = {},
): AsyncState<T> {
  const { enabled = true, onUnauthorized } = options

  const [data, setData] = useState<T>()
  const [error, setError] = useState<string>()
  const [loading, setLoading] = useState(enabled)
  const [nonce, setNonce] = useState(0)

  const reload = useCallback(() => setNonce((n) => n + 1), [])

  useEffect(() => {
    if (!enabled) {
      setLoading(false)
      return
    }

    const controller = new AbortController()
    let cancelled = false

    setError(undefined)

    fn(controller.signal)
      .then((result) => {
        if (cancelled) return
        setData(result)
        setLoading(false)
      })
      .catch((err: unknown) => {
        if (cancelled || controller.signal.aborted) return

        // An expired or revoked session is not a panel-level error: the whole
        // app has to go back to the login screen.
        if (err instanceof ApiError && err.isUnauthorized && onUnauthorized) {
          onUnauthorized()
          return
        }

        setError(err instanceof Error ? err.message : 'Something went wrong.')
        setLoading(false)
      })

    return () => {
      cancelled = true
      controller.abort()
    }
    // fn is intentionally not a dependency: it is redefined every render, and
    // the caller's deps say when the request actually changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...deps, enabled, nonce])

  return { data, error, loading, reload }
}
