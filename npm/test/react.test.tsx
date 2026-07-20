import assert from 'node:assert/strict'
import { test } from 'node:test'

import { renderToStaticMarkup } from 'react-dom/server'

import { Analytics } from '../src/react.js'

const config = {
  backendUrl: 'https://zenith.example.com',
  siteKey: 'zk_public',
  apiKey: 'zk_secret_must_not_leak',
  jwtSecret: 'a-signing-secret-long-enough-to-pass-ok',
  siteDomain: 'example.com',
}

test('renders a script pointing at the collect endpoint', () => {
  const html = renderToStaticMarkup(<Analytics config={config} />)

  assert.match(html, /^<script /)
  assert.match(html, /data-endpoint="https:\/\/zenith\.example\.com\/api\/collect"/)
  assert.match(html, /data-site-key="zk_public"/)
})

test('inlines the tracker rather than requesting it', () => {
  const html = renderToStaticMarkup(<Analytics config={config} />)

  // The snippet itself is in the page: no src attribute, nothing to block by
  // URL, and no extra request on every pageview.
  assert.ok(!html.includes('src='), 'the tracker must be inlined, not fetched')
  assert.match(html, /sendBeacon/)
})

// The whole config object is handed over for convenience, so the component
// must be incapable of putting a secret in the page.
test('never renders a secret, even when handed the full config', () => {
  const html = renderToStaticMarkup(<Analytics config={config} />)

  assert.ok(!html.includes('zk_secret_must_not_leak'), 'the api key reached the page')
  assert.ok(!html.includes(config.jwtSecret), 'the signing secret reached the page')
})

test('a trailing slash on backendUrl does not double the path separator', () => {
  const html = renderToStaticMarkup(
    <Analytics config={{ backendUrl: 'https://zenith.example.com/', siteKey: 'zk_public' }} />,
  )

  assert.match(html, /data-endpoint="https:\/\/zenith\.example\.com\/api\/collect"/)
})

// Analytics must never be the reason a page fails to render.
test('renders nothing when the config is incomplete', () => {
  const missingKey = renderToStaticMarkup(
    <Analytics config={{ backendUrl: 'https://zenith.example.com', siteKey: '' }} />,
  )
  assert.equal(missingKey, '')

  const missingUrl = renderToStaticMarkup(
    <Analytics config={{ backendUrl: '', siteKey: 'zk_public' }} />,
  )
  assert.equal(missingUrl, '')
})

// The failure this exists to prevent: a production build with no site key
// bakes an empty snippet into every prerendered page and collects nothing,
// with a dashboard full of zeroes as the only symptom. Warning only in
// development meant the one build that mattered was the one that stayed quiet.
test('warns in production when the config is incomplete', () => {
  const original = process.env.NODE_ENV
  const warnings: string[] = []
  const realWarn = console.warn
  console.warn = (msg: unknown) => void warnings.push(String(msg))

  try {
    process.env.NODE_ENV = 'production'
    renderToStaticMarkup(
      <Analytics config={{ backendUrl: 'https://zenith.example.com', siteKey: '' }} />,
    )
  } finally {
    console.warn = realWarn
    process.env.NODE_ENV = original
  }

  assert.equal(warnings.length, 1, 'a production build said nothing about collecting nothing')
  assert.match(warnings[0]!, /siteKey/)
  assert.match(warnings[0]!, /ZENITH_SITE_KEY/)
  // The build-time trap is the actual cause, so the message has to name it.
  assert.match(warnings[0]!, /build time/)
})

// A correctly configured render must stay silent, or the warning becomes noise
// that everyone learns to scroll past.
test('says nothing when the config is complete', () => {
  const warnings: string[] = []
  const realWarn = console.warn
  console.warn = (msg: unknown) => void warnings.push(String(msg))

  try {
    renderToStaticMarkup(<Analytics config={config} />)
  } finally {
    console.warn = realWarn
  }

  assert.equal(warnings.length, 0, `warned about a valid config: ${warnings[0]}`)
})
