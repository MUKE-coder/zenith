#!/usr/bin/env node
import { existsSync, readFileSync } from 'node:fs'
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { createInterface } from 'node:readline/promises'
import { dirname, join, relative as relativePath, sep } from 'node:path'
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
  hash    Turn a password into a hash for ZENITH_PW_HASH

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

  // Everything is written under the same root Next itself uses, so a `src`
  // project keeps its config beside its routes and the `@` alias resolves.
  const root = projectRoot(cwd)
  const configPath = join(cwd, root, 'config', 'zenith.ts')
  const configLabel = relative(cwd, configPath)

  if (existsSync(configPath) && !force) {
    console.error(`${configLabel} already exists. Pass --force to overwrite it.`)
    process.exit(1)
  }

  const router = detectRouter(cwd)
  const secret = generateSecret()

  await mkdir(dirname(configPath), { recursive: true })
  await writeFile(configPath, configTemplate(), 'utf8')
  console.log(`Created ${configLabel}`)

  // Deliberately not added to .gitignore. The file names the environment
  // variables the secrets arrive in but holds none of them, so committing it
  // is the point: it is how a teammate or a fresh deploy learns the shape.

  const routePath =
    router === 'app'
      ? join(cwd, root, 'app', 'analytics-dashboard', '[[...zenith]]', 'route.ts')
      : join(cwd, root, 'pages', 'api', 'zenith', '[[...zenith]].ts')

  const importPath = configImport(cwd, root, dirname(routePath), configPath)

  if (existsSync(routePath) && !force) {
    console.log(`Route already exists at ${relative(cwd, routePath)}, leaving it alone.`)
  } else {
    await mkdir(dirname(routePath), { recursive: true })
    await writeFile(
      routePath,
      router === 'app' ? appRouteTemplate(importPath) : pagesRouteTemplate(importPath),
      'utf8',
    )
    console.log(`Created ${relative(cwd, routePath)}`)
  }

  const prefix = root === '' ? '' : `${root}/`
  const layoutFile = router === 'app' ? `${prefix}app/layout.tsx` : `${prefix}pages/_document.tsx`

  console.log(`
Detected the ${router === 'app' ? 'App Router' : 'Pages Router'}.

Next:

  1. Set siteDomain, and the backendUrl fallback, in ${configLabel}.
     That file is safe to commit -- it holds no secrets.

  2. Put the secrets in your environment, not in the file:

       ZENITH_URL=https://zenith.example.com
       ZENITH_SITE_KEY=zk_...          # public; from your Zenith console: Add site
       ZENITH_API_KEY=zk_...           # secret; same place
       ZENITH_PW_HASH=$(npx zenith hash)
       ZENITH_JWT_SECRET=${secret}

     The jwt secret above was generated for you. Keep it stable -- changing it
     signs everyone out.

  3. Add the tracking component to your ${layoutFile}:

       import { Analytics } from 'zenith-analytics/next'
       import { ZENITH_PUBLIC } from '${importPath}'

       <Analytics config={ZENITH_PUBLIC} />

     Render it on the server -- a layout already is. That keeps the snippet
     inline, where the browser will run it. Pass ZENITH_PUBLIC rather than the
     whole config: a 'use client' boundary anywhere above it would serialize
     every field it was handed into the browser payload.
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
Then open /analytics-dashboard on your site. Until the three secrets are set
the route answers 503 and the tracker keeps working.
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

    console.log(`\nSet this in your deployment environment:\n\n  ZENITH_PW_HASH=${digest}\n`)
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

/** `src` when Next's routes live there, otherwise the project root itself. */
function projectRoot(cwd: string): string {
  return existsSync(join(cwd, 'src', 'app')) || existsSync(join(cwd, 'src', 'pages')) ? 'src' : ''
}

function relative(cwd: string, path: string): string {
  return path.startsWith(cwd) ? path.slice(cwd.length + 1) : path
}

/**
 * How the route should refer to the config.
 *
 * `@/config/zenith` is what a create-next-app project expects, but the alias is
 * a project's own choice and nothing guarantees it exists or points where we
 * are writing. So it is checked rather than assumed, and a project without it
 * gets a relative path that is correct for the layout actually on disk --
 * which is also the bug in the path this replaces, hardcoded three levels up
 * and wrong the moment routes lived under `src`.
 */
function configImport(cwd: string, root: string, routeDir: string, configPath: string): string {
  if (hasAtAlias(cwd, root)) return '@/config/zenith'

  const specifier = relativePath(routeDir, configPath.replace(/\.ts$/, '')).split(sep).join('/')
  return specifier.startsWith('.') ? specifier : `./${specifier}`
}

/** True when the project maps `@/*` onto the same root we are writing into. */
function hasAtAlias(cwd: string, root: string): boolean {
  for (const file of ['tsconfig.json', 'jsconfig.json']) {
    const target = aliasTarget(join(cwd, file))
    if (target === undefined) continue

    // "./src/*" -> "src", "./*" -> "". Anything else (a monorepo pointing the
    // alias elsewhere) is not ours to guess at.
    const prefix = target.replace(/^\.\//, '').replace(/\*$/, '').replace(/\/$/, '')
    if (prefix === root) return true
  }
  return false
}

function aliasTarget(path: string): string | undefined {
  let source: string
  try {
    source = readFileSync(path, 'utf8')
  } catch {
    return undefined
  }

  try {
    const parsed = JSON.parse(source) as {
      compilerOptions?: { paths?: Record<string, string[]> }
    }
    return parsed.compilerOptions?.paths?.['@/*']?.[0]
  } catch {
    // tsconfig.json permits comments and trailing commas, so a parse failure
    // is ordinary. Fall back to reading the one line that matters.
    return /"@\/\*"\s*:\s*\[\s*"([^"]+)"/.exec(source)?.[1]
  }
}

function configTemplate(): string {
  return `import type { ZenithConfig } from "zenith-analytics";

/**
 * Zenith analytics (https://www.npmjs.com/package/zenith-analytics).
 *
 * The two values below are PUBLIC by design: the site key ships inside the
 * tracking snippet on every page, and only authorizes writing events.
 * The secrets (apiKey — reads analytics; passwordHash + jwtSecret — gate the
 * dashboard) come exclusively from the deployment environment:
 *
 *   ZENITH_API_KEY     — from the Zenith console → Add site
 *   ZENITH_PW_HASH     — bcrypt hash from \`npx zenith hash\`
 *   ZENITH_JWT_SECRET  — any long random string (\`npx zenith init\` generates one)
 *
 * Without them the tracker still runs; only the dashboard stays offline.
 */
export const ZENITH_PUBLIC = {
  backendUrl: process.env.ZENITH_URL || "https://zenith.example.com",
  siteKey: process.env.ZENITH_SITE_KEY || "",
};

export const ZENITH_CONFIG: Partial<ZenithConfig> = {
  ...ZENITH_PUBLIC,
  apiKey: process.env.ZENITH_API_KEY,
  dashboardPath: "/analytics-dashboard",
  protected: true,
  passwordHash: process.env.ZENITH_PW_HASH,
  jwtSecret: process.env.ZENITH_JWT_SECRET,
  siteDomain: "example.com",
};

/**
 * createZenithRoute validates its config at module load and throws on
 * missing secrets — correct for a production deploy, fatal for a local
 * build without env vars. The route only mounts the real handler when
 * this is true.
 */
export function zenithDashboardReady(): boolean {
  return Boolean(
    ZENITH_CONFIG.apiKey && ZENITH_CONFIG.passwordHash && ZENITH_CONFIG.jwtSecret
  );
}
`
}

function appRouteTemplate(importPath: string): string {
  return `import { createZenithRoute } from "zenith-analytics/next";

import { ZENITH_CONFIG, zenithDashboardReady } from "${importPath}";

// Without this, Next could statically render the route at build time and
// serve every visitor the same cached page — fatal for a password gate.
export const dynamic = "force-dynamic";

const notConfigured = () =>
  new Response("Zenith dashboard is not configured on this deployment.", {
    status: 503,
  });

// Secrets present → the real dashboard proxy. Absent (local dev, CI) → a
// plain 503 instead of createZenithRoute's intentional startup throw.
const handlers = zenithDashboardReady()
  ? createZenithRoute(ZENITH_CONFIG)
  : { GET: async () => notConfigured(), POST: async () => notConfigured() };

export const { GET, POST } = handlers;
`
}

function pagesRouteTemplate(importPath: string): string {
  return `import type { NextApiRequest, NextApiResponse } from "next";
import { createZenithApiRoute } from "zenith-analytics/next";

import { ZENITH_CONFIG, zenithDashboardReady } from "${importPath}";

async function notConfigured(_req: NextApiRequest, res: NextApiResponse) {
  res.status(503).send("Zenith dashboard is not configured on this deployment.");
}

// Secrets present → the real dashboard proxy. Absent (local dev, CI) → a
// plain 503 instead of createZenithApiRoute's intentional startup throw.
export default zenithDashboardReady()
  ? createZenithApiRoute(ZENITH_CONFIG)
  : notConfigured;

// The handler reads the password form itself; Next's parser would consume the
// stream before it could.
export const config = { api: { bodyParser: false } };
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
