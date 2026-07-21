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
npx zenith init      # scaffolds config/zenith.ts and the dashboard route
npx zenith hash      # generates a bcrypt hash for the dashboard password
```

`init` writes `config/zenith.ts` (under `src/` if your routes live there). Set `siteDomain` there
— **the file is safe to commit**, because the secrets are not in it:

```ts
import type { ZenithConfig } from 'zenith-analytics'

// Public by design: the site key ships in the snippet on every page.
export const ZENITH_PUBLIC = {
  backendUrl: process.env.ZENITH_URL || 'https://zenith.example.com',
  siteKey: process.env.ZENITH_SITE_KEY || '',
}

export const ZENITH_CONFIG: Partial<ZenithConfig> = {
  ...ZENITH_PUBLIC,
  apiKey: process.env.ZENITH_API_KEY,
  dashboardPath: '/analytics-dashboard',
  protected: true,
  passwordHash: process.env.ZENITH_PW_HASH,
  jwtSecret: process.env.ZENITH_JWT_SECRET,
  siteDomain: 'example.com',
}
```

The three secrets come from the deployment environment and nowhere else:

```sh
ZENITH_API_KEY=zk_...        # reads your analytics — from the console → Add site
ZENITH_PW_HASH=$2b$10$...    # from `npx zenith hash`
ZENITH_JWT_SECRET=...        # any long random string; `npx zenith init` prints one
```

Without them the tracker still runs and the dashboard answers 503 — see
[the dashboard](#the-domain-native-dashboard-nextjs).

## Track pageviews

Drop the component into your root layout:

```tsx
import { Analytics } from 'zenith-analytics/next'
import { ZENITH_PUBLIC } from '@/config/zenith'

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>
        {children}
        <Analytics config={ZENITH_PUBLIC} />
      </body>
    </html>
  )
}
```

That's it. Pageviews — including client-side route changes — are recorded from
then on.

> [!IMPORTANT]
> **`ZENITH_SITE_KEY` is read when the page renders, and a prerendered page
> renders during `next build`.** If the key is only set at run time — a Docker
> `env_file`, a compose `environment:` block — the build sees nothing, an empty
> snippet is baked into every page, and no pageviews are ever recorded. The
> only symptom is a dashboard of zeroes. Set it as a build argument too. See
> [Deploying](#deploying).

Since 0.4.1 the component warns in the build log when it renders with no key,
naming the missing variable — so a build that would have shipped blind says so.
On a site that must not ship blind at all, make it fatal:

```tsx
<Analytics config={ZENITH_PUBLIC} required />
```

`required` throws when the key is missing instead of warning, which fails the
build of a prerendered page rather than letting it deploy silent. It is opt-in
per site; the default never breaks a page over analytics.

**Pass `ZENITH_PUBLIC`, not `ZENITH_CONFIG`.** The component reads only
`backendUrl` and `siteKey`, so the two objects behave identically here — but
they fail differently. `ZENITH_CONFIG` carries `apiKey` and `jwtSecret`, and if
a `'use client'` boundary ever ends up above this component, React serializes
every field it was handed into the browser payload. Handing it the public half
means there is no secret in reach for that mistake to leak.

**Render it on the server**, which a layout does by default. The snippet is
inlined into the HTML and finds its configuration through
`document.currentScript`, so a script inserted later by client-side React would
never run.

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
import { ZENITH_CONFIG, zenithDashboardReady } from '@/config/zenith'

export const dynamic = 'force-dynamic'

const notConfigured = () =>
  new Response('Zenith dashboard is not configured on this deployment.', { status: 503 })

const handlers = zenithDashboardReady()
  ? createZenithRoute(ZENITH_CONFIG)
  : { GET: async () => notConfigured(), POST: async () => notConfigured() }

export const { GET, POST } = handlers
```

**Pages Router** — `pages/api/zenith/[[...zenith]].ts`:

```ts
import type { NextApiRequest, NextApiResponse } from 'next'
import { createZenithApiRoute } from 'zenith-analytics/next'
import { ZENITH_CONFIG, zenithDashboardReady } from '@/config/zenith'

async function notConfigured(_req: NextApiRequest, res: NextApiResponse) {
  res.status(503).send('Zenith dashboard is not configured on this deployment.')
}

export default zenithDashboardReady() ? createZenithApiRoute(ZENITH_CONFIG) : notConfigured
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

### Why the route is guarded

`createZenithRoute` validates its config the moment the module loads and throws if a secret is
missing. That is deliberate — a dashboard that boots without a password hash is a dashboard
serving your client's analytics to the internet, and failing at startup is the only honest
answer. But *eager* and *fatal* is the wrong trade on a machine that was never meant to have the
secrets: a teammate's first `npm run dev`, or a CI build that renders your marketing pages,
would die on an analytics route nobody asked for.

`zenithDashboardReady()` resolves it by choosing which handler to mount, not by softening the
validation. With the secrets present the real proxy is constructed and validates exactly as
before; without them the route never calls `createZenithRoute` at all and answers a plain
**503**. Tracking is unaffected either way — it needs only the public pair, so pageviews keep
recording while the dashboard sits offline.

The failure it cannot catch is the interesting one: secrets that are present but *wrong*. A
malformed hash or a too-short `jwtSecret` passes the guard and then throws out of
`createZenithRoute`, which is right — that is a real misconfiguration, and it should be loud.

### Why `force-dynamic`

Without it Next may statically render the route at build time and serve every visitor the same
cached page. For a password gate that means handing one client's dashboard to whoever asks. It
belongs at the top of the file, above the guard, so it applies to both branches.

Your client visits `yoursite.com/analytics-dashboard`, enters the password, and sees their
analytics — on their own domain. The proxy reads the data server-side with the secret `apiKey`,
which never reaches the browser.

### Setting the password

```sh
npx zenith hash     # prompts, prints a bcrypt hash
```

Set the result as `ZENITH_PW_HASH` in your deployment environment. Never store a plaintext
password, and note that the hash is still a secret — it belongs in the environment, not in
`config/zenith.ts`. The password is verified against it **in your app** — the Zenith service
never learns it. A correct password mints a signed, HttpOnly, first-party cookie lasting
`sessionTtl` (12 hours by default).

To change it, run `npx zenith hash` again and replace the variable. To publish the dashboard
with no gate at all, set `protected: false` in the config.

### Two dashboards, two passwords

Easy to confuse, so worth stating plainly:

| | Your console | Your client's dashboard |
|---|---|---|
| **Where** | `your-zenith-server/dashboard/` | `theirsite.com/analytics-dashboard` |
| **Signs in with** | Email + password | A password only |
| **Set by** | `ZENITH_ADMIN_EMAIL` / `ZENITH_ADMIN_PASSWORD` on the server | `ZENITH_PW_HASH` in your app's environment |
| **Sees** | Every site you manage | Exactly one site, read-only |

This package only configures the second one.

## Deploying

Two things outside your code decide whether a correct integration works once it
is live. Both fail silently, and both have bitten real deployments.

**Environment variables are read when the page renders.** A statically
prerendered page — the Next.js default — renders during `next build`, so
`ZENITH_URL` and `ZENITH_SITE_KEY` must be present *for the build*, not only at
run time. The three secrets (`ZENITH_API_KEY`, `ZENITH_PW_HASH`,
`ZENITH_JWT_SECRET`) are read only by the dashboard route, which is
`force-dynamic`, so those are needed at run time alone.

| Variable | Needed at | Because |
|---|---|---|
| `ZENITH_URL` | **build** | the tracker is inlined when the page renders |
| `ZENITH_SITE_KEY` | **build** | same — a runtime value cannot fix baked HTML |
| `ZENITH_API_KEY` | run time | read only by the `force-dynamic` dashboard route |
| `ZENITH_PW_HASH` | run time | read only by the dashboard route |
| `ZENITH_JWT_SECRET` | run time | read only by the dashboard route |

On Vercel or Netlify the build runs where your variables already are, so this
usually just works. With Docker, pass the two public values as build arguments:

```dockerfile
ARG ZENITH_URL
ARG ZENITH_SITE_KEY
ENV ZENITH_URL=$ZENITH_URL
ENV ZENITH_SITE_KEY=$ZENITH_SITE_KEY
RUN npm run build
```

```yaml
# docker-compose.yml — a build arg AND, for the secrets, an env_file
services:
  web:
    build:
      context: ./web
      args:
        ZENITH_URL: ${ZENITH_URL}
        ZENITH_SITE_KEY: ${ZENITH_SITE_KEY}
    env_file:
      - .env
```

> [!WARNING]
> Supplying the site key only through `env_file` or `environment:` is the most
> common cause of "no pageviews": those reach the container at run time, after
> the HTML was built. And changing a build argument needs
> `docker compose build web`, not a restart — a restart re-serves the same
> pages.

**If your app sends a Content-Security-Policy, it must allow your Zenith
origin.** The embedded dashboard loads its stylesheet, script and fonts from
your Zenith service and calls back to your own origin for data:

```
script-src  'self' https://zenith.example.com;
style-src   'self' 'unsafe-inline' https://zenith.example.com;
font-src    'self' data: https://zenith.example.com;
connect-src 'self' https://zenith.example.com;
```

> [!WARNING]
> Allowing the origin for `script-src` alone is the trap: the dashboard boots,
> logs in and shows the right numbers — completely unstyled, because the
> stylesheet was blocked. It looks like a broken build and is a blocked
> stylesheet. `font-src` fails the same way, quieter.

## config/zenith.ts

A typed module, read server-side only, that exports two objects and one predicate. It is **safe
to commit**: it names the environment variables the secrets arrive in and holds none of their
values. `npx zenith init` writes it — under `src/` when your routes live there, so the `@` alias
resolves. If your project has no `@` alias, `init` writes a correct relative import instead.

- **`ZENITH_PUBLIC`** — `backendUrl` and `siteKey`. Give this to `<Analytics />`.
- **`ZENITH_CONFIG`** — the full `Partial<ZenithConfig>`. Give this to the dashboard route.
- **`zenithDashboardReady()`** — true when the three secrets are present.

| Field | Source | Required | Default | What it is |
|---|---|---|---|---|
| `backendUrl` | `ZENITH_URL` — public | ✓ | — | Your Zenith service, e.g. `https://zenith.example.com` |
| `siteKey` | `ZENITH_SITE_KEY` — public | ✓ | — | Public key — ships in the page |
| `siteDomain` | in the file — public | ✓ | — | The site being measured, e.g. `example.com` |
| `dashboardPath` | in the file — public | — | `/analytics-dashboard` | Where the dashboard mounts |
| `protected` | in the file — public | — | `true` | Password-gate the dashboard |
| `apiKey` | `ZENITH_API_KEY` — **secret** | ✓ | — | Reads analytics, server-side only |
| `passwordHash` | `ZENITH_PW_HASH` — **secret** | if protected | — | bcrypt hash from `npx zenith hash` |
| `jwtSecret` | `ZENITH_JWT_SECRET` — **secret** | if protected | — | Signs the dashboard cookie, 32+ chars |
| `sessionTtl` | in the file — public | — | `43200` (12h) | Dashboard session length, in seconds |

The split is the whole design. The public rows describe *where* your analytics live and are
harmless in a public repo; the three secret rows are the only values that grant anything, and
they exist solely in your deployment environment. `backendUrl` and `siteKey` read from the
environment too — they change per deployment — but they fall back to a literal in the file
because shipping them is fine.

Every value is validated when the proxy is constructed, and each failure says how to fix it — a
missing hash, a plaintext password where a hash belongs, or a `siteKey` and `apiKey` that are
the same value (easy to transpose, and catastrophic: it would put the secret key in every
visitor's page).

## CLI

```sh
npx zenith init     # scaffold config/zenith.ts + the guarded dashboard route
npx zenith hash     # generate a bcrypt hash for the dashboard password
```

`init` detects App Router vs Pages Router and writes the right wiring for it, generates a
`jwtSecret` and prints it for you to put in your environment, and never touches your
`.gitignore` — the file it writes has nothing to hide. It refuses to clobber an existing
`config/zenith.ts`, and steps around an existing route; `--force` overwrites both.

`hash` prompts for the password rather than taking it as an argument, so it never lands in your
shell history or the process list.

## The two keys

| Key       | Visibility                      | Authorizes                    |
|-----------|---------------------------------|-------------------------------|
| `siteKey` | Public — ships in the snippet   | Writing events only           |
| `apiKey`  | Secret — server-side only       | Reading that site's analytics |

Treat `siteKey` as readable by anyone (it's in your page source). Keep `apiKey` and `jwtSecret`
out of client code and out of git — which, since `config/zenith.ts` only ever reads them from
`process.env`, is what happens by default.

## Entry points

- `zenith-analytics` — server-side: the `ZenithConfig` type, the proxy handler, tracker-script
  helpers. It is handed your secrets, so **do not** import it into browser code.
- `zenith-analytics/client` — browser-safe: `track()`. No config, no secrets.
- `zenith-analytics/next` — `<Analytics />` plus the App Router and Pages Router adapters.
- `zenith-analytics/react` — `<Analytics />` on its own, for a React app that isn't Next.js.

## The Zenith service

This package talks to a Zenith service you run. That side is configured through its own
environment, separate from your app's — and note that `ZENITH_JWT_SECRET` appears in both
without being the same value: there it belongs to the service, here it signs your dashboard's
session cookie. The service's variables are `ZENITH_JWT_SECRET` (the only required one),
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
