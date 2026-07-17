import { useState } from 'react'

import { EmptyState, Panel } from '../components/Panel'
import { api } from '../lib/api'
import type { Site } from '../lib/types'
import styles from './Setup.module.css'

type Props = {
  site: Site | undefined
  onDeleted: () => void
}

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
        <EmptyState title="No site selected." hint="Add a site to see its install steps." />
      </Panel>
    )
  }

  // Derive the backend origin from where the console is served. In the console
  // that is Zenith's own origin, which is exactly the backendUrl a client's
  // zenith.config.js needs.
  const backend = window.location.origin

  const snippet =
    `<script\n` +
    `  src="${backend}/track.js"\n` +
    `  data-site-key="${site.site_key}"\n` +
    `  data-endpoint="${backend}/api/collect"\n` +
    `  defer\n` +
    `></script>`

  const npmSteps =
    `npm install zenith-analytics\n` +
    `npx zenith init      # scaffolds zenith.config.js + the dashboard route\n` +
    `npx zenith hash      # a password for the client's dashboard`

  const config =
    `module.exports = {\n` +
    `  backendUrl: "${backend}",\n` +
    `  siteKey:    "${site.site_key}",\n` +
    `  apiKey:     process.env.ZENITH_API_KEY,  // keep this out of the repo\n` +
    `  siteDomain: "${site.domain}",\n` +
    `}`

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
            Then fill in <code>zenith.config.js</code>:
          </p>
          <CodeBlock code={config} />
        </div>
      </Panel>

      <Panel title="Or drop in the snippet">
        <div className={styles.step}>
          <p className={styles.stepLabel}>
            For any site, not just Next.js. Paste this into the page&nbsp;<code>&lt;head&gt;</code>.
            It's cookieless and about 1&nbsp;KB — no consent banner needed.
          </p>
          <CodeBlock code={snippet} />
        </div>
      </Panel>

      <Panel title="Keys">
        <KeyRow
          name="Site key"
          tag="Public"
          tagClass={styles.public}
          value={site.site_key}
          hint="Ships in the snippet. Anyone can read it. Writes events only."
          alwaysVisible
        />
        <KeyRow
          name="API key"
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

function CodeBlock({ code }: { code: string }) {
  const [copied, setCopied] = useState(false)

  async function copy() {
    try {
      await navigator.clipboard.writeText(code)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {
      // Clipboard blocked (insecure origin, denied permission). The code is
      // right there to select by hand; a failed copy is not worth an error.
    }
  }

  return (
    <div className={styles.code}>
      <button type="button" className={styles.copy} onClick={copy}>
        {copied ? 'Copied' : 'Copy'}
      </button>
      {code}
    </div>
  )
}

type KeyRowProps = {
  name: string
  tag: string
  tagClass: string
  value: string
  hint: string
  alwaysVisible?: boolean
}

function KeyRow({ name, tag, tagClass, value, hint, alwaysVisible }: KeyRowProps) {
  // The secret key is hidden until asked for: it should not sit in a
  // screenshot or over someone's shoulder by default.
  const [shown, setShown] = useState(Boolean(alwaysVisible))

  return (
    <div className={styles.keyRow}>
      <div className={styles.keyMeta}>
        <div className={styles.keyName}>{name}</div>
        <div className={`${styles.keyTag} ${tagClass}`}>{tag}</div>
      </div>
      <div className={styles.keyValue} title={shown ? value : hint}>
        {shown ? value : hint}
      </div>
      {!alwaysVisible && (
        <button type="button" className={styles.reveal} onClick={() => setShown((v) => !v)}>
          {shown ? 'Hide' : 'Reveal'}
        </button>
      )}
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
