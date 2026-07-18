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

Pageviews arrive on their own. Anything else you name yourself:

```ts
import { track } from 'zenith-analytics/client'

track('signup')
track('signup', { plan: 'pro', source: 'pricing' })
```

Properties are optional and arrive as strings. They're for slicing an event — plan, tier,
variant — not for storing text: keep them short and low-cardinality, or the breakdown becomes
one row per event.

`zenith-analytics/client` is browser-safe — it holds no secrets and never throws, so a failed
analytics call can't break the page it measures. Calls made before the snippet has loaded are
queued and sent once it does, so you don't have to care about ordering.

## The domain-native dashboard (Next.js)

**App Router** — `app/analytics-dashboard/[[...zenith]]/route.ts`:

```ts
import { createZenithRoute } from 'zenith-analytics/next'
import config from '../../../zenith.config.js'

export const { GET, POST } = createZenithRoute(config)
export const dynamic = 'force-dynamic'
```

`force-dynamic` matters: without it Next may statically render the route at build time and
serve every visitor the same cached page — which for a password gate means serving one
client's dashboard to whoever asks.

**Pages Router** — `pages/api/zenith/[[...zenith]].ts`:

```ts
import { createZenithApiRoute } from 'zenith-analytics/next'
import config from '../../../zenith.config.js'

export default createZenithApiRoute(config)
export const config = { api: { bodyParser: false } }
```

plus a rewrite in `next.config.js`, so the URL your client sees is their own:

```js
async rewrites() {
  return [
    { source: '/analytics-dashboard', destination: '/api/zenith' },
    { source: '/analytics-dashboard/:path*', destination: '/api/zenith/:path*' },
  ]
}
```

`bodyParser: false` matters: the handler reads the password form itself, and Next's parser
would consume the stream first, leaving nothing to read.

`npx zenith init` scaffolds whichever router it detects, with the exact wiring.

Your client visits `yoursite.com/analytics-dashboard`, enters the password, and sees their
analytics — on their own domain. The proxy reads the data server-side with the secret `apiKey`,
which never reaches the browser.

### Setting the password

```sh
npx zenith hash     # prompts, prints a bcrypt hash
```

Paste it into `passwordHash`. Never store a plaintext password. The password is verified
against that hash **in your app** — the Zenith service never learns it. A correct password
mints a signed, HttpOnly, first-party cookie lasting `sessionTtl` (12 hours by default).

To change it, run `npx zenith hash` again and replace the value. To publish the dashboard with
no gate at all, set `protected: false`.

### Two dashboards, two passwords

Easy to confuse, so worth stating plainly:

| | Your console | Your client's dashboard |
|---|---|---|
| **Where** | `your-zenith-server/dashboard/` | `theirsite.com/analytics-dashboard` |
| **Signs in with** | Email + password | A password only |
| **Set by** | `ZENITH_ADMIN_EMAIL` / `ZENITH_ADMIN_PASSWORD` on the server | `passwordHash` in `zenith.config.js` |
| **Sees** | Every site you manage | Exactly one site, read-only |

This package only configures the second one.

## zenith.config.js

Read server-side only. Holds two secrets — keep it out of git (`npx zenith init` adds it to
`.gitignore`).

| Field | Required | Default | What it is |
|---|---|---|---|
| `backendUrl` | ✓ | — | Your Zenith service, e.g. `https://zenith.example.com` |
| `siteKey` | ✓ | — | Public key — ships in the page |
| `apiKey` | ✓ | — | Secret key — reads analytics, server-side only |
| `siteDomain` | ✓ | — | The site being measured, e.g. `example.com` |
| `dashboardPath` | — | `/analytics-dashboard` | Where the dashboard mounts |
| `protected` | — | `true` | Password-gate the dashboard |
| `passwordHash` | if protected | — | bcrypt hash from `npx zenith hash` |
| `jwtSecret` | if protected | — | Signs the dashboard cookie, 32+ chars |
| `sessionTtl` | — | `43200` (12h) | Dashboard session length, in seconds |

Every value is validated at startup, and each failure says how to fix it — a missing hash, a
plaintext password where a hash belongs, or a `siteKey` and `apiKey` that are the same value
(easy to transpose, and catastrophic: it would put the secret key in every visitor's page).

## CLI

```sh
npx zenith init     # scaffold zenith.config.js + the dashboard route for your router
npx zenith hash     # generate a bcrypt hash for the dashboard password
```

`init` detects App Router vs Pages Router and writes the right wiring for it.

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
- `zenith-analytics/next` — `<Analytics />` plus the App Router and Pages Router adapters.
- `zenith-analytics/react` — `<Analytics />` on its own, for a React app that isn't Next.js.

## The Zenith service

This package talks to a Zenith service you run. That side is configured with environment
variables, not with `zenith.config.js` — `ZENITH_JWT_SECRET` (the only required one),
`ZENITH_ADMIN_EMAIL` / `ZENITH_ADMIN_PASSWORD`, `ZENITH_PORT`, `ZENITH_DATA_DIR`,
`ZENITH_TOKEN_TTL`, and the rest. Every variable and its default is tabulated in the
**[Zenith README](https://github.com/MUKE-coder/zenith#core)**.

Two features live entirely on that side, with nothing to install here:

- **Monthly reports.** Give a site an owner email in the console and that owner is emailed
  last month's numbers on the 1st. Set the Resend API key under Settings first.
- **SEO audits.** On demand, the optional audit worker opens every page in a real browser and
  reports titles, meta descriptions, heading order, broken links, structured data, and Core
  Web Vitals. The result downloads as Markdown or JSON.

Two things that surprise people:

- **Country data is opt-in.** Countries need a lookup database that can't be redistributed
  with Zenith, so you download a free country `.mmdb` ([DB-IP Lite](https://db-ip.com/db/download/ip-to-country-lite)
  needs no account) and set `ZENITH_GEOIP_DB` to its path. Without it everything works and
  country reads `Unknown`. Country is resolved when an event is recorded, so adding the
  database later doesn't backfill —
  [full steps](https://github.com/MUKE-coder/zenith#country-data-geoip).
- **The email API key isn't an environment variable.** Resend's key and MAIL FROM are set in
  the console under Settings and stored in the database.

## License

MIT. Part of [Zenith](https://github.com/MUKE-coder/zenith).
