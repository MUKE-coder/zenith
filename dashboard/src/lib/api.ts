import { isEmbedded, statsUrl } from './embed'
import type {
  Audit,
  AuditDetail,
  AuditsResponse,
  Events,
  Geo,
  Granularity,
  LoginResponse,
  Pages,
  Realtime,
  Referrers,
  ReportsResponse,
  Session,
  Settings,
  Site,
  SitesResponse,
  Summary,
  Tech,
  Timeseries,
} from './types'

/** Where the session token lives between page loads. */
const TOKEN_KEY = 'zenith.token'

/**
 * An error carrying the server's message.
 *
 * The API writes errors in the interface's voice and says how to fix them, so
 * they are shown as-is rather than replaced with something vaguer.
 */
export class ApiError extends Error {
  // Declared explicitly rather than as a constructor parameter property: the
  // build runs with erasableSyntaxOnly, which allows only type syntax that can
  // be stripped without emitting code.
  readonly status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }

  /** True when the session is missing, expired, or revoked. */
  get isUnauthorized(): boolean {
    return this.status === 401
  }
}

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token)
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY)
}

type RequestOptions = {
  method?: string
  body?: unknown
  signal?: AbortSignal
}

async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const headers: Record<string, string> = {}

  // Embedded, the caller is authenticated by the proxy's first-party cookie
  // and there is no token to send. Attaching a stale one from some other
  // origin's localStorage would be noise at best.
  const token = isEmbedded() ? null : getToken()
  if (token) headers['Authorization'] = `Bearer ${token}`
  if (options.body !== undefined) headers['Content-Type'] = 'application/json'

  const res = await fetch(path, {
    method: options.method ?? 'GET',
    headers,
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
    signal: options.signal,
    // Same-origin, so the session cookie rides along automatically.
    credentials: isEmbedded() ? 'same-origin' : 'omit',
  })

  if (!res.ok) {
    throw new ApiError(res.status, await errorMessage(res))
  }

  // 204: logout and collect answer with no body.
  if (res.status === 204) return undefined as T

  return (await res.json()) as T
}

async function errorMessage(res: Response): Promise<string> {
  try {
    const body = (await res.json()) as { error?: string }
    if (body.error) return body.error
  } catch {
    // A non-JSON error body (a proxy timeout, say) is not worth surfacing raw.
  }
  return "Something went wrong. Try again."
}

/** Query string from defined values only, so absent params are truly absent. */
function query(params: Record<string, string | number | boolean | undefined>): string {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== '') search.set(key, String(value))
  }
  const encoded = search.toString()
  return encoded ? `?${encoded}` : ''
}

/** Params common to every stats request. */
export type StatsParams = {
  site?: string
  from?: string
  to?: string
  compare?: boolean
  granularity?: Granularity
  limit?: number
  name?: string
}

export const api = {
  async login(email: string, password: string): Promise<LoginResponse> {
    const res = await request<LoginResponse>('/api/auth/login', {
      method: 'POST',
      body: { email, password },
    })
    setToken(res.token)
    return res
  },

  async logout(): Promise<void> {
    try {
      await request<void>('/api/auth/logout', { method: 'POST' })
    } finally {
      // Drop the token locally even if the server call failed: the session is
      // over as far as this browser is concerned.
      clearToken()
    }
  },

  session: (signal?: AbortSignal) => request<Session>('/api/auth/me', { signal }),

  sites: (signal?: AbortSignal) => request<SitesResponse>('/api/sites', { signal }),

  createSite: (site: { name: string; domain: string; owner_email?: string }) =>
    request<Site>('/api/sites', { method: 'POST', body: site }),

  updateSite: (id: string, changes: { name?: string; domain?: string; owner_email?: string }) =>
    request<Site>(`/api/sites/${id}`, { method: 'PATCH', body: changes }),

  deleteSite: (id: string) => request<void>(`/api/sites/${id}`, { method: 'DELETE' }),

  audits: (siteId: string, signal?: AbortSignal) =>
    request<AuditsResponse>(`/api/audits${query({ site: siteId })}`, { signal }),

  audit: (id: string, signal?: AbortSignal) =>
    request<AuditDetail>(`/api/audits/${id}`, { signal }),

  runAudit: (siteId: string) =>
    request<Audit>('/api/audits', { method: 'POST', body: { site_id: siteId } }),

  settings: (signal?: AbortSignal) => request<Settings>('/api/settings', { signal }),

  updateSettings: (changes: { resend_api_key?: string; mail_from?: string }) =>
    request<Settings>('/api/settings', { method: 'PUT', body: changes }),

  reports: (siteId: string, signal?: AbortSignal) =>
    request<ReportsResponse>(`/api/sites/${siteId}/reports`, { signal }),

  sendTestReport: (siteId: string) =>
    request<{ status: string; sent_to: string }>(`/api/sites/${siteId}/reports/test`, {
      method: 'POST',
    }),

  summary: (p: StatsParams, signal?: AbortSignal) =>
    request<Summary>(statsUrl('summary', query(p)), { signal }),

  timeseries: (p: StatsParams, signal?: AbortSignal) =>
    request<Timeseries>(statsUrl('timeseries', query(p)), { signal }),

  pages: (p: StatsParams, signal?: AbortSignal) =>
    request<Pages>(statsUrl('pages', query(p)), { signal }),

  referrers: (p: StatsParams, signal?: AbortSignal) =>
    request<Referrers>(statsUrl('referrers', query(p)), { signal }),

  geo: (p: StatsParams, signal?: AbortSignal) =>
    request<Geo>(statsUrl('geo', query(p)), { signal }),

  tech: (p: StatsParams, signal?: AbortSignal) =>
    request<Tech>(statsUrl('tech', query(p)), { signal }),

  events: (p: StatsParams, signal?: AbortSignal) =>
    request<Events>(statsUrl('events', query(p)), { signal }),

  realtime: (p: StatsParams, signal?: AbortSignal) =>
    request<Realtime>(statsUrl('realtime', query(p)), { signal }),
}
