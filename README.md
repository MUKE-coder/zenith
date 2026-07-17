# Zenith

The vantage point over all your sites. Privacy-first, multi-site web analytics and SEO auditing
that you self-host once and share with clients — where each client's dashboard lives natively on
their own domain.

- **Cookieless.** A daily-rotating visitor hash, no cookies, no fingerprinting, no consent banner.
- **Multi-site.** Deploy once; every site you manage reports into it.
- **Domain-native.** Your client reads their analytics at `theirsite.com/analytics-dashboard`,
  not a foreign subdomain.
- **SEO audits.** On-demand, headless-Chromium audits in a worker that can't take ingestion down.

> **Status: v1 done.** Every clause of the definition of done is verified end-to-end against a
> live `docker compose up`: deploy, add a site, install the npm package and see live pageviews,
> switch between sites, hand a client a password-protected dashboard on their own domain, run an
> on-demand SEO audit, and have each owner emailed a monthly report. 314 tests across the two Go
> services, the npm package, and the dashboard.

---

## Quickstart

From nothing to live analytics on a client's site.

### 1. Deploy

```sh
cp .env.example .env
```

Fill in two things — a signing secret and your admin password:

```sh
# .env
ZENITH_JWT_SECRET=      # openssl rand -base64 32
ZENITH_ADMIN_EMAIL=you@example.com
ZENITH_ADMIN_PASSWORD=  # 12+ characters
```

There's no default secret. Zenith refuses to start without one, because a fallback would be the
key signing every token in every deployment that forgot to change it. Keep it stable — changing
it signs everyone out, which is also how you sign everyone out if it ever leaks.

```sh
cd deploy
docker compose --env-file ../.env up          # core only
docker compose --env-file ../.env --profile seo up   # with SEO audits
```

```sh
curl http://localhost:8080/health
# {"status":"ok"}
```

The console is at `http://localhost:8080/dashboard/`. Sign in with the admin credentials above.

### 2. Add the client's site

**Add site** in the console, or:

```sh
curl -X POST http://localhost:8080/api/sites \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"Client Site","domain":"client.com","owner_email":"owner@client.com"}'
```

You get back two keys. Keep both — the next step needs them.

### 3. Install it in their project

```sh
cd their-nextjs-project
npm install zenith-analytics
npx zenith init      # writes zenith.config.js, scaffolds the dashboard route
npx zenith hash      # prints a bcrypt hash for the dashboard password
```

Paste the keys and the hash into `zenith.config.js`, then add the snippet to your layout:

```tsx
import { trackerScriptProps } from 'zenith-analytics'
import config from '../zenith.config.js'

// In <head>:
<script {...trackerScriptProps(config)} />
```

### 4. See the data

Load a page of their site, then open the console. Pageviews appear immediately — the live
counter updates every 15 seconds.

That's the whole loop. [Hand the dashboard to your client](#hand-the-dashboard-to-your-client)
next, or set up [monthly reports](#monthly-reports).

Everything stateful lives in the `zenith-data` volume — DuckDB (events) and SQLite (app data).
**That volume is the thing to back up.** `docker compose down` keeps it; `down -v` destroys it.

### Accounts

Core provisions the developer account from `ZENITH_ADMIN_EMAIL` and `ZENITH_ADMIN_PASSWORD` on
boot. To add one to a running deployment instead:

```sh
cd core
ZENITH_ADMIN_EMAIL=you@example.com \
ZENITH_ADMIN_PASSWORD='a long passphrase' \
  go run ./cmd/seed
```

Passwords are read from the environment, never a flag — a password in `-password` lands in your
shell history and in the process list. Both paths refuse to overwrite an existing account, so
leaving the variables set can't silently reset your password on the next restart.

### Sign in

```sh
curl -X POST http://localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"you@example.com","password":"a long passphrase"}'
# {"token":"eyJ...","expires_at":"...","role":"developer","email":"you@example.com"}
```

Send the token as `Authorization: Bearer <token>`. `POST /api/auth/logout` ends the session
immediately rather than waiting for the token to expire.

### Add a site

```sh
curl -X POST http://localhost:8080/api/sites \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"Client Site","domain":"client.com","owner_email":"owner@client.com"}'
```

You get back both keys (see [Two keys per site](#two-keys-per-site)). `GET /api/sites` lists
them all. Domains are normalized, so `https://www.Client.com/` and `client.com` are the same
site — which is what keeps the self-referral filter working.

### Open the console

```sh
cd dashboard
npm install
npm run dev
```

Sign in at `localhost:5173` to switch between sites and read their analytics.

### Send an event

```sh
curl -X POST http://localhost:8080/api/collect \
  -H 'Content-Type: application/json' \
  -d '{"site_key":"zk_...","url":"https://example.com/pricing","referrer":"https://news.ycombinator.com/"}'
# 204 No Content
```

`name` defaults to `pageview`; any other name is a custom event and may carry `props`:

```sh
  -d '{"site_key":"zk_...","url":"https://example.com/signup","name":"signup","props":{"plan":"pro"}}'
```

The server derives the path and UTM parameters from `url`, so the snippet stays tiny and the
parsing rules live in one place. Bots are answered `204` and never recorded.

### Read the stats

Every endpoint needs `Authorization: Bearer <token>`.

| Endpoint                | Answers                                          |
|-------------------------|--------------------------------------------------|
| `GET /api/stats/summary`    | Pageviews, unique visitors, sessions          |
| `GET /api/stats/timeseries` | Traffic over time, bucketed                   |
| `GET /api/stats/pages`      | Top pages, plus entry and exit pages          |
| `GET /api/stats/referrers`  | Referral sources and the UTM breakdown        |
| `GET /api/stats/geo`        | Countries                                     |
| `GET /api/stats/tech`       | Devices, browsers, operating systems          |
| `GET /api/stats/events`     | Custom events; `?name=` adds its properties   |
| `GET /api/stats/realtime`   | Visitors active in the last 5 minutes         |

```sh
curl -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/stats/summary?site=<id>&from=2026-07-01&to=2026-07-31&compare=true"
```

Common parameters:

- **`site`** — which site. A developer must name one; an **owner's token names their own**, and
  asking for a different site is a 403.
- **`from` / `to`** — a date (`2026-07-01`) or a timestamp. Omit both for the last 30 days. A bare
  `to` date covers that whole day, so `from=2026-07-01&to=2026-07-31` means all of July.
- **`compare=true`** — adds the equal-length period immediately before, and the percent change.
  Change is `null` when the previous period was zero: growth from nothing has no percentage.
- **`granularity`** — `hour`, `day`, `week`, or `month`. Defaults to fit the range.
- **`limit`** — rows per breakdown, 1 to 500.

Sessions aren't stored, they're derived: a run of one visitor's events with no gap longer than
30 minutes. Empty timeseries buckets come back as zeroes, so a quiet day shows as a dip rather
than vanishing from the chart.

### Two keys per site

A site has two keys, because writing events and reading analytics have different threat models.

| Key        | Visibility                        | Authorizes            |
|------------|-----------------------------------|-----------------------|
| `site_key` | **Public** — ships in the snippet | Writing events only   |
| `api_key`  | **Secret** — server-side only     | Reading that site's analytics |

Treat `site_key` as readable by anyone: it's in your page source. A leaked one lets someone send
junk events to that site; it can never read a client's traffic. `api_key` belongs in
`zenith.config.js` on the server and must never reach a browser.

---

## Layout

| Path            | What it is                                                        |
|-----------------|-------------------------------------------------------------------|
| `core/`         | Go service: ingestion, stats API, auth, scheduler, console        |
| `audit-worker/` | Go service: headless-Chromium SEO audits, separate image          |
| `dashboard/`    | React + Vite SPA: the console and the owner's dashboard           |
| `npm/`          | TypeScript package: tracking snippet, `zenith init` CLI, proxy    |
| `deploy/`       | Dockerfiles and Compose                                           |

Two services, not one: Chromium is ~1GB, and an out-of-memory audit must never be able to take
analytics ingestion down with it.

Two stores, not one: DuckDB is columnar and answers `GROUP BY` over large event volumes; SQLite is
transactional and holds users, sites, and settings. Events never go in SQLite; app data never goes
in DuckDB. Both sit behind the `EventStore` / `AppStore` interfaces in
[`core/internal/storage`](core/internal/storage/storage.go), so a hosted tier can later swap in
ClickHouse and Postgres without business logic changing.

---

## Local dev

Prerequisites: **Go 1.26+**, **Node 20+**, a **C toolchain** (DuckDB is a C++ library, so `core`
needs CGO), and Docker for the Compose path.

### core

```sh
cd core
go run ./cmd/core
```

Listens on `:8080` and writes to `./data` by default.

| Variable                 | Default                   | What it does                                  |
|--------------------------|---------------------------|-----------------------------------------------|
| `ZENITH_JWT_SECRET`      | —                         | **Required.** Signs session tokens, 32+ chars |
| `ZENITH_PORT`            | `8080`                    | HTTP listen port                              |
| `ZENITH_DATA_DIR`        | `./data`                  | Where both databases live                     |
| `ZENITH_EVENTS_DB`       | `$DATA_DIR/events.duckdb` | DuckDB event store                            |
| `ZENITH_APP_DB`          | `$DATA_DIR/zenith.sqlite` | SQLite app store                              |
| `ZENITH_TOKEN_TTL`       | `24h`                     | How long a session lasts                      |
| `ZENITH_ADMIN_EMAIL`     | —                         | Developer account to provision on boot        |
| `ZENITH_ADMIN_PASSWORD`  | —                         | Its password, 12+ characters                  |
| `ZENITH_GEOIP_DB`        | —                         | MaxMind country `.mmdb`; without it, country is unknown |
| `ZENITH_DASHBOARD_DIR`   | `./dashboard`             | Built SPA; missing means API-only              |
| `ZENITH_RESEND_ENDPOINT` | Resend                    | Point email at a mock to verify a deploy       |
| `ZENITH_ENV`             | `production`              | `development` relaxes the secret requirement  |

The Resend API key and MAIL FROM are **not** environment variables — they're set in the console
and stored in the database, because they're configuration a developer changes, not deployment
plumbing.

In development, `ZENITH_ENV=development` lets core generate a throwaway signing secret so
`go run ./cmd/core` works with no setup. It says so loudly on boot, and every restart signs
everyone out. Never use it in a deployment.

Migrations run automatically on boot and are idempotent — re-running is a no-op.

Run the tests with:

```sh
cd core
go test ./...
```

### dashboard

```sh
cd dashboard
npm install
npm run dev
```

Serves on `:5173` and proxies `/api` to core, so dev is same-origin exactly as production is.
Point it elsewhere with `ZENITH_CORE_URL`.

### npm package

```sh
cd npm
npm install
npm run build
```

### audit-worker

```sh
cd audit-worker
go run ./cmd/worker
```

Polls for audit jobs. The audit itself arrives in Phase 9.

---

## Install it in a client's site

This is the part nobody else does: your client reads their analytics at **their own URL**,
password-protected, looking like a page of their own site.

```sh
cd their-nextjs-project
npm install zenith-analytics
npx zenith init
npx zenith hash          # prints a bcrypt hash for the dashboard password
```

`zenith init` writes `zenith.config.js`, scaffolds the dashboard route, and adds the config to
`.gitignore` — it holds two secrets. Fill in the keys from **Add site**, then add the snippet to
your layout:

```tsx
import { trackerScriptProps } from 'zenith-analytics'
import config from '../zenith.config.js'

// In <head>:
<script {...trackerScriptProps(config)} />
```

Custom events, from anywhere in the client bundle:

```ts
import { track } from 'zenith-analytics/client'
track('signup', { plan: 'pro' })
```

Then `theirsite.com/analytics-dashboard` asks for the password and shows their analytics.

### Hand the dashboard to your client

The client never visits a Zenith URL, never makes an account, and never learns Zenith exists.
You send them two things:

> Your analytics are at **https://theirsite.com/analytics-dashboard**
> Password: *(whatever you hashed with `npx zenith hash`)*

That's the handoff. What they get:

- A page on **their own domain**, styled neutrally, that reads as part of their site.
- **Only their site.** The site is named by the api key server-side, so there is no
  parameter they could change to see another client's traffic — and no site switcher to
  suggest other clients exist.
- **Read-only.** No settings, no other sites, nothing to break.
- A session that lasts 12 hours (`sessionTtl` in `zenith.config.js`).

To change their password, run `npx zenith hash` again and replace `passwordHash`. To turn the
dashboard off entirely, set `protected: false` to publish it, or remove the route.

If you also set their **owner email**, they get the monthly report without ever opening the
dashboard at all — which for most clients is the whole product.

### How the domain-native dashboard works

```
Owner's browser ──► theirsite.com/analytics-dashboard   (their domain, first-party cookie)
                          │  the proxy runs inside their app
                          │  password gate → signed HttpOnly cookie
                          ▼
                    Zenith service          (server-to-server, X-Zenith-API-Key)
```

Every data request the page makes is **same-origin** against their own server. The proxy
re-authenticates it, then forwards it to Zenith with the site's secret `api_key` attached
server-side. The browser never sees that key, and the key names the site — so a tampered
`?site=` parameter changes nothing.

The password is verified against a bcrypt hash in `zenith.config.js`. Zenith itself never learns
it. `sessionTtl` controls how long a dashboard session lasts (12 hours by default).

---

## SEO audits

Optional, and off by default:

```sh
cd deploy
docker compose --profile seo up
```

Then **Run audit** in the console. The audit reads the site's `sitemap.xml`, renders each page
with a real headless Chromium, and reports:

- **On-page** — title, meta description, H1s, heading order, image alt text, canonical, robots.
- **Broken links**, internal and outbound, with the URLs and what they returned.
- **Structured data** — JSON-LD presence and validity.
- **Core Web Vitals** from the actual render: LCP, FCP, TTFB, CLS.

Each page gets a score out of 100 (errors cost more than warnings), and the site score is their
average. Audits never block: enqueueing returns in milliseconds and the console polls.

The worker is a **separate service and image** on purpose. It carries Chromium, which makes it
~1GB against core's ~200MB, and an out-of-memory kill while rendering a client's site must not
be able to take analytics ingestion down with it. Skip it and everything else still works.

| Variable                   | Default | What it does                                        |
|----------------------------|---------|-----------------------------------------------------|
| `ZENITH_AUDIT_CONCURRENCY` | `1`     | Audits at once. Each one is more live Chromium tabs. |
| `ZENITH_CHROME_PATH`       | `$PATH` | Chromium binary                                      |

A crashed worker doesn't strand an audit: jobs left running are requeued when a worker starts
and every five minutes after.

---

## Monthly reports

Each client gets last month's numbers emailed to them on the **1st at 09:00 UTC**, automatically.
No cron to configure — the scheduler is built into core.

Set it up once in **Settings**:

1. A **Resend API key**. Stored server-side and never returned to the browser — not masked, not
   truncated. Once saved, you can change anything else without re-typing it.
2. **Send from** — an address on a domain you've verified with Resend.
3. An **owner email** per site. Clear it to stop that site's report.

**Send test** delivers the report immediately so you can see what your client sees. It
deliberately doesn't record the send, so the real report still goes out on the 1st.

A send is safe to retry and impossible to duplicate: `report_history` has a `UNIQUE (site_id,
period)` constraint, so a restart on the 1st can't mail a client twice. A failed month is retried
on the next run and its reason (Resend's own words, like "domain is not verified") surfaces in
Settings rather than being silently swallowed.

---

## How the privacy works

No cookies, no fingerprinting, no persistent identifier, and so no consent banner.

A visitor is counted as a keyed hash of things already in their request:

```
visitor_hash = HMAC-SHA256(daily_salt, date | site_id | ip | user_agent)
```

The same person on the same site on the same day hashes to the same value, so they're counted
once. The salt lives **in memory only** and rotates at UTC midnight — it is never written to
disk, a log, or either database. That's what makes yesterday's visitor unlinkable to today's:
the key that would prove they're the same person no longer exists anywhere.

The raw IP and user agent are read from the request, used to derive that hash and a coarse
country/device/browser bucket, and then dropped. **Neither is ever stored** — there is no IP
column in the schema, and a test enforces that one can't be added. `site_id` is inside the hash
so the same person on two of your sites cannot be linked across them.

The trade: restarting core starts a new salt, so a visitor already counted today may be counted
once more. That's deliberate. Over-counting a few visitors is a rounding error; a salt you could
recover from disk would be a permanent way to de-anonymize everyone in the database.

Referrers are stored as a hostname, never a full URL — referring URLs routinely carry private
context in their paths and query strings.

---

## Running it safely

**Put it behind HTTPS.** Zenith does not terminate TLS — that's your reverse proxy's job
(Caddy, nginx, Traefik, a load balancer). Over plain HTTP, session tokens and the Resend key
travel in clear text. Caddy is two lines:

```
zenith.example.com {
    reverse_proxy localhost:8080
}
```

Zenith reads `X-Forwarded-Proto` and `X-Forwarded-For`, so a standard proxy setup works
unchanged: the dashboard's cookie is marked `Secure` behind HTTPS, and visitor IPs resolve to
the real client rather than to your proxy.

**Don't expose Zenith directly to the internet.** It trusts `X-Forwarded-For`, which it has to —
behind a proxy, without it every visitor would appear to come from the proxy and your unique
visitor count would be 1. But that trust only makes sense when something in front is setting the
header. Reachable directly, anyone can send their own, which forges the visitor hash and slips
past the per-IP rate limit on `/api/collect`. The worst case is junk in one site's numbers, not
a data leak — but put a proxy in front. You need one for TLS anyway.

**Back up the volume.** `zenith-data` holds both databases and is the only stateful thing in a
deployment. `docker compose down` keeps it; `down -v` destroys it.

What Zenith enforces for you:

| | |
|---|---|
| **Passwords** | bcrypt, 12+ characters, never logged, never returned. Over-72-byte passwords are rejected rather than silently truncated. |
| **Sessions** | HS256, always expiring, `alg=none` rejected. Logout revokes immediately rather than waiting for expiry. |
| **Secrets** | The Resend key is never returned once saved — not even masked. A test drives every flow at debug level and asserts nothing leaks into logs. |
| **Site scoping** | A query can't express a cross-site question. An owner's site comes from their signed token, never a URL parameter. |
| **Input** | Every endpoint caps its body, rejects unknown fields, and validates. `/api/collect` is rate-limited per IP. |
| **The two keys** | The public one writes events and can't read. The secret one reads one site and can't list sites. |

What's on you: HTTPS, backups, and keeping `ZENITH_JWT_SECRET` out of git.

---

## Docs

- [`project-description.md`](project-description.md) — the vision, architecture, and definition of done.
- [`phases.md`](phases.md) — the build plan and current progress.
- [`style-guide.md`](style-guide.md) — the visual system every screen conforms to.
