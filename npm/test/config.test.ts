import assert from 'node:assert/strict'
import { test } from 'node:test'

import { ConfigError, resolveConfig } from '../src/config.js'

const valid = {
  backendUrl: 'https://zenith.example.com',
  siteKey: 'zk_public',
  apiKey: 'zk_secret',
  siteDomain: 'example.com',
  passwordHash: '$2a$10$abcdefghijklmnopqrstuv',
  jwtSecret: 'a-signing-secret-long-enough-to-pass-ok',
}

test('resolves a valid config and applies defaults', () => {
  const config = resolveConfig(valid)

  assert.equal(config.dashboardPath, '/analytics-dashboard')
  assert.equal(config.protected, true)
  assert.ok(config.sessionTtl! > 0)
})

test('strips trailing slashes so URLs do not double up', () => {
  const config = resolveConfig({ ...valid, backendUrl: 'https://zenith.example.com/' })
  assert.equal(config.backendUrl, 'https://zenith.example.com')
})

// A swap puts the secret key in every visitor's page source. Both are
// zk_-prefixed and impossible to tell apart by eye.
test('rejects the same value for both keys', () => {
  assert.throws(
    () => resolveConfig({ ...valid, siteKey: 'zk_same', apiKey: 'zk_same' }),
    ConfigError,
  )
})

// A protected dashboard with no password is an unprotected dashboard.
test('rejects protected without a password hash', () => {
  const { passwordHash, ...withoutHash } = valid
  void passwordHash
  assert.throws(() => resolveConfig(withoutHash), ConfigError)
})

test('rejects a plaintext password in passwordHash', () => {
  assert.throws(
    () => resolveConfig({ ...valid, passwordHash: 'hunter2' }),
    /not a bcrypt hash/,
  )
})

test('rejects protected without a signing secret', () => {
  const { jwtSecret, ...withoutSecret } = valid
  void jwtSecret
  assert.throws(() => resolveConfig(withoutSecret), ConfigError)
})

test('rejects a short signing secret', () => {
  assert.throws(() => resolveConfig({ ...valid, jwtSecret: 'too-short' }), /at least 32/)
})

// Turning off the gate is a real choice, and must not then demand a password.
test('an unprotected dashboard needs no password', () => {
  const config = resolveConfig({
    backendUrl: 'https://zenith.example.com',
    siteKey: 'zk_public',
    apiKey: 'zk_secret',
    siteDomain: 'example.com',
    protected: false,
  })
  assert.equal(config.protected, false)
})

test('rejects missing required fields', () => {
  for (const field of ['backendUrl', 'siteKey', 'apiKey', 'siteDomain'] as const) {
    const broken = { ...valid, [field]: '' }
    assert.throws(() => resolveConfig(broken), ConfigError, `expected ${field} to be required`)
  }
})

test('rejects a backendUrl that is not http(s)', () => {
  for (const url of ['zenith.example.com', 'ftp://x.com', 'javascript:alert(1)', 'not a url']) {
    assert.throws(() => resolveConfig({ ...valid, backendUrl: url }), ConfigError, url)
  }
})

test('rejects a dashboardPath that is not rooted', () => {
  assert.throws(() => resolveConfig({ ...valid, dashboardPath: 'analytics' }), /must start with/)
})

test('rejects a non-positive session ttl', () => {
  assert.throws(() => resolveConfig({ ...valid, sessionTtl: 0 }), ConfigError)
  assert.throws(() => resolveConfig({ ...valid, sessionTtl: -60 }), ConfigError)
})

// Every message has to say how to fix it: this is read by someone whose
// deployment just refused to start.
test('errors say how to fix the problem', () => {
  try {
    resolveConfig({ ...valid, jwtSecret: 'short' })
    assert.fail('expected a throw')
  } catch (err) {
    assert.match((err as Error).message, /openssl rand/)
  }
})

// The base64url form npx zenith hash now prints must resolve to the same raw
// bcrypt the comparison needs — that is the whole point of accepting it.
test('accepts the base64url-encoded hash and decodes it back to raw bcrypt', () => {
  const rawHash = '$2b$10$C/6dZOCT5XOYgCz1tTWbkODKBGTs5d0ElGOHCyhfNr1Oe2xL.uZcy'
  const encoded = Buffer.from(rawHash, 'utf8').toString('base64url')

  // No '$' survives the encoding — the whole reason it exists.
  assert.doesNotMatch(encoded, /\$/)

  const config = resolveConfig({ ...valid, passwordHash: encoded })
  assert.equal(config.passwordHash, rawHash, 'the encoded hash was not decoded back to raw bcrypt')
})

// A raw hash already in someone's environment keeps working untouched.
test('still accepts a raw bcrypt hash unchanged', () => {
  const rawHash = '$2b$10$C/6dZOCT5XOYgCz1tTWbkODKBGTs5d0ElGOHCyhfNr1Oe2xL.uZcy'
  const config = resolveConfig({ ...valid, passwordHash: rawHash })
  assert.equal(config.passwordHash, rawHash)
})

// The base64url path must not become a way to smuggle a plaintext password
// through: a plaintext does not round-trip into something bcrypt-shaped.
test('a base64url-encoded plaintext is still rejected', () => {
  const encodedPlaintext = Buffer.from('hunter2hunter2', 'utf8').toString('base64url')
  assert.throws(() => resolveConfig({ ...valid, passwordHash: encodedPlaintext }), ConfigError)
})

// A hash whose $ signs were eaten by interpolation is neither raw bcrypt nor
// valid base64url of one, so it is still caught — with the clear message.
test('rejects a hash mangled by interpolation', () => {
  assert.throws(
    () => resolveConfig({ ...valid, passwordHash: '2b10C/6dZOCT5XOYgCz1tTWbkO' }),
    ConfigError,
  )
})
