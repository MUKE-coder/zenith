import { compare } from 'bcryptjs'

import { resolveConfig } from './config.js'
import type { ZenithConfig } from './config.js'
import { gatePage } from './gate.js'
import { escapeHtml } from './html.js'
import {
  SESSION_COOKIE,
  clearedCookie,
  readCookie,
  sessionCookie,
  signSession,
  verifySession,
} from './session.js'

/**
 * The domain-native proxy.
 *
 * This is the signature of the product. It runs inside the owner's own app, so
 * the dashboard's URL is their domain, the session cookie is first-party, and
 * there is no CORS to configure -- to the owner it is simply a page of their
 * own site. Nothing here is visible to the visitor's browser except HTML the
 * owner's server rendered.
 *
 * The two secrets it holds -- the site's api key and the session signing
 * secret -- never leave this process. The browser is given a signed cookie and
 * nothing else; every request for data is re-authenticated here and forwarded
 * to Zenith with the api key attached server-side.
 *
 * Framework-agnostic: it takes a web Request and returns a web Response, which
 * is what Next's App Router speaks natively and what the Pages Router adapter
 * converts to and from.
 */

/** Paths under the dashboard mount that the proxy answers itself. */
const LOGOUT_PATH = '/logout'
const API_PREFIX = '/api/'

/** Stats endpoints the proxy will forward, under /api/stats upstream. */
const ALLOWED_ENDPOINTS = new Set([
  'summary',
  'timeseries',
  'pages',
  'referrers',
  'geo',
  'tech',
  'events',
  'realtime',
])

/**
 * The SEO audit the developer ran, read-only.
 *
 * These live under /api/audits upstream rather than /api/stats, so they are
 * matched separately. Reads only, and deliberately: an audit is a headless
 * Chromium crawl of every page, and a POST reachable from a client's dashboard
 * would let anyone past the password gate spend the deployment's crawl budget
 * at will. Zenith refuses it server-side too -- an api key carries owner
 * claims and creating an audit is developer-only -- but the proxy is the
 * boundary that should never have offered it.
 */
const AUDIT_LIST = 'audits'
const AUDIT_DETAIL = /^audits\/([A-Za-z0-9_-]{1,64})$/

export type ZenithHandler = (request: Request) => Promise<Response>

/**
 * Builds the request handler that serves the dashboard.
 *
 * The config is validated once, here, rather than per request: a deployment
 * mistake should stop the app from starting, not produce a 500 the first time
 * a client opens their dashboard.
 */
export function createHandler(input: Partial<ZenithConfig>): ZenithHandler {
  const config = resolveConfig(input)

  return async function handle(request: Request): Promise<Response> {
    const url = new URL(request.url)
    const sub = subPath(url.pathname, config.dashboardPath)

    // The request did not come through the mounted route.
    if (sub === null) return notFound()

    if (sub === LOGOUT_PATH) return logout(config, request)

    // The password gate posts back to the dashboard's own URL.
    if (request.method === 'POST' && sub === '') return signIn(config, request)

    if (!isAuthorized(config, request)) {
      // An unauthenticated data request must not be answered with a login
      // page: the caller is a fetch, and HTML would be nonsense to it.
      if (sub.startsWith(API_PREFIX)) {
        return json({ error: 'Your session has expired. Reload the page.' }, 401)
      }
      return gateResponse(config)
    }

    if (sub.startsWith(API_PREFIX)) return forward(config, request, sub)

    return dashboardShell(config)
  }
}

/**
 * Returns the path beneath the dashboard mount, or null if outside it.
 *
 * Exact-prefix matching with a boundary check: "/analytics-dashboard-secret"
 * must not be treated as living under "/analytics-dashboard".
 */
function subPath(pathname: string, mount: string): string | null {
  if (pathname === mount) return ''
  if (pathname.startsWith(`${mount}/`)) return pathname.slice(mount.length)
  return null
}

function isAuthorized(config: ZenithConfig, request: Request): boolean {
  // An unprotected dashboard is a deliberate choice the owner made, and
  // resolveConfig has already refused the accidental version of it.
  if (!config.protected) return true

  const token = readCookie(request.headers.get('cookie'), SESSION_COOKIE)
  const claims = verifySession(config.jwtSecret!, token)

  // The session names the site it was issued for, so a cookie minted for one
  // site cannot be replayed against a second dashboard sharing a secret.
  return claims !== null && claims.site === config.siteKey
}

/** Verifies the password and starts a session. */
async function signIn(config: ZenithConfig, request: Request): Promise<Response> {
  if (!config.protected) return redirect(config.dashboardPath)

  const form = await request.formData().catch(() => null)
  const password = form?.get('password')

  if (typeof password !== 'string' || password === '') {
    return gateResponse(config, 'Enter the password.', 400)
  }

  const ok = await compare(password, config.passwordHash!)
  if (!ok) {
    // Deliberately vague, and deliberately slow: bcrypt's own cost is what
    // makes guessing expensive.
    return gateResponse(config, 'Incorrect password.', 401)
  }

  const token = signSession(config.jwtSecret!, config.siteKey, config.sessionTtl!)

  return new Response(null, {
    status: 303,
    headers: {
      // 303 with a GET Location: reloading the dashboard must not re-submit
      // the password form.
      Location: config.dashboardPath,
      'Set-Cookie': sessionCookie(token, config.sessionTtl!, isSecure(request)),
    },
  })
}

function logout(config: ZenithConfig, request: Request): Response {
  return new Response(null, {
    status: 303,
    headers: {
      Location: config.dashboardPath,
      'Set-Cookie': clearedCookie(isSecure(request)),
    },
  })
}

/**
 * Maps a dashboard endpoint onto the Zenith path it is allowed to reach, or
 * null if it is not allowed at all.
 *
 * The mapping is explicit rather than a prefix rewrite so that adding a route
 * to Zenith cannot quietly widen what a client's dashboard can ask for.
 */
function upstreamPath(endpoint: string): string | null {
  if (ALLOWED_ENDPOINTS.has(endpoint)) return `/api/stats/${endpoint}`
  if (endpoint === AUDIT_LIST) return '/api/audits'

  const detail = AUDIT_DETAIL.exec(endpoint)
  if (detail) return `/api/audits/${detail[1]}`

  return null
}

/**
 * Forwards a data request to Zenith.
 *
 * The api key is attached here, server-side. The browser never sees it, never
 * sends it, and cannot ask for a site it does not name: the key itself decides
 * which site Zenith answers for, so a tampered query parameter changes nothing.
 */
async function forward(config: ZenithConfig, request: Request, sub: string): Promise<Response> {
  const endpoint = sub.slice(API_PREFIX.length)

  // Every endpoint behind this proxy reads. The fetch below hardcodes GET, so
  // without this a POST would be quietly downgraded into one and answered --
  // which reads like the write succeeded. Refusing is the honest answer, and
  // it keeps the dashboard from ever appearing to offer a write it cannot do.
  if (request.method !== 'GET') {
    return json({ error: 'This endpoint is read-only.' }, 405)
  }

  // An allowlist, not a pass-through. Without it this route is an open proxy
  // into the Zenith service, authenticated with the owner's api key, reachable
  // by anyone who gets past the gate.
  const route = upstreamPath(endpoint)
  if (!route) {
    return json({ error: 'Unknown endpoint.' }, 404)
  }

  const incoming = new URL(request.url)
  const target = new URL(`${config.backendUrl}${route}`)

  // Forward the query, minus `site`: the api key already names the site, and
  // honouring a client-supplied one would invite the confusion of appearing to
  // choose.
  for (const [key, value] of incoming.searchParams) {
    if (key !== 'site') target.searchParams.set(key, value)
  }

  let upstream: Response
  try {
    upstream = await fetch(target, {
      method: 'GET',
      headers: {
        'X-Zenith-API-Key': config.apiKey,
        Accept: 'application/json',
      },
      // No cookies or credentials travel to Zenith: this is a server-to-server
      // call authenticated solely by the api key.
      cache: 'no-store',
    })
  } catch {
    return json({ error: "Couldn't reach the analytics service. Try again." }, 502)
  }

  const body = await upstream.text()

  return new Response(body, {
    status: upstream.status,
    headers: {
      'Content-Type': 'application/json',
      'Cache-Control': 'private, no-store',
    },
  })
}

/**
 * The dashboard page.
 *
 * The SPA is loaded from the Zenith service as a script, but every data
 * request it makes is same-origin against this proxy -- so the page is the
 * owner's, and the API it talks to is the owner's URL.
 */
function dashboardShell(config: ZenithConfig): Response {
  // Stable filenames, not content hashes: this page is rendered on the owner's
  // server, which cannot know a hash that changes with every Zenith build.
  // Freshness comes from revalidation instead -- Zenith serves these with
  // `Cache-Control: no-cache` and an ETag.
  const assets = `${config.backendUrl}/dashboard/assets`

  const html = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex, nofollow">
<meta name="color-scheme" content="light dark">
<title>Analytics — ${escapeHtml(config.siteDomain)}</title>
<link rel="stylesheet" href="${assets}/zenith.css">
</head>
<body>
<div id="root"></div>
<script>
  // Tells the SPA to call this page's own origin rather than the Zenith
  // service: same-origin, first-party cookie, no CORS.
  // siteDomain so the page can name the owner's own site rather than ours:
  // this is their page, and it should read like it.
  window.__ZENITH__ = {
    basePath: ${JSON.stringify(config.dashboardPath)},
    siteDomain: ${JSON.stringify(config.siteDomain)},
    embedded: true
  };
</script>
<script type="module" src="${assets}/zenith.js"></script>
</body>
</html>`

  return new Response(html, {
    status: 200,
    headers: {
      'Content-Type': 'text/html; charset=utf-8',
      'Cache-Control': 'private, no-store',
    },
  })
}

function gateResponse(config: ZenithConfig, error?: string, status = 200): Response {
  return new Response(
    gatePage({ siteName: config.siteDomain, action: config.dashboardPath, error }),
    {
      status,
      headers: {
        'Content-Type': 'text/html; charset=utf-8',
        // A cached password gate is a password gate someone else sees.
        'Cache-Control': 'private, no-store',
      },
    },
  )
}

function redirect(location: string): Response {
  return new Response(null, { status: 303, headers: { Location: location } })
}

function notFound(): Response {
  return new Response('Not found', { status: 404 })
}

function json(body: unknown, status: number): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json', 'Cache-Control': 'private, no-store' },
  })
}

/**
 * Whether to mark the cookie Secure.
 *
 * Behind a proxy the request URL is often http even when the browser used
 * https, so the forwarded protocol is checked first. Marking Secure on plain
 * http would make the cookie undeliverable and lock the owner out of their own
 * dashboard in local development.
 */
function isSecure(request: Request): boolean {
  const forwarded = request.headers.get('x-forwarded-proto')
  if (forwarded) return forwarded.split(',')[0]!.trim() === 'https'
  return new URL(request.url).protocol === 'https:'
}
