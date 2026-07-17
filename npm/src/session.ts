import { createHmac, randomBytes, timingSafeEqual } from 'node:crypto'

/**
 * The dashboard's session token.
 *
 * A small HS256 JWT signed with node:crypto rather than a JWT library: the
 * only token this package ever issues is one it also verifies, minutes later,
 * in the same process. That does not justify a dependency on someone else's
 * parser running inside the owner's app.
 *
 * This token is separate from core's. It says "whoever holds this passed the
 * password gate on this site" and nothing more -- core never sees it, and it
 * grants nothing on its own.
 */

export type SessionClaims = {
  /** The site this session may read. */
  site: string
  /** Issued at, seconds since the epoch. */
  iat: number
  /** Expires at, seconds since the epoch. */
  exp: number
}

/** The cookie the session lives in. */
export const SESSION_COOKIE = 'zenith_session'

const HEADER = base64url(JSON.stringify({ alg: 'HS256', typ: 'JWT' }))

/** Signs a session token for a site. */
export function signSession(secret: string, site: string, ttlSeconds: number): string {
  const now = Math.floor(Date.now() / 1000)
  const claims: SessionClaims = { site, iat: now, exp: now + ttlSeconds }

  const payload = base64url(JSON.stringify(claims))
  const body = `${HEADER}.${payload}`

  return `${body}.${sign(secret, body)}`
}

/**
 * Verifies a session token and returns its claims, or null.
 *
 * Returns null for every failure. A caller that cannot tell a bad signature
 * from an expired token cannot accidentally report the difference to whoever
 * is probing it.
 */
export function verifySession(secret: string, token: string | undefined): SessionClaims | null {
  if (!token) return null

  const parts = token.split('.')
  if (parts.length !== 3) return null

  const [header, payload, signature] = parts as [string, string, string]

  // The algorithm is our decision, never the token's: a parser that trusts the
  // header will accept alg=none and any identity that comes with it.
  if (header !== HEADER) return null

  const expected = sign(secret, `${header}.${payload}`)
  if (!safeEqual(signature, expected)) return null

  let claims: SessionClaims
  try {
    claims = JSON.parse(Buffer.from(payload, 'base64url').toString('utf8')) as SessionClaims
  } catch {
    return null
  }

  if (typeof claims.exp !== 'number' || typeof claims.site !== 'string' || !claims.site) {
    return null
  }
  if (claims.exp <= Math.floor(Date.now() / 1000)) return null

  return claims
}

function sign(secret: string, body: string): string {
  return createHmac('sha256', secret).update(body).digest('base64url')
}

function base64url(value: string): string {
  return Buffer.from(value, 'utf8').toString('base64url')
}

/** Compares two signatures without leaking where they diverge. */
function safeEqual(a: string, b: string): boolean {
  const left = Buffer.from(a)
  const right = Buffer.from(b)

  // timingSafeEqual throws on a length mismatch, which would itself be a
  // timing signal. Different lengths cannot match anyway.
  if (left.length !== right.length) return false
  return timingSafeEqual(left, right)
}

/** A random secret, for `zenith init`. */
export function generateSecret(): string {
  return randomBytes(32).toString('base64')
}

/** Serializes the session cookie. */
export function sessionCookie(token: string, ttlSeconds: number, secure: boolean): string {
  const parts = [
    `${SESSION_COOKIE}=${token}`,
    'Path=/',
    `Max-Age=${ttlSeconds}`,
    // The dashboard's own script never reads this cookie, so script must not
    // be able to: HttpOnly is what stops an XSS anywhere on the owner's site
    // from lifting a working session.
    'HttpOnly',
    // Lax, not Strict: the owner clicks a link in the monthly report email and
    // must land signed in. Strict would drop the cookie on that navigation.
    'SameSite=Lax',
  ]
  if (secure) parts.push('Secure')
  return parts.join('; ')
}

/** Serializes the cookie that ends a session. */
export function clearedCookie(secure: boolean): string {
  const parts = [`${SESSION_COOKIE}=`, 'Path=/', 'Max-Age=0', 'HttpOnly', 'SameSite=Lax']
  if (secure) parts.push('Secure')
  return parts.join('; ')
}

/** Reads one cookie out of a Cookie header. */
export function readCookie(header: string | null | undefined, name: string): string | undefined {
  if (!header) return undefined

  for (const pair of header.split(';')) {
    const index = pair.indexOf('=')
    if (index === -1) continue
    if (pair.slice(0, index).trim() === name) {
      return pair.slice(index + 1).trim()
    }
  }
  return undefined
}
