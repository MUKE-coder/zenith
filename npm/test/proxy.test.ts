import assert from 'node:assert/strict'
import { after, before, test } from 'node:test'
import { createServer } from 'node:http'
import type { Server } from 'node:http'

import { hash } from 'bcryptjs'

import { createHandler } from '../src/proxy.js'
import { SESSION_COOKIE, signSession } from '../src/session.js'

const PASSWORD = 'a real password'
const SECRET = 'a-signing-secret-long-enough-to-pass-ok'
const MOUNT = '/analytics-dashboard'

/** A stand-in for the Zenith service, so the proxy has something to forward to. */
let upstream: Server
let upstreamUrl: string
let received: Array<{ path: string; apiKey: string | undefined }> = []

before(async () => {
  upstream = createServer((req, res) => {
    received.push({ path: req.url ?? '', apiKey: req.headers['x-zenith-api-key'] as string })
    res.writeHead(200, { 'Content-Type': 'application/json' })
    res.end(JSON.stringify({ pageviews: 42 }))
  })

  await new Promise<void>((resolve) => upstream.listen(0, '127.0.0.1', resolve))
  const address = upstream.address()
  if (typeof address === 'string' || address === null) throw new Error('no address')
  upstreamUrl = `http://127.0.0.1:${address.port}`
})

after(() => {
  upstream.close()
})

async function config() {
  return {
    backendUrl: upstreamUrl,
    siteKey: 'zk_public',
    apiKey: 'zk_secret',
    siteDomain: 'example.com',
    dashboardPath: MOUNT,
    protected: true,
    passwordHash: await hash(PASSWORD, 4),
    jwtSecret: SECRET,
  }
}

function url(path: string): string {
  return `https://example.com${path}`
}

/** The one request the upstream should have received. */
function onlyReceived(): { path: string; apiKey: string | undefined } {
  assert.equal(received.length, 1, `upstream saw ${received.length} requests, expected 1`)
  return received[0]!
}

/** A request carrying a valid dashboard session. */
function authed(path: string, init: RequestInit = {}): Request {
  const token = signSession(SECRET, 'zk_public', 3600)
  return new Request(url(path), {
    ...init,
    headers: { ...(init.headers ?? {}), cookie: `${SESSION_COOKIE}=${token}` },
  })
}

test('an unauthenticated visitor gets the password gate', async () => {
  const handle = createHandler(await config())
  const res = await handle(new Request(url(MOUNT)))

  assert.equal(res.status, 200)
  assert.match(res.headers.get('content-type') ?? '', /text\/html/)

  const html = await res.text()
  assert.match(html, /Password/)
  assert.match(html, /example\.com/)
  // The gate must not be cached: a cached gate is one someone else sees.
  assert.match(res.headers.get('cache-control') ?? '', /no-store/)
})

// The page is on the owner's domain and must not be indexed.
test('the gate asks not to be indexed', async () => {
  const handle = createHandler(await config())
  const res = await handle(new Request(url(MOUNT)))
  assert.match(await res.text(), /noindex/)
})

test('the right password starts a session', async () => {
  const handle = createHandler(await config())

  const body = new URLSearchParams({ password: PASSWORD })
  const res = await handle(
    new Request(url(MOUNT), {
      method: 'POST',
      headers: { 'content-type': 'application/x-www-form-urlencoded' },
      body,
    }),
  )

  // 303 so a reload does not re-post the password.
  assert.equal(res.status, 303)
  assert.equal(res.headers.get('location'), MOUNT)

  const cookie = res.headers.get('set-cookie') ?? ''
  assert.match(cookie, new RegExp(SESSION_COOKIE))
  assert.match(cookie, /HttpOnly/)
})

test('the wrong password does not', async () => {
  const handle = createHandler(await config())

  const res = await handle(
    new Request(url(MOUNT), {
      method: 'POST',
      headers: { 'content-type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({ password: 'wrong password' }),
    }),
  )

  assert.equal(res.status, 401)
  assert.equal(res.headers.get('set-cookie'), null)
  assert.match(await res.text(), /Incorrect password/)
})

test('an empty password does not', async () => {
  const handle = createHandler(await config())

  const res = await handle(
    new Request(url(MOUNT), {
      method: 'POST',
      headers: { 'content-type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({ password: '' }),
    }),
  )

  assert.equal(res.status, 400)
  assert.equal(res.headers.get('set-cookie'), null)
})

test('a session serves the dashboard', async () => {
  const handle = createHandler(await config())
  const res = await handle(authed(MOUNT))

  assert.equal(res.status, 200)
  const html = await res.text()
  assert.match(html, /<div id="root">/)
  assert.match(html, /example\.com/)
})

// The whole point: the browser is told to call the owner's own origin.
test('the dashboard is pointed at this origin, not at Zenith', async () => {
  const handle = createHandler(await config())
  const html = await (await handle(authed(MOUNT))).text()

  assert.match(html, /__ZENITH__/)
  assert.match(html, new RegExp(`basePath[^,]*${MOUNT}`))
})

// This page is rendered here but names files built over in the dashboard
// package. Nothing but agreement keeps the two in step, so the names are
// pinned: when they last drifted, the dashboard silently failed to load and
// only a real browser noticed.
test('the shell names the assets Zenith actually serves', async () => {
  const handle = createHandler(await config())
  const html = await (await handle(authed(MOUNT))).text()

  assert.match(html, new RegExp(`${upstreamUrl}/dashboard/assets/zenith\\.js`))
  assert.match(html, new RegExp(`${upstreamUrl}/dashboard/assets/zenith\\.css`))
})

// The secret keys must never reach the browser.
test('no secret is rendered into the page', async () => {
  const handle = createHandler(await config())

  for (const path of [MOUNT]) {
    const gate = await (await createHandler(await config())(new Request(url(path)))).text()
    const shell = await (await handle(authed(path))).text()

    for (const html of [gate, shell]) {
      assert.doesNotMatch(html, /zk_secret/, 'the api key reached the browser')
      assert.doesNotMatch(html, new RegExp(SECRET), 'the signing secret reached the browser')
      assert.doesNotMatch(html, /\$2[aby]\$/, 'the password hash reached the browser')
    }
  }
})

test('stats are forwarded with the api key attached server-side', async () => {
  received = []
  const handle = createHandler(await config())

  const res = await handle(authed(`${MOUNT}/api/summary?from=2026-07-01&to=2026-07-31`))

  assert.equal(res.status, 200)
  assert.deepEqual(await res.json(), { pageviews: 42 })

  const sent = onlyReceived()
  assert.equal(sent.apiKey, 'zk_secret')
  assert.match(sent.path, /^\/api\/stats\/summary/)
  assert.match(sent.path, /from=2026-07-01/)
})

// The api key already names the site. Honouring a client-supplied one would
// look like it could choose.
test('a client-supplied site parameter is dropped', async () => {
  received = []
  const handle = createHandler(await config())

  await handle(authed(`${MOUNT}/api/summary?site=someone-elses-site&from=2026-07-01`))

  const sent = onlyReceived()
  assert.doesNotMatch(sent.path, /site=/)
  assert.match(sent.path, /from=2026-07-01/)
})

// Without an allowlist this route is an open proxy into Zenith, authenticated
// with the owner's api key: anyone past the gate could read /api/sites and
// enumerate every client.
test('unknown endpoints are refused, not forwarded', async () => {
  received = []
  const handle = createHandler(await config())

  for (const path of ['sites', 'auth/login', 'collect']) {
    const res = await handle(authed(`${MOUNT}/api/${path}`))
    assert.equal(res.status, 404, `forwarded ${path}`)
  }
  assert.equal(received.length, 0, 'something was forwarded upstream')
})

// Traversal cannot smuggle a path past the allowlist. The URL parser
// normalizes "/api/../sites" to "/sites" before the handler sees it, so it
// never matches the /api/ prefix -- but the guarantee worth pinning is that
// nothing reaches Zenith, not which branch declines it.
test('path traversal never reaches the upstream', async () => {
  received = []
  const handle = createHandler(await config())

  for (const path of [
    `${MOUNT}/api/../sites`,
    `${MOUNT}/api/summary/../../sites`,
    `${MOUNT}/api/%2e%2e/sites`,
    `${MOUNT}/api/..%2Fsites`,
  ]) {
    await handle(authed(path))
  }

  assert.equal(received.length, 0, 'a traversal attempt was forwarded to Zenith')
})

test('data requests without a session are refused', async () => {
  received = []
  const handle = createHandler(await config())

  const res = await handle(new Request(url(`${MOUNT}/api/summary`)))

  assert.equal(res.status, 401)
  // A fetch must get JSON back, not a login page it cannot read.
  assert.match(res.headers.get('content-type') ?? '', /application\/json/)
  assert.equal(received.length, 0)
})

test('a session for another site is refused', async () => {
  const handle = createHandler(await config())

  const token = signSession(SECRET, 'zk_a_different_site', 3600)
  const res = await handle(
    new Request(url(MOUNT), { headers: { cookie: `${SESSION_COOKIE}=${token}` } }),
  )

  // Falls back to the gate rather than serving the dashboard.
  assert.equal(res.status, 200)
  assert.match(await res.text(), /Password/)
})

test('an expired session is refused', async () => {
  const handle = createHandler(await config())

  const token = signSession(SECRET, 'zk_public', -1)
  const res = await handle(
    new Request(url(MOUNT), { headers: { cookie: `${SESSION_COOKIE}=${token}` } }),
  )

  assert.match(await res.text(), /Password/)
})

test('logout clears the session', async () => {
  const handle = createHandler(await config())
  const res = await handle(authed(`${MOUNT}/logout`))

  assert.equal(res.status, 303)
  assert.match(res.headers.get('set-cookie') ?? '', /Max-Age=0/)
})

// "/analytics-dashboard-secret" must not be treated as living under the mount.
test('a path that merely starts with the mount is not the mount', async () => {
  const handle = createHandler(await config())

  const res = await handle(authed('/analytics-dashboard-secret'))
  assert.equal(res.status, 404)
})

test('an unprotected dashboard needs no password', async () => {
  const handle = createHandler({
    backendUrl: upstreamUrl,
    siteKey: 'zk_public',
    apiKey: 'zk_secret',
    siteDomain: 'example.com',
    dashboardPath: MOUNT,
    protected: false,
  })

  const res = await handle(new Request(url(MOUNT)))
  assert.equal(res.status, 200)
  assert.match(await res.text(), /<div id="root">/)
})

// A cookie is not marked Secure over plain http, or local development breaks.
test('the cookie is Secure behind an https proxy', async () => {
  const handle = createHandler(await config())

  const res = await handle(
    new Request('http://example.com' + MOUNT, {
      method: 'POST',
      headers: {
        'content-type': 'application/x-www-form-urlencoded',
        'x-forwarded-proto': 'https',
      },
      body: new URLSearchParams({ password: PASSWORD }),
    }),
  )

  assert.match(res.headers.get('set-cookie') ?? '', /Secure/)
})

test('an unreachable Zenith service is a clear error, not a crash', async () => {
  const handle = createHandler({
    ...(await config()),
    backendUrl: 'http://127.0.0.1:1', // nothing listens here
  })

  const res = await handle(authed(`${MOUNT}/api/summary`))

  assert.equal(res.status, 502)
  const body = (await res.json()) as { error: string }
  assert.match(body.error, /Couldn't reach/)
})

// The developer runs an audit in their console; the client reads it in their
// own dashboard. Without this the SEO tab asked the owner's own site for
// /api/audits -- a route only Zenith has -- and showed nothing.
test('the audit list is forwarded to /api/audits', async () => {
  received = []
  const handle = createHandler(await config())

  const res = await handle(authed(`${MOUNT}/api/audits`))
  assert.equal(res.status, 200)

  const seen = onlyReceived()
  assert.match(seen.path, /^\/api\/audits/)
  // Never /api/stats/audits: audits are not a stats endpoint.
  assert.doesNotMatch(seen.path, /\/api\/stats\//)
  assert.equal(seen.apiKey, 'zk_secret')
})

test('an audit detail is forwarded with its id', async () => {
  received = []
  const handle = createHandler(await config())

  const res = await handle(authed(`${MOUNT}/api/audits/job-abc123`))
  assert.equal(res.status, 200)
  assert.match(onlyReceived().path, /^\/api\/audits\/job-abc123/)
})

// An audit is a headless crawl of every page. A POST reachable from a client's
// dashboard would let anyone past the password gate spend the crawl budget.
test('running an audit is not reachable through the proxy', async () => {
  received = []
  const handle = createHandler(await config())

  const res = await handle(
    authed(`${MOUNT}/api/audits`, { method: 'POST', body: '{"site_id":"site-1"}' }),
  )

  // The proxy only forwards GETs, so the POST must not have reached Zenith.
  assert.equal(received.length, 0, 'a POST reached the Zenith service')
  assert.notEqual(res.status, 201)
})

// The allowlist is the boundary: a new route in Zenith must not become
// reachable from a client's dashboard by accident.
test('an unknown audit path is refused', async () => {
  received = []
  const handle = createHandler(await config())

  for (const path of ['/api/audits/../sites', '/api/auditsx', '/api/audits/a/b']) {
    const res = await handle(authed(`${MOUNT}${path}`))
    assert.equal(res.status, 404, `${path} was not refused`)
  }
  assert.equal(received.length, 0, 'a refused path still reached Zenith')
})
