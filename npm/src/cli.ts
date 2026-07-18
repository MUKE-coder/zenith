#!/usr/bin/env node
import { existsSync } from 'node:fs'
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { createInterface } from 'node:readline/promises'
import { dirname, join } from 'node:path'
import process from 'node:process'

import { hash } from 'bcryptjs'

import { generateSecret } from './session.js'

/**
 * `zenith` CLI.
 *
 * Two commands, both about the awkward parts of setup: writing a config with
 * the right shape, and producing a password hash without ever writing the
 * password down.
 */

const BCRYPT_COST = 10
const MIN_PASSWORD_LENGTH = 8

async function main(): Promise<void> {
  const [command, ...args] = process.argv.slice(2)

  switch (command) {
    case 'init':
      await init(args)
      break
    case 'hash':
      await hashPassword()
      break
    case '--version':
    case '-v':
      console.log(await packageVersion())
      break
    case undefined:
    case '--help':
    case '-h':
      usage()
      break
    default:
      console.error(`Unknown command: ${command}\n`)
      usage()
      process.exit(1)
  }
}

function usage(): void {
  console.log(`Usage: zenith <command>

  init    Set up Zenith in this project
  hash    Turn a password into a hash for zenith.config.js

Options:
  -v, --version   Print the version
`)
}

/**
 * Scaffolds the config and the dashboard route.
 *
 * It writes files and prints what to do next; it does not edit existing source
 * -- a tool that rewrites someone's layout.tsx by pattern-matching will
 * eventually corrupt one, and the paste is three lines.
 */
async function init(args: string[]): Promise<void> {
  const cwd = process.cwd()
  const force = args.includes('--force')

  const configPath = join(cwd, 'zenith.config.js')
  if (existsSync(configPath) && !force) {
    console.error('zenith.config.js already exists. Pass --force to overwrite it.')
    process.exit(1)
  }

  const router = detectRouter(cwd)
  const secret = generateSecret()

  await writeFile(configPath, configTemplate(secret), 'utf8')
  console.log('Created zenith.config.js')

  // The secret is a secret. It goes in the file the owner already ignores.
  await ensureGitignored(cwd, 'zenith.config.js')

  const routePath =
    router === 'app'
      ? join(cwd, appRouterDir(cwd), 'analytics-dashboard', '[[...zenith]]', 'route.ts')
      : join(cwd, 'pages', 'api', 'zenith', '[[...zenith]].ts')

  if (existsSync(routePath) && !force) {
    console.log(`Route already exists at ${relative(cwd, routePath)}, leaving it alone.`)
  } else {
    await mkdir(dirname(routePath), { recursive: true })
    await writeFile(routePath, router === 'app' ? appRouteTemplate() : pagesRouteTemplate(), 'utf8')
    console.log(`Created ${relative(cwd, routePath)}`)
  }

  console.log(`
Detected the ${router === 'app' ? 'App Router' : 'Pages Router'}.

Next:

  1. Fill in backendUrl, siteKey, apiKey, and siteDomain in zenith.config.js.
     Get the keys from your Zenith console: Add site.

  2. Set a dashboard password:

       npx zenith hash

     and paste the result into passwordHash.

  3. Add the tracking component to your ${router === 'app' ? 'app/layout.tsx' : 'pages/_document.tsx'}:

       import { Analytics } from 'zenith-analytics/next'
       import config from './zenith.config.js'

       <Analytics config={config} />

     Render it on the server -- a layout already is. That keeps the snippet
     inline, where the browser will run it, and keeps the config's secrets
     out of the browser payload.
${
  router === 'pages'
    ? `
  4. Rewrite the dashboard path onto the API route in next.config.js:

       async rewrites() {
         return [
           { source: '/analytics-dashboard', destination: '/api/zenith' },
           { source: '/analytics-dashboard/:path*', destination: '/api/zenith/:path*' },
         ]
       }
`
    : ''
}
Then open /analytics-dashboard on your site.
`)
}

/** Hashes a password read from the terminal. */
async function hashPassword(): Promise<void> {
  const rl = createInterface({ input: process.stdin, output: process.stdout })

  try {
    // Read from a prompt, never an argument: a password in argv lands in shell
    // history and in the process list.
    const password = await rl.question('Dashboard password: ')

    if (password.length < MIN_PASSWORD_LENGTH) {
      console.error(`\nToo short. Use at least ${MIN_PASSWORD_LENGTH} characters.`)
      process.exit(1)
    }

    const digest = await hash(password, BCRYPT_COST)

    console.log(`\nAdd this to zenith.config.js:\n\n  passwordHash: ${JSON.stringify(digest)},\n`)
  } finally {
    rl.close()
  }
}

function detectRouter(cwd: string): 'app' | 'pages' {
  if (existsSync(join(cwd, 'app')) || existsSync(join(cwd, 'src', 'app'))) return 'app'
  if (existsSync(join(cwd, 'pages')) || existsSync(join(cwd, 'src', 'pages'))) return 'pages'
  // A fresh Next project is App Router; guessing that beats refusing to help.
  return 'app'
}

function appRouterDir(cwd: string): string {
  return existsSync(join(cwd, 'src', 'app')) ? join('src', 'app') : 'app'
}

function relative(cwd: string, path: string): string {
  return path.startsWith(cwd) ? path.slice(cwd.length + 1) : path
}

async function ensureGitignored(cwd: string, entry: string): Promise<void> {
  const path = join(cwd, '.gitignore')

  let current = ''
  try {
    current = await readFile(path, 'utf8')
  } catch {
    // No .gitignore yet.
  }

  if (current.split(/\r?\n/).some((line) => line.trim() === entry)) return

  const prefix = current === '' || current.endsWith('\n') ? '' : '\n'
  await writeFile(path, `${current}${prefix}\n# Zenith: holds your api key and signing secret\n${entry}\n`, 'utf8')
  console.log(`Added ${entry} to .gitignore`)
}

function configTemplate(secret: string): string {
  return `/**
 * Zenith configuration.
 *
 * This file holds two secrets -- apiKey and jwtSecret -- and is read
 * server-side only. Never import it into client code, and keep it out of git.
 */
module.exports = {
  // Your Zenith service.
  backendUrl: process.env.ZENITH_URL || 'https://zenith.example.com',

  // Public: ships in the tracking snippet, so anyone can read it.
  // It authorizes writing events to this site and nothing else.
  siteKey: process.env.ZENITH_SITE_KEY || '',

  // Secret: authorizes reading this site's analytics. Server-side only.
  apiKey: process.env.ZENITH_API_KEY || '',

  // Where the dashboard lives on your domain.
  dashboardPath: '/analytics-dashboard',

  // Password-gate the dashboard. Turning this off publishes your analytics.
  protected: true,

  // bcrypt hash of the dashboard password. Generate with: npx zenith hash
  passwordHash: process.env.ZENITH_PW_HASH || '',

  // Signs the dashboard session cookie. Generated for you; keep it stable --
  // changing it signs everyone out.
  jwtSecret: process.env.ZENITH_JWT_SECRET || ${JSON.stringify(secret)},

  // Your domain, without the scheme.
  siteDomain: 'example.com',
}
`
}

function appRouteTemplate(): string {
  return `import { createZenithRoute } from 'zenith-analytics/next'

import config from '../../../zenith.config.js'

export const { GET, POST } = createZenithRoute(config)

// Without this, Next may render the route at build time and serve every
// visitor the same cached page -- including the password gate.
export const dynamic = 'force-dynamic'
`
}

function pagesRouteTemplate(): string {
  return `import { createZenithApiRoute } from 'zenith-analytics/next'

import zenithConfig from '../../../zenith.config.js'

export default createZenithApiRoute(zenithConfig)

// The handler reads the password form itself; Next's parser would consume the
// stream before it could.
export const config = { api: { bodyParser: false } }
`
}

async function packageVersion(): Promise<string> {
  try {
    const pkg = await readFile(new URL('../package.json', import.meta.url), 'utf8')
    return (JSON.parse(pkg) as { version: string }).version
  } catch {
    return '0.1.0'
  }
}

main().catch((err: unknown) => {
  console.error(err instanceof Error ? err.message : String(err))
  process.exit(1)
})
