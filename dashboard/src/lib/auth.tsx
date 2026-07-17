import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'

import { ApiError, api, clearToken, getToken } from './api'
import { embedConfig, isEmbedded } from './embed'
import type { Session } from './types'

type AuthState = {
  session: Session | undefined
  /** True until we know whether the stored token is still good. */
  checking: boolean
  signIn: (email: string, password: string) => Promise<void>
  signOut: () => Promise<void>
  /** Drops the session locally, for when the server says a token is dead. */
  expire: () => void
}

const AuthContext = createContext<AuthState | undefined>(undefined)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Session>()
  const [checking, setChecking] = useState(true)

  // A stored token may have expired or been revoked while the tab was closed,
  // so it is verified against the server rather than trusted on sight.
  useEffect(() => {
    // Embedded, the proxy's password gate already ran and its cookie is the
    // session. There is no token to check and no console login to fall back
    // to: reaching this code at all means the proxy let us through.
    if (isEmbedded()) {
      setSession({ role: 'owner', expires_at: '' })
      setChecking(false)
      return
    }

    if (!getToken()) {
      setChecking(false)
      return
    }

    const controller = new AbortController()
    let cancelled = false

    api
      .session(controller.signal)
      .then((s) => {
        if (!cancelled) setSession(s)
      })
      .catch((err: unknown) => {
        if (cancelled || controller.signal.aborted) return
        if (err instanceof ApiError && err.isUnauthorized) clearToken()
      })
      .finally(() => {
        if (!cancelled) setChecking(false)
      })

    return () => {
      cancelled = true
      controller.abort()
    }
  }, [])

  const signIn = useCallback(async (email: string, password: string) => {
    const res = await api.login(email, password)
    setSession({ role: res.role, expires_at: res.expires_at })
  }, [])

  const signOut = useCallback(async () => {
    const embed = embedConfig()
    if (embed) {
      // The session is the proxy's HttpOnly cookie, which script cannot clear.
      // Only the server that set it can.
      window.location.href = `${embed.basePath}/logout`
      return
    }

    await api.logout()
    setSession(undefined)
  }, [])

  const expire = useCallback(() => {
    const embed = embedConfig()
    if (embed) {
      // Embedded, an expired session means the proxy's cookie lapsed. Reload
      // and let the gate ask again -- there is no login screen here to show.
      window.location.href = embed.basePath
      return
    }

    clearToken()
    setSession(undefined)
  }, [])

  const value = useMemo(
    () => ({ session, checking, signIn, signOut, expire }),
    [session, checking, signIn, signOut, expire],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used inside AuthProvider')
  return ctx
}
