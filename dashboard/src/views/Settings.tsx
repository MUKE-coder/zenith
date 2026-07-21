import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'

import { IconEye, IconEyeOff, IconGlobe } from '../components/icons'
import { EmptyState, ErrorState, Panel, SkeletonRows } from '../components/Panel'
import { useAsync } from '../hooks/useAsync'
import { api } from '../lib/api'
import type { Site } from '../lib/types'
import styles from './Settings.module.css'

type Props = {
  onUnauthorized: () => void
}

/**
 * Global settings and per-site owner emails.
 *
 * Developer only. An owner never sees this: it is the deployment's
 * configuration, and it lists every client.
 */
export function Settings({ onUnauthorized }: Props) {
  const settings = useAsync((signal) => api.settings(signal), [], { onUnauthorized })
  const sites = useAsync((signal) => api.sites(signal), [], { onUnauthorized })

  return (
    <div className={styles.stack}>
      <Panel title="Email">
        {settings.error ? (
          <ErrorState message={settings.error} onRetry={settings.reload} />
        ) : settings.loading || !settings.data ? (
          <SkeletonRows rows={3} />
        ) : (
          <EmailSettings
            configured={settings.data.resend_configured}
            mailFrom={settings.data.mail_from}
            ready={settings.data.email_ready}
            onSaved={settings.reload}
          />
        )}
      </Panel>

      <Panel title="Monthly reports">
        {sites.error ? (
          <ErrorState message={sites.error} onRetry={sites.reload} />
        ) : sites.loading || !sites.data ? (
          <SkeletonRows rows={3} />
        ) : sites.data.sites.length === 0 ? (
          <EmptyState
            icon={<IconGlobe />}
            title="No sites yet"
            hint="Once you add a site and give it an owner email, that owner gets last month's report on the 1st."
          />
        ) : (
          <>
            <p className={styles.hint} style={{ marginTop: 0, marginBottom: 'var(--space-4)' }}>
              Each site's owner gets last month's report on the 1st. Clear an address to stop
              sending. Sending by hand goes out now and covers the month so far, so a site added
              this month still has something to show — it does not replace the scheduled one.
            </p>
            {sites.data.sites.map((site) => (
              <SiteReportRow
                key={site.id}
                site={site}
                emailReady={settings.data?.email_ready ?? false}
                onUnauthorized={onUnauthorized}
              />
            ))}
          </>
        )}
      </Panel>
    </div>
  )
}

type EmailSettingsProps = {
  configured: boolean
  mailFrom: string
  ready: boolean
  onSaved: () => void
}

function EmailSettings({ configured, mailFrom, ready, onSaved }: EmailSettingsProps) {
  // The key is never sent to the browser, so the field starts empty and only a
  // typed value is submitted. Leaving it blank keeps the stored key.
  const [key, setKey] = useState('')
  const [showKey, setShowKey] = useState(false)
  const [from, setFrom] = useState(mailFrom)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string>()
  const [saved, setSaved] = useState(false)

  useEffect(() => setFrom(mailFrom), [mailFrom])

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setError(undefined)
    setSaved(false)
    setSaving(true)

    try {
      await api.updateSettings({
        // Omit the key when untouched, rather than sending a blank that would
        // read as "clear it".
        ...(key.trim() ? { resend_api_key: key.trim() } : {}),
        mail_from: from.trim(),
      })
      setKey('')
      setSaved(true)
      onSaved()
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Something went wrong.')
    } finally {
      setSaving(false)
    }
  }

  return (
    <form onSubmit={onSubmit} noValidate>
      <div className={styles.field}>
        <label className="label" htmlFor="resend-key">
          Resend API key
        </label>
        <div className={styles.revealWrap}>
          <input
            id="resend-key"
            className={`input ${styles.revealInput}`}
            type={showKey ? 'text' : 'password'}
            value={key}
            onChange={(e) => setKey(e.target.value)}
            placeholder={configured ? 'Stored — type a new key to replace it' : 're_...'}
            autoComplete="off"
          />
          <button
            type="button"
            className={styles.reveal}
            onClick={() => setShowKey((v) => !v)}
            aria-label={showKey ? 'Hide key' : 'Show key'}
            aria-pressed={showKey}
            tabIndex={-1}
          >
            {showKey ? <IconEyeOff /> : <IconEye />}
          </button>
        </div>
        <p className={styles.hint}>
          {configured
            ? 'A key is stored. It is never shown again — leave this blank to keep it.'
            : 'Get one from resend.com. Reports cannot send without it.'}
        </p>
      </div>

      <div className={styles.field}>
        <label className="label" htmlFor="mail-from">
          Send from
        </label>
        <input
          id="mail-from"
          className="input"
          type="text"
          value={from}
          onChange={(e) => setFrom(e.target.value)}
          placeholder="Zenith <reports@example.com>"
          autoComplete="off"
        />
        <p className={styles.hint}>
          Must be an address on a domain you have verified with Resend.
        </p>
      </div>

      {error && (
        <p className={`${styles.hint} ${styles.error}`} role="alert">
          {error}
        </p>
      )}

      <div className={styles.actions}>
        <button type="submit" className="button-primary" disabled={saving}>
          {saving ? 'Saving…' : 'Save settings'}
        </button>

        <span className={`${styles.status} ${ready ? styles.ok : styles.warn}`}>
          <span className={styles.dot} aria-hidden="true" />
          {ready ? 'Reports are ready to send' : 'Reports are not configured'}
        </span>

        {saved && <span className={styles.hint}>Saved.</span>}
      </div>
    </form>
  )
}

type SiteReportRowProps = {
  site: Site
  emailReady: boolean
  onUnauthorized: () => void
}

function SiteReportRow({ site, emailReady, onUnauthorized }: SiteReportRowProps) {
  const [email, setEmail] = useState(site.owner_email ?? '')
  const [saving, setSaving] = useState(false)
  const [sending, setSending] = useState(false)
  const [message, setMessage] = useState<string>()
  const [failed, setFailed] = useState(false)

  // Analytics is what the button used to do, so it stays the default. SEO is
  // opt-in because it needs an audit to have been run first.
  const [analytics, setAnalytics] = useState(true)
  const [seo, setSeo] = useState(false)

  const reports = useAsync((signal) => api.reports(site.id, signal), [site.id], {
    onUnauthorized,
  })

  const dirty = email.trim() !== (site.owner_email ?? '')

  async function save() {
    setSaving(true)
    setMessage(undefined)
    setFailed(false)

    try {
      await api.updateSite(site.id, { owner_email: email.trim() })
      setMessage('Saved.')
    } catch (err: unknown) {
      setFailed(true)
      setMessage(err instanceof Error ? err.message : 'Something went wrong.')
    } finally {
      setSaving(false)
    }
  }

  async function send() {
    setSending(true)
    setMessage(undefined)
    setFailed(false)

    try {
      const res = await api.sendReport(site.id, { analytics, seo })

      const sent = [res.analytics && 'analytics', res.seo && 'SEO'].filter(Boolean).join(' and ')
      // The note carries a partial success -- an SEO report skipped because
      // the site has never been audited. Saying only "sent" would hide it.
      setMessage(`Sent ${sent} to ${res.sent_to}.${res.note ? ` ${res.note}` : ''}`)
    } catch (err: unknown) {
      setFailed(true)
      // The server's message names the real problem ("domain is not verified").
      setMessage(err instanceof Error ? err.message : 'Something went wrong.')
    } finally {
      setSending(false)
    }
  }

  const lastFailure = reports.data?.reports.find((r) => r.status === 'failed')

  return (
    <div className={styles.siteRow}>
      <div style={{ minWidth: 0 }}>
        <div className={styles.siteName}>{site.name}</div>
        <div className={styles.siteDomain}>{site.domain}</div>

        {message && (
          <p className={`${styles.hint} ${failed ? styles.error : styles.ok}`} role="status">
            {message}
          </p>
        )}

        {/* A failed send has to surface here, or the developer hears about it
            from their client instead. */}
        {!message && lastFailure && (
          <p className={`${styles.hint} ${styles.error}`}>
            {lastFailure.period} failed: {lastFailure.error}
          </p>
        )}

        {/* The report's "view full dashboard" button needs somewhere to point,
            and only this site's own app knows where. Said here because the
            symptom -- an email that simply has no button -- gives the
            developer nothing to search for. */}
        {!message && !lastFailure && !site.dashboard_path && (
          <p className={styles.hint}>
            No dashboard link in this site&apos;s reports.{' '}
            <span className={styles.subtle}>Set its path in Setup → Client dashboard.</span>
          </p>
        )}
      </div>

      <div className={styles.ownerEmail}>
        <label className="visually-hidden" htmlFor={`owner-${site.id}`}>
          Owner email for {site.name}
        </label>
        <input
          id={`owner-${site.id}`}
          className={`input ${styles.ownerInput}`}
          type="email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          placeholder="owner@client.com"
          autoComplete="off"
        />

        {dirty ? (
          <button type="button" className="button-primary" onClick={save} disabled={saving}>
            {saving ? 'Saving…' : 'Save'}
          </button>
        ) : (
          <div className={styles.sendGroup}>
            {/* Which reports go out. Both are real sends to the client, so the
                choice is in front of the button rather than behind a menu. */}
            <div className={styles.sendPick}>
              <label className={styles.check}>
                <input
                  type="checkbox"
                  checked={analytics}
                  onChange={(e) => setAnalytics(e.target.checked)}
                />
                Analytics
              </label>
              <label className={styles.check}>
                <input type="checkbox" checked={seo} onChange={(e) => setSeo(e.target.checked)} />
                SEO
              </label>
            </div>

            <button
              type="button"
              className="button-secondary"
              onClick={send}
              disabled={sending || !emailReady || !email.trim() || (!analytics && !seo)}
              title={
                !emailReady
                  ? 'Add a Resend API key first'
                  : !email.trim()
                    ? 'Add an owner email first'
                    : !analytics && !seo
                      ? 'Choose at least one report'
                      : 'Send to the client now'
              }
            >
              {sending ? 'Sending…' : 'Send report'}
            </button>
          </div>
        )}
      </div>
    </div>
  )
}
