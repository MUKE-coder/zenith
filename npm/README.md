# zenith-analytics

Privacy-first analytics for your site — **cookieless tracking** and a dashboard that lives on
**your own domain**.

This is the client package for [Zenith](https://github.com/MUKE-coder/zenith), a self-hosted
analytics and SEO platform. You install it in your project; it talks to a Zenith service you (or
your developer) run.

- **Cookieless.** A tiny (~1 KB) tracking snippet, no cookies, no fingerprinting — so no consent
  banner.
- **Domain-native dashboard.** A server-side proxy mounts an analytics page on *your* domain,
  e.g. `yoursite.com/analytics-dashboard`, password-protected and same-origin. No foreign
  subdomain, no CORS.
- **First-class Next.js support**, App Router and Pages Router, with a framework-agnostic core
  beneath.

## Install

```sh
npm install zenith-analytics
npx zenith init      # scaffolds zenith.config.js and the dashboard route
npx zenith hash      # generates a bcrypt hash for the dashboard password
```

Fill in the keys (from your Zenith console → **Add site**) in `zenith.config.js`:

```js
module.exports = {
  backendUrl: process.env.ZENITH_URL,       // your Zenith service
  siteKey:    process.env.ZENITH_SITE_KEY,   // public — ships in the snippet
  apiKey:     process.env.ZENITH_API_KEY,    // secret — server-side only
  dashboardPath: '/analytics-dashboard',
  protected: true,
  passwordHash: process.env.ZENITH_PW_HASH,  // from `npx zenith hash`
  jwtSecret:    process.env.ZENITH_JWT_SECRET,
  siteDomain:   'example.com',
}
```

## Track pageviews

Drop the component into your root layout:

```tsx
import { Analytics } from 'zenith-analytics/next'
import config from '../zenith.config.js'

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>
        {children}
        <Analytics config={config} />
      </body>
    </html>
  )
}
```

That's it. Pageviews — including client-side route changes — are recorded from
then on.

**Render it on the server**, which a layout does by default. Two reasons, one
rule: the snippet is inlined into the HTML and finds its configuration through
`document.currentScript`, so a script inserted later by client-side React would
never run; and `zenith.config.js` also holds `apiKey` and `jwtSecret`, which
stay on the server as long as the component does. Inside a `'use client'`
component, React would serialize every field you passed into the browser
payload.

The component itself reads only `backendUrl` and `siteKey`, and the site key is
public by design — it ships in the page and authorizes writing events, nothing
more.

Not using React, or want the script tag yourself? `trackerScriptProps(config)`
returns the props, and `trackerScriptTag(config)` returns plain HTML.

## Custom events

```ts
import { track } from 'zenith-analytics/client'

track('signup', { plan: 'pro' })
```

`zenith-analytics/client` is browser-safe — it holds no secrets and never throws, so a failed
analytics call can't break the page it measures.

## The domain-native dashboard (Next.js)

**App Router** — `app/analytics-dashboard/[[...zenith]]/route.ts`:

```ts
import { createZenithRoute } from 'zenith-analytics/next'
import config from '../../../zenith.config.js'

export const { GET, POST } = createZenithRoute(config)
export const dynamic = 'force-dynamic'
```

**Pages Router** — `pages/api/zenith/[[...zenith]].ts`, plus a rewrite in `next.config.js`.
`npx zenith init` scaffolds whichever router it detects, with the exact wiring.

Your client visits `yoursite.com/analytics-dashboard`, enters the password, and sees their
analytics — on their own domain. The proxy reads the data server-side with the secret `apiKey`,
which never reaches the browser.

## The two keys

| Key       | Visibility                      | Authorizes                    |
|-----------|---------------------------------|-------------------------------|
| `siteKey` | Public — ships in the snippet   | Writing events only           |
| `apiKey`  | Secret — server-side only       | Reading that site's analytics |

Treat `siteKey` as readable by anyone (it's in your page source). Keep `apiKey` and `jwtSecret`
out of client code and out of git.

## Entry points

- `zenith-analytics` — server-side: config, the proxy handler, tracker-script helpers. Reads
  `zenith.config.js`, so **do not** import it into browser code.
- `zenith-analytics/client` — browser-safe: `track()`. No config, no secrets.
- `zenith-analytics/next` — Next.js App Router and Pages Router adapters.

## License

MIT. Part of [Zenith](https://github.com/MUKE-coder/zenith).
