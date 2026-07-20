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
 */
export function Analytics({ config }: AnalyticsProps) {
  const { backendUrl, siteKey } = config ?? {}

  // Missing configuration renders nothing rather than throwing: analytics must
  // never be the reason a page fails to load.
  //
  // But it says so in production too, and that is deliberate. This used to warn
  // only in development, which meant the one deployment that mattered failed in
  // total silence -- a site statically prerendered without ZENITH_SITE_KEY in
  // the build environment bakes an empty snippet into every page and collects
  // nothing, for weeks, with a dashboard full of zeroes as the only symptom.
  // A build log is exactly where that should have been caught.
  if (!backendUrl || !siteKey) {
    const missing = !backendUrl ? 'backendUrl' : 'siteKey'
    const env = !backendUrl ? 'ZENITH_URL' : 'ZENITH_SITE_KEY'

    console.warn(
      `[zenith] <Analytics /> rendered nothing and no pageviews will be recorded: ` +
        `config is missing ${missing}. Set ${env} and check it is present when this ` +
        `page is rendered — a statically prerendered page reads it at build time, ` +
        `not at run time, so it must be set for the build too. ` +
        `Both values are on the Setup tab of your Zenith console.`,
    )
    return null
  }

  return (
    <script
      {...trackerAttributes(backendUrl, siteKey)}
      dangerouslySetInnerHTML={{ __html: TRACKER_SOURCE }}
    />
  )
}
