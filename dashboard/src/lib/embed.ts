/**
 * Embedded mode: the dashboard running on the owner's own domain.
 *
 * The npm proxy renders the page and declares this global. When it is present,
 * the SPA is a page of someone else's site: it talks to that site's origin,
 * has no login of its own (the proxy's password gate already happened), and
 * never mentions the other sites the developer manages.
 */

export type EmbedConfig = {
  /** Where the proxy is mounted, e.g. "/analytics-dashboard". */
  basePath: string

  /** The owner's domain, so the page can name their site instead of ours. */
  siteDomain?: string

  embedded: true
}

declare global {
  interface Window {
    __ZENITH__?: EmbedConfig
  }
}

export function embedConfig(): EmbedConfig | undefined {
  if (typeof window === 'undefined') return undefined
  return window.__ZENITH__
}

export function isEmbedded(): boolean {
  return embedConfig()?.embedded === true
}

/**
 * The URL for a stats endpoint.
 *
 * Embedded, requests go to the owner's own origin under the mount, and the
 * proxy attaches the api key server-side. In the console they go to Zenith
 * directly with a session token. The two differ because the credentials do.
 */
export function statsUrl(endpoint: string, query: string): string {
  const embed = embedConfig()
  if (embed) return `${embed.basePath}/api/${endpoint}${query}`
  return `/api/stats/${endpoint}${query}`
}

/**
 * The URL for an audit endpoint.
 *
 * Same split as statsUrl, and it needs its own because audits do not live
 * under /api/stats. Embedded, the path collapses to the mount: `audits` and
 * `audits/<id>` are both names the proxy knows and maps back to /api/audits.
 * Without this the owner's SEO tab asked their own site for /api/audits, a
 * route only Zenith has -- which is why it showed nothing.
 */
export function auditsUrl(path: string, query = ''): string {
  const embed = embedConfig()
  if (embed) return `${embed.basePath}/api/audits${path}${query}`
  return `/api/audits${path}${query}`
}
