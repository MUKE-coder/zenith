import assert from 'node:assert/strict'
import { test } from 'node:test'

import {
  clearedCookie,
  readCookie,
  sessionCookie,
  signSession,
  verifySession,
} from '../src/session.js'

const SECRET = 'a-signing-secret-long-enough-to-pass-ok'
const OTHER = 'a-completely-different-secret-of-length'

test('signs and verifies a session', () => {
  const token = signSession(SECRET, 'zk_site', 3600)
  const claims = verifySession(SECRET, token)

  assert.ok(claims)
  assert.equal(claims.site, 'zk_site')
  assert.ok(claims.exp > claims.iat)
})

test('rejects a token signed with another secret', () => {
  const token = signSession(OTHER, 'zk_site', 3600)
  assert.equal(verifySession(SECRET, token), null)
})

test('rejects a tampered payload', () => {
  const token = signSession(SECRET, 'zk_site', 3600)
  const [header, , signature] = token.split('.')

  // Re-encode the claims naming a different site, keeping the signature.
  const forged = Buffer.from(
    JSON.stringify({ site: 'zk_someone_else', iat: 0, exp: 9999999999 }),
  ).toString('base64url')

  assert.equal(verifySession(SECRET, `${header}.${forged}.${signature}`), null)
})

// The classic forgery: alg=none and no signature at all.
test('rejects alg=none', () => {
  const header = Buffer.from(JSON.stringify({ alg: 'none', typ: 'JWT' })).toString('base64url')
  const payload = Buffer.from(
    JSON.stringify({ site: 'zk_site', iat: 0, exp: 9999999999 }),
  ).toString('base64url')

  assert.equal(verifySession(SECRET, `${header}.${payload}.`), null)
})

test('rejects an expired token', () => {
  const token = signSession(SECRET, 'zk_site', -1)
  assert.equal(verifySession(SECRET, token), null)
})

test('rejects garbage', () => {
  for (const token of ['', 'nonsense', 'a.b', 'a.b.c.d', 'a.b.c']) {
    assert.equal(verifySession(SECRET, token), null, token)
  }
  assert.equal(verifySession(SECRET, undefined), null)
})

test('rejects a token with no site', () => {
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url')
  const payload = Buffer.from(JSON.stringify({ iat: 0, exp: 9999999999 })).toString('base64url')
  // Signed correctly, but structurally meaningless.
  assert.equal(verifySession(SECRET, `${header}.${payload}.x`), null)
})

// The cookie must be unreadable to script: an XSS anywhere on the owner's site
// must not be able to lift a working session.
test('the session cookie is HttpOnly and SameSite=Lax', () => {
  const cookie = sessionCookie('token', 3600, true)

  assert.match(cookie, /HttpOnly/)
  assert.match(cookie, /SameSite=Lax/)
  assert.match(cookie, /Secure/)
  assert.match(cookie, /Max-Age=3600/)
})

// Marking Secure on plain http makes the cookie undeliverable, which locks the
// owner out of their own dashboard in local development.
test('the cookie is not Secure over plain http', () => {
  assert.doesNotMatch(sessionCookie('token', 3600, false), /Secure/)
})

test('the cleared cookie expires immediately', () => {
  assert.match(clearedCookie(true), /Max-Age=0/)
})

test('reads one cookie out of a header', () => {
  assert.equal(readCookie('a=1; zenith_session=abc; b=2', 'zenith_session'), 'abc')
  assert.equal(readCookie('zenith_session=abc', 'zenith_session'), 'abc')
  assert.equal(readCookie('other=1', 'zenith_session'), undefined)
  assert.equal(readCookie(undefined, 'zenith_session'), undefined)
  assert.equal(readCookie('', 'zenith_session'), undefined)
})

// A cookie value containing '=' (base64url does not, but be sure).
test('reads a cookie value containing =', () => {
  assert.equal(readCookie('t=abc=def', 't'), 'abc=def')
})
