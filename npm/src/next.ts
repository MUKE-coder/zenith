import type { IncomingMessage, ServerResponse } from 'node:http'

import type { ZenithConfig } from './config.js'
import { createHandler } from './proxy.js'

/**
 * Next.js adapters.
 *
 * Both routers end up calling the same web-standard handler; these only
 * translate between Next's two request shapes and Request/Response.
 */

/**
 * App Router.
 *
 * Mount at `app/<dashboardPath>/[[...zenith]]/route.ts`:
 *
 *     import { createZenithRoute } from 'zenith-analytics/next'
 *     import config from '../../../zenith.config.js'
 *
 *     export const { GET, POST } = createZenithRoute(config)
 *     export const dynamic = 'force-dynamic'
 *
 * `force-dynamic` matters: without it Next may statically render the route at
 * build time and serve every visitor the same cached page -- which for a
 * password gate means serving one client's dashboard to whoever asks.
 */
export function createZenithRoute(config: Partial<ZenithConfig>): {
  GET: (request: Request) => Promise<Response>
  POST: (request: Request) => Promise<Response>
} {
  const handler = createHandler(config)
  return { GET: handler, POST: handler }
}

/**
 * Pages Router.
 *
 * Mount at `pages/api/zenith/[[...zenith]].ts`:
 *
 *     import { createZenithApiRoute } from 'zenith-analytics/next'
 *     import config from '../../../zenith.config.js'
 *
 *     export default createZenithApiRoute(config)
 *     export const config = { api: { bodyParser: false } }
 *
 * and rewrite the dashboard path onto it in `next.config.js`, so the URL the
 * owner sees is their own:
 *
 *     rewrites: async () => [
 *       { source: '/analytics-dashboard/:path*', destination: '/api/zenith/:path*' },
 *       { source: '/analytics-dashboard', destination: '/api/zenith' },
 *     ]
 *
 * `bodyParser: false` matters: the handler reads the password form itself, and
 * Next's parser would consume the stream first, leaving nothing to read.
 */
export function createZenithApiRoute(
  config: Partial<ZenithConfig>,
): (req: IncomingMessage, res: ServerResponse) => Promise<void> {
  const handler = createHandler(config)

  return async function apiRoute(req: IncomingMessage, res: ServerResponse): Promise<void> {
    const request = await toRequest(req, resolvedPath(config, req))
    const response = await handler(request)
    await writeResponse(res, response)
  }
}

/**
 * Recovers the URL the browser actually asked for.
 *
 * The rewrite means req.url is "/api/zenith/..." by the time it reaches here,
 * but the handler matches against the dashboard path and would see nothing it
 * recognizes. Next passes the original through x-invoke-path where available;
 * otherwise the rewritten tail is mapped back onto the mount.
 */
function resolvedPath(config: Partial<ZenithConfig>, req: IncomingMessage): string {
  const mount = config.dashboardPath ?? '/analytics-dashboard'

  const invoked = header(req, 'x-invoke-path')
  if (invoked) return invoked + queryOf(req.url)

  const url = req.url ?? '/'
  const marker = '/api/zenith'

  if (url.startsWith(marker)) {
    const tail = url.slice(marker.length)
    const [path = '', query = ''] = splitQuery(tail)
    return mount + path + (query ? `?${query}` : '')
  }
  return url
}

function splitQuery(value: string): [string, string] {
  const index = value.indexOf('?')
  if (index === -1) return [value, '']
  return [value.slice(0, index), value.slice(index + 1)]
}

function queryOf(url: string | undefined): string {
  if (!url) return ''
  const index = url.indexOf('?')
  return index === -1 ? '' : url.slice(index)
}

function header(req: IncomingMessage, name: string): string | undefined {
  const value = req.headers[name]
  return Array.isArray(value) ? value[0] : value
}

/** Converts a Node request into a web Request. */
async function toRequest(req: IncomingMessage, path: string): Promise<Request> {
  const proto = header(req, 'x-forwarded-proto') ?? 'http'
  const host = header(req, 'host') ?? 'localhost'
  const url = new URL(path, `${proto}://${host}`)

  const headers = new Headers()
  for (const [key, value] of Object.entries(req.headers)) {
    if (value === undefined) continue
    headers.set(key, Array.isArray(value) ? value.join(', ') : value)
  }

  const method = req.method ?? 'GET'

  // A string body: the only thing ever posted here is the password form, and
  // it keeps this clear of the DOM/undici disagreement over what a
  // BufferSource is.
  const body = method === 'GET' || method === 'HEAD' ? undefined : await readBody(req)

  return new Request(url, { method, headers, body })
}

function readBody(req: IncomingMessage): Promise<string> {
  return new Promise((resolve, reject) => {
    const chunks: Buffer[] = []
    req.on('data', (chunk: Buffer) => chunks.push(chunk))
    req.on('end', () => resolve(Buffer.concat(chunks).toString('utf8')))
    req.on('error', reject)
  })
}

/** Writes a web Response to a Node response. */
async function writeResponse(res: ServerResponse, response: Response): Promise<void> {
  res.statusCode = response.status

  response.headers.forEach((value, key) => {
    // Set-Cookie is the one header that may legitimately repeat, and
    // Headers.forEach folds duplicates into one comma-joined string that
    // browsers will not parse back into separate cookies.
    if (key.toLowerCase() === 'set-cookie') return
    res.setHeader(key, value)
  })

  const cookies = response.headers.getSetCookie?.() ?? []
  if (cookies.length > 0) res.setHeader('Set-Cookie', cookies)

  const body = response.body ? Buffer.from(await response.arrayBuffer()) : undefined
  res.end(body)
}
