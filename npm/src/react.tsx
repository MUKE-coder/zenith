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

  // Missing configuration renders nothing rather than throwing. Analytics must
  // never be the reason a page fails to load -- but in development it says so,
  // because silently collecting nothing is its own kind of bug.
  if (!backendUrl || !siteKey) {
    if (typeof process !== 'undefined' && process.env?.NODE_ENV !== 'production') {
      console.warn(
        `[zenith] <Analytics /> rendered nothing: config is missing ${
          !backendUrl ? 'backendUrl' : 'siteKey'
        }. Both are on the Setup tab of your Zenith console.`,
      )
    }
    return null
  }

  return (
    <script
      {...trackerAttributes(backendUrl, siteKey)}
      dangerouslySetInnerHTML={{ __html: TRACKER_SOURCE }}
    />
  )
}
