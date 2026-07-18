import { TRACKER_SOURCE, trackerAttributes } from './tracker.js'

/**
 * What `<Analytics />` needs. Your `zenith.config.js` already satisfies it, so
 * hand the whole object over -- only these two fields are ever read.
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
 *     import config from '../zenith.config.js'
 *
 *     export default function RootLayout({ children }) {
 *       return (
 *         <html lang="en">
 *           <body>
 *             {children}
 *             <Analytics config={config} />
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
 * 2. `zenith.config.js` also holds `apiKey` and `jwtSecret`. Rendered on the
 *    server, the object stays there and only the two public values below reach
 *    the page. Passed into a `'use client'` component, React would serialize
 *    every field it was given into the browser payload -- secrets included.
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
