# Zenith — Build Phases

Build in order. Each phase produces something runnable before the next begins. Mark a task
`[x]` only when it works end to end, not when the code merely exists. Do not skip ahead: later
phases assume the earlier ones are solid.

Legend: `[ ]` todo · `[x]` done · `[~]` in progress

---

## Phase 0 — Foundations & scaffolding
Goal: the repo exists, builds, and runs an empty Go server + empty SPA behind Docker Compose.

- [x] Create monorepo layout: `/core` (Go), `/audit-worker` (Go), `/dashboard` (React SPA), `/npm` (TS package), `/deploy` (compose + Docker).
- [x] Initialize Go modules for `core` and `audit-worker`.
- [x] Initialize the SPA with React + Vite + TypeScript.
- [x] Initialize the npm package with TypeScript and a build (tsup or similar).
- [x] `core` serves a `/health` endpoint returning `{ status: "ok" }`.
- [x] Define storage interfaces `EventStore` and `AppStore` (methods stubbed).
- [x] Wire DuckDB behind `EventStore`; open/create the DB file on boot.
- [x] Wire SQLite behind `AppStore`; run migrations on boot.
- [x] `docker-compose.yml` starts `core` with a mounted data volume.
- [x] Root `README` documents `docker compose up` and the local dev loop.

## Phase 1 — Data model & migrations
Goal: every table the product needs exists and is versioned.

- [x] SQLite migration: `users` (id, email, password_hash, role, created_at).
- [x] SQLite migration: `sites` (id, name, domain, site_key, owner_email, created_at).
- [x] SQLite migration: `sessions` / JWT revocation store.
- [x] SQLite migration: `settings` (resend_api_key, mail_from, singleton row).
- [x] SQLite migration: `audit_jobs` (id, site_id, status, requested_at, finished_at).
- [x] SQLite migration: `audit_results` (job_id, page_url, checks JSON, scores).
- [x] SQLite migration: `report_history` (site_id, period, sent_at, status).
- [x] DuckDB schema: `events` (site_id, ts, type, path, referrer, utm_*, country, device, browser, os, visitor_hash, event_name, props JSON).
- [x] Seed script: create the first developer/admin user.
- [x] Confirm migrations are idempotent and re-runnable.

## Phase 2 — Auth
Goal: the developer can log in; JWTs gate protected routes.

- [x] `POST /api/auth/login` — verify email + password hash, issue HS256 JWT.
- [x] Password hashing with bcrypt or argon2; never store plaintext.
- [x] Auth middleware validating the JWT on protected endpoints.
- [x] Role claim in the JWT: `developer` (all sites) vs `owner` (one site).
- [x] Support login details supplied via env (auto-provision the admin on first boot).
- [x] `POST /api/auth/logout` and token invalidation.
- [x] Owner-scoped token issuance for a single site (used by the domain-native page).
      Resolved in Phase 7. Two owner-scoped credentials exist, both site-scoped and
      tested: `Issuer.IssueOwner` mints an owner JWT, and `X-Zenith-API-Key` resolves a
      site's secret key to the same owner claims. The domain-native page uses the latter,
      because its own session is the proxy's first-party cookie — see Phase 7's note.

## Phase 3 — Event ingestion (the core loop)
Goal: a real pageview from a browser lands in DuckDB.

> **Decided (Phase 3):** a site has **two** keys, because they have different threat models.
> `site_key` is **public** — it ships in the browser snippet, anyone can read it from page source,
> and it authorizes **writing events only**. `api_key` is **secret** — it lives server-side in
> `zenith.config.js`, never reaches a browser, and authorizes **reading analytics**. A leaked
> snippet key lets someone send junk events to one site; it can never read a client's traffic.
> Reading also accepts a developer or owner JWT.

- [x] `POST /api/collect` — accept a pageview payload, validate, enqueue/insert.
- [x] Cookieless `visitor_hash` = hash(date + site + ip + ua + daily_salt); rotate salt every 24h; never persist raw salt.
- [x] Parse user-agent → device / browser / os.
- [x] Resolve IP → country only (coarse, no storage of IP). Opt-in via `ZENITH_GEOIP_DB`:
      the MaxMind database can't be redistributed, so country is unknown without it.
- [x] Custom events: accept `event_name` + `props` on the same endpoint.
- [x] Basic rate limiting / payload-size guards on `/api/collect`.
- [x] Reject events for unknown/invalid `site_key`.

## Phase 4 — Analytics query API
Goal: every dashboard number has an endpoint behind it.

- [x] `GET /api/stats/summary` — pageviews, uniques, sessions for a date range.
- [x] `GET /api/stats/timeseries` — bucketed by hour/day/week/month.
- [x] `GET /api/stats/pages` — top pages, entry/exit.
- [x] `GET /api/stats/referrers` — sources + UTM breakdown.
- [x] `GET /api/stats/geo` and `/api/stats/tech` — country, device, browser, os.
- [x] `GET /api/stats/events` — custom event counts + property breakdowns.
- [x] `GET /api/stats/realtime` — active visitors now.
- [x] Prior-period comparison support on summary + timeseries.
- [x] All stats endpoints scoped by site and enforced by role.

## Phase 5 — Dashboard SPA
Goal: a clean, polished UI over the query API (see `style-guide.md`).

- [x] App shell: sidebar, top bar, site switcher (developer only).
- [x] Summary cards with prior-period deltas.
- [x] Main time-series chart with range + granularity controls.
- [x] Top pages, referrers, geo, tech panels.
- [x] Custom events view.
- [x] Real-time active-visitors indicator.
- [x] Login screen (developer console + owner password gate). The console login is in the
      SPA; the owner's gate is served by the npm proxy on the owner's own domain (Phase 7),
      as its own self-contained page — it has to render before the bundle it protects.
- [x] Empty states and loading states for every panel.
- [x] Fully responsive down to mobile; visible keyboard focus; reduced-motion respected.

## Phase 6 — Site management
Goal: the developer manages many sites from one place.

- [x] `POST /api/sites` — create a site, generate a `site_key`. *(Pulled forward into
      Phase 5: the site switcher needed real sites to switch between, and building it
      against a mock would have meant rewiring it a phase later. Generates both keys.)*
- [x] `GET /api/sites` — list all sites (developer scope). *(Pulled forward into Phase 5.)*
- [x] `PATCH /api/sites/:id` — edit name, domain, owner email. *(Pulled forward into Phase 8:
      the settings screen needs to set a site's owner email. The keys are deliberately not
      editable — rotating one breaks every installed snippet or every dashboard session, and
      an edit form must not be able to do that.)*
- [x] `DELETE /api/sites/:id`. Deletes the site's events from the event store first, then
      cascades its audits and reports in the app store. Confirm-guarded in the UI.
- [x] Site switcher in the SPA wired to the real list. *(Phase 5.)*
- [x] Per-site setup screen showing the snippet + `site_key` to install. A Setup tab
      (developer-only) with npm steps, a `zenith.config.js` template, a drop-in `<script>`
      pointing at core's `/track.js`, both keys (public shown, secret behind Reveal), and
      the delete control.

## Phase 7 — npm package & domain-native dashboard
Goal: `npm install` in a Next.js project → live tracking + a dashboard on the owner's domain.

- [x] Cookieless tracking snippet that POSTs to `/api/collect`. Inlined, not fetched: no
      extra request and nothing for a blocker to match by URL.
- [x] `zenith.config.js` schema + loader.
- [x] `zenith init` CLI: scaffold config, inject snippet, mount route. Also `zenith hash`
      for the dashboard password.
- [x] Next.js App Router route that mounts `/analytics-dashboard`.
- [x] Next.js Pages Router support (via a `next.config.js` rewrite onto an API route).
- [x] Server-side **proxy** from the mounted route to the central service (same-origin, no CORS).
- [x] Password gate on the owner route → owner-scoped JWT.
- [x] `zenith.track()` helper for custom events.
- [x] Verify the owner page is same-origin and looks native to the host site.

> **Decided (Phase 7):** the password gate lives in the **owner's app**, not in core, resolving
> the ambiguity between `project-description.md` §4's diagram and §7's config. §7 is explicit
> that `zenith.config.js` carries `passwordHash` and `jwtSecret`, and it is the better design:
> the session cookie is first-party, and core never learns a client's dashboard password. The
> proxy then reads analytics server-side with the site's secret `api_key` — which is why core
> gained the `X-Zenith-API-Key` path that Phase 2's owner-token task was waiting on.

## Phase 8 — Settings & monthly email reports
Goal: each site owner automatically receives a monthly report.

- [x] Settings screen: global Resend API key + MAIL FROM (stored server-side, masked in UI).
      The key is never returned at all — not even truncated. The UI shows a fixed mask, and
      echoing it back keeps the stored key rather than overwriting it.
- [x] Per-site owner email field. Clearing it turns that site's report off.
- [x] Monthly HTML report template (clean, matches the style guide).
- [x] Report builder: compile prior-month stats per site into the template.
- [x] Built-in scheduler runs monthly, per site, and sends via Resend. 09:00 UTC on the 1st.
- [x] "Send test report now" button per site.
- [x] Record each send in `report_history`; surface failures.

> **Decided (Phase 8):** a **test send never writes to `report_history`**. It is a preview, and
> recording it would mark the month as sent — so the scheduler would skip the client's real
> report, and the preview would have silently replaced the thing it was previewing. The monthly
> run records every outcome, sent or failed, and retries a failed month next time it runs.

## Phase 9 — SEO audit worker
Goal: on-demand, async, full-site SEO audit with a readable report.

- [x] `audit-worker` service with chromedp + Chromium in its own Docker image.
      1.09GB against core's 217MB — which is the whole reason they are separate.
- [x] `POST /api/audits` — enqueue an audit job for a site; return immediately (~70ms).
      A second audit of a site already being audited returns the running one.
- [x] Worker polls `audit_jobs`, claims and runs jobs with a concurrency cap
      (`ZENITH_AUDIT_CONCURRENCY`, default 1 — each concurrent audit is another set
      of live Chromium tabs, so this is the knob that bounds memory).
- [x] Fetch and parse `sitemap.xml` → page list (with a page cap of 50). Follows a
      sitemap index one level; tries https then http.
- [x] Per page: title, meta, headings, alt text, canonical, robots checks.
- [x] Broken-link detection (internal + outbound), cached across pages so a nav bar
      is not re-checked once per page.
- [x] Structured-data (JSON-LD) presence + validity.
- [x] Core Web Vitals / performance from the real render.
- [x] Write results to `audit_results`; compute per-page + site scores.
- [x] Console audit report view: status (queued/running/done), scores, per-page issues.
- [x] Graceful handling of timeouts, missing sitemap, and worker crashes.

> **Note (Phase 9):** the worker is a separate Go module, so it **cannot import core's
> `internal/` packages** — the language enforces the service boundary the architecture asks
> for. The cost is that a handful of queue queries duplicate knowledge of a schema core owns.
> Core runs the migrations; the worker only claims jobs and writes results.

## Phase 10 — Deploy, docs, hardening
Goal: someone else can run Zenith from the README alone.

- [x] `docker-compose.yml` runs core + audit-worker + volume together. *(Phase 9.)*
- [x] Optional-worker documented (skip SEO by not starting the worker). *(Phase 9: the
      worker sits behind a `seo` compose profile, so `docker compose up` is core only.)*
- [x] `.env.example` covering every required variable.
- [x] Security pass: secrets never logged (a test drives every flow at debug level and asserts
      it), JWT expiry, HTTPS + `X-Forwarded-For` guidance, input validation.
- [x] Quickstart docs: deploy → add a site → install the npm package → see data.
- [x] Owner-handoff docs: how to share the domain-native dashboard with a client.
- [x] End-to-end smoke test of the definition-of-done checklist in `project-description.md`.
      All nine clauses walked against a live `docker compose --profile seo up` deployment,
      including a real SEO audit run through the containerized worker and data surviving a
      full restart.

---

## Later (post-v1, not now)
- [ ] Hosted SaaS tier: swap DuckDB→ClickHouse, SQLite→Postgres behind the same interfaces.
- [ ] Scheduled SEO audits + audit history diffing.
- [ ] Per-site Resend overrides.
- [ ] Multi-user / read-only viewers per site.
