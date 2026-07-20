import { useState } from 'react'

import { IconGlobe } from '../components/icons'
import { EmptyState, Panel } from '../components/Panel'
import { api } from '../lib/api'
import type { Site } from '../lib/types'
import styles from './Setup.module.css'

type Props = {
  site: Site | undefined
  onDeleted: () => void
}

/** What the npm package mounts the dashboard at unless told otherwise. */
const DEFAULT_DASHBOARD_PATH = '/analytics-dashboard'

/**
 * Per-site install instructions and keys.
 *
 * Developer-only: it shows the secret api key. An owner never reaches this —
 * the tab is not rendered for them.
 */
export function Setup({ site, onDeleted }: Props) {
  if (!site) {
    return (
      <Panel title="Setup">
        <EmptyState
          icon={<IconGlobe />}
          title="No site selected"
          hint="Pick a site above, or add one — its keys and install steps appear here."
        />
      </Panel>
    )
  }

  return <SiteSetup site={site} onDeleted={onDeleted} />
}

function SiteSetup({ site, onDeleted }: { site: Site; onDeleted: () => void }) {
  // Derive the backend origin from where the console is served. In the console
  // that is Zenith's own origin, which is exactly the backendUrl a client's
  // config needs.
  const backend = window.location.origin

  // The dashboard path is the client's choice, so every snippet below is
  // written against whatever they picked rather than the default. It is also
  // what the monthly report's link is built from, which is why it is stored
  // rather than left as a fact only their codebase knows.
  const [path, setPath] = useState(site.dashboard_path || DEFAULT_DASHBOARD_PATH)

  // Leading slash stripped: it is a directory name here, not a URL.
  const routeDir = (path || DEFAULT_DASHBOARD_PATH).replace(/^\//, '')

  const snippet =
    `<script\n` +
    `  src="${backend}/track.js"\n` +
    `  data-site-key="${site.site_key}"\n` +
    `  data-endpoint="${backend}/api/collect"\n` +
    `  defer\n` +
    `></script>`

  const npmSteps =
    `npm install zenith-analytics\n` +
    `npx zenith init      # scaffolds config/zenith.ts + the dashboard route\n` +
    `npx zenith hash      # a password for the client's dashboard`

  const config =
    `import type { ZenithConfig } from "zenith-analytics"\n\n` +
    `// Both values are PUBLIC by design: the site key ships in the snippet on\n` +
    `// every page, and only authorizes writing events.\n` +
    `export const ZENITH_PUBLIC = {\n` +
    `  backendUrl: process.env.ZENITH_URL || "${backend}",\n` +
    `  siteKey: process.env.ZENITH_SITE_KEY || "${site.site_key}",\n` +
    `}\n\n` +
    `// The secrets come only from the environment, never this file.\n` +
    `export const ZENITH_CONFIG: Partial<ZenithConfig> = {\n` +
    `  ...ZENITH_PUBLIC,\n` +
    `  apiKey: process.env.ZENITH_API_KEY,\n` +
    `  dashboardPath: "${path || DEFAULT_DASHBOARD_PATH}",\n` +
    `  protected: true,\n` +
    `  passwordHash: process.env.ZENITH_PW_HASH,\n` +
    `  jwtSecret: process.env.ZENITH_JWT_SECRET,\n` +
    `  siteDomain: "${site.domain}",\n` +
    `}\n\n` +
    `// createZenithRoute validates at module load and throws on missing\n` +
    `// secrets — right for production, fatal for a local build without them.\n` +
    `export function zenithDashboardReady(): boolean {\n` +
    `  return Boolean(\n` +
    `    ZENITH_CONFIG.apiKey && ZENITH_CONFIG.passwordHash && ZENITH_CONFIG.jwtSecret\n` +
    `  )\n` +
    `}`

  const layout =
    `import { Analytics } from "zenith-analytics/next"\n` +
    `import { ZENITH_PUBLIC } from "@/config/zenith"\n\n` +
    `export default function RootLayout({ children }) {\n` +
    `  return (\n` +
    `    <html lang="en">\n` +
    `      <body>\n` +
    `        {children}\n` +
    `        <Analytics config={ZENITH_PUBLIC} />\n` +
    `      </body>\n` +
    `    </html>\n` +
    `  )\n` +
    `}`

  const route =
    `import { createZenithRoute } from "zenith-analytics/next"\n\n` +
    `import { ZENITH_CONFIG, zenithDashboardReady } from "@/config/zenith"\n\n` +
    `// Without this, Next could statically render the route at build time and\n` +
    `// serve every visitor the same cached page — fatal for a password gate.\n` +
    `export const dynamic = "force-dynamic"\n\n` +
    `const notConfigured = () =>\n` +
    `  new Response("Zenith dashboard is not configured on this deployment.", {\n` +
    `    status: 503,\n` +
    `  })\n\n` +
    `// Secrets present → the real dashboard proxy. Absent (local dev, CI) → a\n` +
    `// plain 503 instead of createZenithRoute's intentional startup throw.\n` +
    `const handlers = zenithDashboardReady()\n` +
    `  ? createZenithRoute(ZENITH_CONFIG)\n` +
    `  : { GET: async () => notConfigured(), POST: async () => notConfigured() }\n\n` +
    `export const { GET, POST } = handlers`

  return (
    <div className={styles.stack}>
      <Panel title="Install with the npm package">
        <div className={styles.step}>
          <p className={styles.stepLabel}>
            The recommended path for a Next.js site. It sets up tracking and the domain-native
            dashboard in one go.
          </p>
          <CodeBlock code={npmSteps} />

          <p className={styles.stepLabel}>
            Then <code>config/zenith.ts</code>. This file holds no secrets, so it is safe to
            commit — the two values in <code>ZENITH_PUBLIC</code> are public by design, and
            everything that isn&apos;t comes from the environment.
          </p>
          <CodeBlock code={config} />

          <p className={styles.stepLabel}>
            Drop the tracker into your root layout, passing only the public half. Leave it
            there — a layout renders on the server, which keeps the snippet inline where the
            browser runs it.
          </p>
          <CodeBlock code={layout} />

          <p className={styles.stepLabel}>
            Finally, create the dashboard route at{' '}
            <code>app/{routeDir}/[[...zenith]]/route.ts</code>. The folder name has to match{' '}
            <code>dashboardPath</code> above, or the proxy answers on a path nothing requests.
          </p>
          <CodeBlock code={route} />
        </div>
      </Panel>

      <DashboardPathPanel site={site} path={path} onChange={setPath} />

      <Panel title="Or drop in the snippet">
        <div className={styles.step}>
          <p className={styles.stepLabel}>
            For any site, not just Next.js. Paste this into the page&nbsp;<code>&lt;head&gt;</code>.
            It's cookieless and about 1&nbsp;KB — no consent banner needed.
          </p>
          <CodeBlock code={snippet} />
        </div>
      </Panel>

      <Panel title="Keys and URL">
        <p className={styles.stepLabel}>
          Zenith generated these when you added {site.name} — you never invent them. These
          three values are everything your app needs.
        </p>
        <KeyRow
          name="Zenith URL"
          envName="ZENITH_URL"
          tag="Public"
          tagClass={styles.public}
          value={backend}
          hint="Where your app sends events and reads stats: this console's own address."
          alwaysVisible
        />
        <KeyRow
          name="Site key"
          envName="ZENITH_SITE_KEY"
          tag="Public"
          tagClass={styles.public}
          value={site.site_key}
          hint="Ships in the snippet, so anyone can read it. Writes events only."
          alwaysVisible
        />
        <KeyRow
          name="API key"
          envName="ZENITH_API_KEY"
          tag="Secret"
          tagClass={styles.secret}
          value={site.api_key}
          hint="Reads this site's analytics. Server-side only — never put it in a browser."
        />
      </Panel>

      <Panel title="Danger zone">
        <DeleteSite site={site} onDeleted={onDeleted} />
      </Panel>
    </div>
  )
}

/**
 * Copies a value and says so.
 *
 * A silent copy button leaves you pressing it twice to be sure, so it
 * confirms and then quietly goes back to offering.
 */
function CopyButton({ value, className }: { value: string; className?: string }) {
  const [copied, setCopied] = useState(false)

  async function copy() {
    try {
      await navigator.clipboard.writeText(value)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {
      // Clipboard blocked (insecure origin, denied permission). The value is
      // right there to select by hand; a failed copy is not worth an error.
    }
  }

  return (
    <button type="button" className={className ?? styles.reveal} onClick={copy}>
      {copied ? 'Copied' : 'Copy'}
    </button>
  )
}

/**
 * Where the client mounted their dashboard.
 *
 * Zenith cannot discover this: the dashboard is a route in the client's own
 * app, and the path they chose is a fact only their codebase knows. Recording
 * it here is what lets the monthly report end with a working link instead of a
 * guess -- and a broken link in a client-facing email is worse than none.
 */
function DashboardPathPanel({
  site,
  path,
  onChange,
}: {
  site: Site
  path: string
  onChange: (path: string) => void
}) {
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState<string>()
  const [failed, setFailed] = useState(false)

  // What the server last confirmed, tracked here rather than read off the
  // site prop: that prop comes from a list this view never refetches, so it
  // still holds the old value after a save. Reading it left Save enabled and
  // the panel looking like nothing had happened.
  const [saved, setSaved] = useState(site.dashboard_path ?? '')
  const dirty = path.trim() !== saved

  async function save() {
    setSaving(true)
    setMessage(undefined)
    setFailed(false)

    try {
      const updated = await api.updateSite(site.id, { dashboard_path: path.trim() })
      const stored = updated.dashboard_path ?? ''

      setSaved(stored)
      onChange(stored)
      setMessage(
        updated.dashboard_url
          ? `Saved. Reports will link to ${updated.dashboard_url}`
          : 'Saved. This site has no dashboard, so reports will omit the link.',
      )
    } catch (err: unknown) {
      setFailed(true)
      setMessage(err instanceof Error ? err.message : 'Something went wrong.')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Panel title="Client dashboard">
      <p className={styles.stepLabel}>
        The path you gave <code>dashboardPath</code> in the config above. Saving it here is
        what puts a working &ldquo;view full analytics&rdquo; link at the end of{' '}
        {site.name}&apos;s monthly report. Leave it empty if this site has no dashboard — the
        report then omits the link rather than guessing at one.
      </p>

      <div className={styles.pathRow}>
        <span className={styles.pathOrigin}>{site.domain}</span>
        <input
          className={`input ${styles.pathInput}`}
          value={path}
          onChange={(e) => onChange(e.target.value)}
          placeholder={DEFAULT_DASHBOARD_PATH}
          spellCheck={false}
          autoComplete="off"
          aria-label={`Dashboard path for ${site.name}`}
        />
        <button type="button" className="button-primary" onClick={save} disabled={saving || !dirty}>
          {saving ? 'Saving…' : 'Save'}
        </button>
      </div>

      {message && (
        <p className={`${styles.stepLabel} ${failed ? styles.error : ''}`} role="status">
          {message}
        </p>
      )}
    </Panel>
  )
}

function CodeBlock({ code }: { code: string }) {
  return (
    <div className={styles.code}>
      <CopyButton value={code} className={styles.copy} />
      {code}
    </div>
  )
}

type KeyRowProps = {
  name: string
  /** The name this value goes by in config — what you actually paste it into. */
  envName: string
  tag: string
  tagClass: string
  value: string
  hint: string
  alwaysVisible?: boolean
}

function KeyRow({ name, envName, tag, tagClass, value, hint, alwaysVisible }: KeyRowProps) {
  // The secret key is hidden until asked for: it should not sit in a
  // screenshot or over someone's shoulder by default.
  const [shown, setShown] = useState(Boolean(alwaysVisible))

  return (
    <div className={styles.keyRow}>
      <div className={styles.keyHead}>
        <div className={styles.keyMeta}>
          <span className={styles.keyName}>{name}</span>
          <span className={`${styles.keyTag} ${tagClass}`}>{tag}</span>
        </div>
        <div className={styles.keyControls}>
          {!alwaysVisible && (
            <button type="button" className={styles.reveal} onClick={() => setShown((v) => !v)}>
              {shown ? 'Hide' : 'Reveal'}
            </button>
          )}
          {/* Copy works whether or not the value is on screen: you rarely
              need to read a secret, only to paste it. */}
          <CopyButton value={value} />
        </div>
      </div>

      <code className={styles.keyEnv}>{envName}</code>

      <div className={styles.keyValue} title={shown ? value : undefined}>
        {shown ? value : '•'.repeat(32)}
      </div>

      <p className={styles.keyHint}>{hint}</p>
    </div>
  )
}

function DeleteSite({ site, onDeleted }: { site: Site; onDeleted: () => void }) {
  const [confirming, setConfirming] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [error, setError] = useState<string>()

  async function remove() {
    setDeleting(true)
    setError(undefined)
    try {
      await api.deleteSite(site.id)
      onDeleted()
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Something went wrong.')
      setDeleting(false)
    }
  }

  if (!confirming) {
    return (
      <div className={styles.step}>
        <p className={styles.stepLabel}>
          Deletes {site.name} and all of its analytics and audits. This cannot be undone.
        </p>
        <button
          type="button"
          className={`button-secondary ${styles.danger}`}
          onClick={() => setConfirming(true)}
        >
          Delete this site
        </button>
      </div>
    )
  }

  return (
    <div className={styles.step}>
      <p className={styles.stepLabel}>
        Really delete <strong>{site.name}</strong> and everything it has collected?
      </p>
      {error && <p style={{ color: 'var(--negative)', fontSize: 'var(--text-label)' }}>{error}</p>}
      <div style={{ display: 'flex', gap: 'var(--space-3)' }}>
        <button
          type="button"
          className={`button-secondary ${styles.danger}`}
          onClick={remove}
          disabled={deleting}
        >
          {deleting ? 'Deleting…' : `Yes, delete ${site.name}`}
        </button>
        <button
          type="button"
          className="button-ghost"
          onClick={() => setConfirming(false)}
          disabled={deleting}
        >
          Cancel
        </button>
      </div>
    </div>
  )
}
