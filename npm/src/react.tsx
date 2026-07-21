import { TRACKER_SOURCE, trackerAttributes } from './tracker.js'

/**
 * What `<Analytics />` needs. The `ZENITH_PUBLIC` export in `config/zenith.ts`
 * is exactly this shape, so hand that over -- these two fields are the only
 * ones ever read, and both are public.
 */
export type AnalyticsProps = {
  config: {
    backendUrl: string
    siteKey: string
  }

  /**
   * Fail loudly instead of quietly when the config is missing.
   *
   * By default a missing site key renders nothing and warns, because analytics
   * must never be the reason a page fails. But that warning is only useful if
   * someone reads the log, and the failure it describes -- a prerendered page
   * built without the key, collecting nothing -- is easy to miss for weeks.
   *
   * Set this on a site you know must track, and a missing key throws instead.
   * On a statically prerendered page that turns `next build` red, which is
   * where the mistake should surface. Opt in per site; the default stays safe.
   */
  required?: boolean
}

/**
 * The drop-in tracking component.
 *
 * Render it once in your root layout and pageviews -- including client-side
 * route changes -- are recorded from then on:
 *
 *     import { Analytics } from 'zenith-analytics/next'
 *     import { ZENITH_PUBLIC } from '@/config/zenith'
 *
 *     export default function RootLayout({ children }) {
 *       return (
 *         <html lang="en">
 *           <body>
 *             {children}
 *             <Analytics config={ZENITH_PUBLIC} />
 *           </body>
 *         </html>
 *       )
 *     }
 *
 * **Render it on the server**, which a layout does by default. Two separate
 * reasons, one rule:
 *
 * 1. The snippet is inlined into the HTML and finds its own configuration
 *    through `document.currentScript`, which the browser sets only while
 *    executing a script it parsed. A script inserted later by client-side
 *    React is never run.
 *
 * 2. Rendered on the server, whatever object you pass stays there and only the
 *    two public values below reach the page. Passed into a `'use client'`
 *    component, React would serialize every field it was given into the browser
 *    payload -- which is why `ZENITH_PUBLIC` exists to be passed here, rather
 *    than the full config that also carries `apiKey` and `jwtSecret`.
 *
 * Nothing here can read a secret even if handed one: it takes `backendUrl` and
 * `siteKey`, and the site key is public by design -- it ships in the page and
 * authorizes writing events, nothing more.
 *
 * On a site that must not ship blind, add `required` and a missing key throws
 * at render instead of warning -- which fails the build of a prerendered page:
 *
 *     <Analytics config={ZENITH_PUBLIC} required />
 */
export function Analytics({ config, required }: AnalyticsProps) {
  const { backendUrl, siteKey } = config ?? {}

  // Missing configuration renders nothing rather than throwing: analytics must
  // never be the reason a page fails to load -- unless the caller asked it to
  // with `required`, in which case it throws and, on a prerendered page, turns
  // the build red where the mistake belongs.
  //
  // Either way it speaks in production, which is deliberate. This used to warn
  // only in development, the one deployment where the mistake cannot happen --
  // a site statically prerendered without ZENITH_SITE_KEY in the build
  // environment bakes an empty snippet into every page and collects nothing,
  // for weeks, with a dashboard full of zeroes as the only symptom.
  if (!backendUrl || !siteKey) {
    const missing = !backendUrl ? 'backendUrl' : 'siteKey'
    const env = !backendUrl ? 'ZENITH_URL' : 'ZENITH_SITE_KEY'

    const message =
      `<Analytics /> has no ${missing}, so no pageviews will be recorded. ` +
      `Set ${env} and make sure it is present when this page is rendered — a ` +
      `statically prerendered page reads it at build time, not at run time, so ` +
      `it must be set for the build too. Both values are on the Setup tab of ` +
      `your Zenith console.`

    // The opt-in strict path: a site the caller declared must track should not
    // be allowed to ship silently blind.
    if (required) {
      throw new Error(`[zenith] ${message}`)
    }

    console.warn(`[zenith] ${message}`)
    return null
  }

  return (
    <script
      {...trackerAttributes(backendUrl, siteKey)}
      dangerouslySetInnerHTML={{ __html: TRACKER_SOURCE }}
    />
  )
}
