/**
 * zenith-analytics — cookieless tracking and a domain-native dashboard.
 *
 * Server-side entry point. It is handed the config, which carries secrets, so
 * nothing here may be imported into browser code.
 */

export { ConfigError, defaultConfig, resolveConfig } from './config.js'
export type { ZenithConfig } from './config.js'

export { createHandler } from './proxy.js'
export type { ZenithHandler } from './proxy.js'

export { generateSecret } from './session.js'
export { TRACKER_SOURCE, trackerAttributes } from './tracker.js'

import { TRACKER_SOURCE, trackerAttributes } from './tracker.js'

/** Kept in step with package.json by hand; nothing reads it but a human. */
export const VERSION = '0.3.0'

/**
 * Props for the tracking script tag.
 *
 * Spread onto a `<script>` in any React app -- no import of this package into
 * the browser, and no React dependency here:
 *
 *     import { trackerScriptProps } from 'zenith-analytics'
 *     import { ZENITH_PUBLIC } from '@/config/zenith'
 *
 *     <script {...trackerScriptProps(ZENITH_PUBLIC)} />
 *
 * Only the public site key is passed. The api key and the signing secret stay
 * on the server, and this function has no way to reach them.
 */
export function trackerScriptProps(config: { backendUrl: string; siteKey: string }): {
  'data-endpoint': string
  'data-site-key': string
  dangerouslySetInnerHTML: { __html: string }
} {
  return {
    ...trackerAttributes(config.backendUrl, config.siteKey),
    dangerouslySetInnerHTML: { __html: TRACKER_SOURCE },
  }
}

/**
 * The tracking snippet as a plain HTML string, for anything that is not React.
 *
 * Inject it into the page's <head>.
 */
export function trackerScriptTag(config: { backendUrl: string; siteKey: string }): string {
  const attrs = trackerAttributes(config.backendUrl, config.siteKey)
  return (
    `<script data-endpoint="${attrs['data-endpoint']}" ` +
    `data-site-key="${attrs['data-site-key']}">${TRACKER_SOURCE}</script>`
  )
}
