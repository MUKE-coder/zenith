# Zenith — Project Description

> The vantage point over all your sites. Privacy-first, multi-site web analytics and
> SEO auditing that a developer self-hosts once and shares with clients — where each
> client's dashboard lives natively on their own domain.

---

## 1. The idea

Building a website is cheap now. What stays annoying is analytics.

The two common options both fail the independent developer:

- **Vercel Analytics** only works when you host on Vercel, so it locks you to one platform.
- **Google Analytics** works everywhere but is heavy, cookie-laden, consent-banner-inducing,
  and genuinely unpleasant to read.

Zenith is for the developer who builds and manages **many** sites for **many** clients. They
deploy Zenith **once**. Every site they manage reports into that single deployment. From one
console, the developer switches between sites, reads clean privacy-first analytics, runs
on-demand SEO audits, and sends each client a polished monthly report by email.

The differentiator: the client (site owner) never has to visit a strange third-party subdomain.
The client's analytics live at **their own domain** — e.g. `theirsite.com/analytics-dashboard` —
password-protected, and styled so it feels like a native part of their site.

---

## 2. Who it's for

Primary user — **the developer / agency**. Manages multiple client sites, deploys once, wants
one place to see everything and a professional artifact (the monthly email) to send clients.

Secondary user — **the site owner / client**. Non-technical. Receives a monthly email report and
can log in to a password-protected, domain-native page to see their own site's analytics — and
nothing else.

Ranked identity of the product:
1. **Multi-site management for developers** (the core reason it exists).
2. **Domain-native embeddable dashboard** (the thing nobody else does well).
3. **SEO auditing** (a valuable, distinct subsystem).
4. **Privacy-first analytics** (the trustworthy foundation everything sits on).

---

## 3. What problem it solves

- One deploy manages unlimited sites instead of one analytics setup per project.
- No platform lock-in: works whether the tracked site is on Vercel, Netlify, a VPS, anywhere.
- Cookieless and privacy-first: no consent banner, minimal legal surface, tiny script.
- Domain-native dashboards: clients see analytics on their own domain, not a foreign subdomain.
- A monthly email report the developer can send clients with zero manual work.
- SEO auditing built in, so the developer doesn't juggle a separate tool.

---

## 4. How it works (end to end)

### Roles and what each sees
- **Developer** logs into the central Zenith console. Sees all sites. Switches between them.
  Configures global settings (Resend key, MAIL FROM). Runs SEO audits. Owns everything.
- **Site owner** logs into a single password-protected page served on their own domain. Sees
  only their one site, read-only. Also receives the monthly email.

### Two modes, one codebase
- **Central multi-site mode** — the default. One Zenith deploy; many sites report to it; the
  developer manages all of them from the console.
- **Embedded single-site mode** — the same server pointed at a single site, with the dashboard
  proxied onto that site's domain. This is just the multi-site server constrained to one site,
  not a separate build.

### The data path
```
Visitor loads a tracked page
   │  (cookieless tracking snippet, ~1KB)
   ▼
POST /api/collect  ──►  Zenith core service  ──►  DuckDB (event store)
                                                   SQLite (app data)

Developer opens the console            Site owner opens theirsite.com/analytics-dashboard
   │                                        │  (served through the npm proxy)
   ▼                                        ▼
SPA dashboard  ◄── JSON API ── Zenith core ── password gate ── issues JWT
```

### Domain-native dashboard (the signature)
An npm package installed in the owner's project mounts a route (default
`/analytics-dashboard`) and **proxies** it, server-side, to the central Zenith service — scoped
by an API key to that one site. Because the proxy runs inside the owner's app, the page is
same-origin: the URL is the owner's domain, cookies are first-party, and there is no CORS. To
the owner it looks like a native page of their own site.

### Cookieless visitor counting
No cookies, no fingerprinting, no persistent identifier. Unique visitors are counted with a
**daily-rotating hash**: `hash(date + site + ip + user_agent + daily_salt)`. The same visitor on
the same day resolves to the same hash; the next day it changes; the raw salt is never stored and
rotates every 24h. This is the Plausible method — GDPR-clean, no consent banner required.

### Monthly email reports
The core service runs a **built-in scheduler** (no external cron). Once a month, per site, it
compiles the previous month's stats into a clean HTML email and sends it via **Resend** to that
site's configured owner address. Resend API key and MAIL FROM are set **globally** in settings.

### SEO auditing
On demand. In the console the developer clicks **Run audit** for a site. The audit runs as an
**async background job** on a **separate worker** (heavy, Chromium-based). It reads the site's
`sitemap.xml`, renders each listed page with a headless browser, and checks:
- On-page: title, meta description, headings, image alt text, canonical, robots, sitemap health.
- Broken links (internal and outbound).
- Structured data (JSON-LD / schema.org) presence and validity.
- Core Web Vitals / performance from the real render.

Results are written back and shown as a report in the console. Audits never block the UI.

---

## 5. Analytics captured (full feature set)

- Pageviews, unique visitors (cookieless), sessions.
- Top pages, entry/exit pages.
- Referrers and referral sources; UTM campaign breakdown.
- Countries (from IP, coarse — country only), device, browser, OS.
- **Custom events** (`zenith.track('signup')`, etc.) with optional properties.
- Time-series (hour/day/week/month), with date-range selection and comparison to prior period.
- Live/real-time active visitors.

---

## 6. Tech stack

### Core service — Go
- HTTP server (standard library or a light router such as chi).
- **DuckDB** — analytics event store and all aggregation queries (columnar, ideal for GROUP BY
  over large event volumes).
- **SQLite** — transactional app data: users, sessions, sites, settings, audit jobs, report
  history. (DuckDB is analytical, not transactional; keep the two concerns in the store each is
  best at.)
- Built-in scheduler (e.g. robfig/cron) for monthly reports.
- JWT (HS256) for auth.
- Resend HTTP API for email.
- Storage accessed behind **interfaces** (`EventStore`, `AppStore`) so the hosted tier can later
  swap DuckDB→ClickHouse and SQLite→Postgres without touching business logic.

### Audit worker — Go
- **chromedp** driving headless Chromium for real rendering + performance.
- Pulls jobs from a queue (a SQLite table, polled — no Redis needed at this scale).
- Shipped as a **separate Docker image** so the ~1GB Chromium weight never touches the core
  service, and so an OOM in an audit can't take down analytics ingestion.

### npm package — TypeScript
- The tracking snippet (cookieless, tiny).
- `zenith init` CLI: scaffolds `zenith.config.js`, injects the tracking script, mounts the
  dashboard route.
- The **proxy** that makes `/analytics-dashboard` domain-native.
- Reads `zenith.config.js` (backend URL, site API key, dashboard path, protected flag, password
  hash, JWT secret).
- First-class Next.js support (App Router + Pages Router); framework-agnostic core beneath it.

### Dashboard UI — prebuilt SPA
- React + Vite, built to static assets, served through the proxy (owner view) and by the console
  (developer view). Same app, scoped by role and API key.
- Charts via a lightweight library (e.g. Recharts or visx).

### Deployment
- **Docker Compose** is the default self-host target: `core` service + `audit-worker` service +
  a volume for the DuckDB/SQLite data. A developer who doesn't want SEO simply doesn't start the
  worker.

### Business model
- **Open-core.** Self-hosting is free and complete. A later hosted tier (Zenith runs the service,
  clients get a key) reuses the same Go code with the swapped storage drivers.

---

## 7. `zenith.config.js` (shape the npm package reads)

```js
module.exports = {
  backendUrl: process.env.ZENITH_URL,        // central Zenith service
  siteKey:    process.env.ZENITH_SITE_KEY,   // scopes this project to one site
  dashboardPath: "/analytics-dashboard",
  protected: true,                            // password-gate the owner view
  passwordHash: process.env.ZENITH_PW_HASH,   // bcrypt/argon2 hash, never plaintext
  jwtSecret:    process.env.ZENITH_JWT_SECRET,
  siteDomain:   "example.com",
};
```

---

## 8. Non-goals (v1)

- No cookie-based cross-site tracking, no fingerprinting, no personal data retention.
- No scheduled SEO audits (on-demand only in v1; scheduling is a later addition).
- No per-site Resend config (global only in v1).
- No hosted SaaS tier yet — architected for it, not built yet.

---

## 9. Definition of done (v1)

A developer can: deploy Zenith with one `docker compose up`; add a site and get a site key;
install the npm package in a Next.js project and see live pageviews; open the console and switch
between multiple sites; give a client a password-protected dashboard on the client's own domain;
run an on-demand SEO audit and read the report; set global Resend settings and have each site's
owner receive an automatic monthly email report.
