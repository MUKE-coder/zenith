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
