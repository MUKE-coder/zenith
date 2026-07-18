import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'

import { api } from '../lib/api'
import type { Site } from '../lib/types'
import styles from './NewSiteDialog.module.css'

type Props = {
  onClose: () => void
  onCreated: (site: Site) => void
}

/**
 * Adds a site.
 *
 * Until this existed the only way to start collecting was to POST to the API
 * by hand, which is a fine thing to support and a terrible thing to require.
 */
export function NewSiteDialog({ onClose, onCreated }: Props) {
  const [name, setName] = useState('')
  const [domain, setDomain] = useState('')
  const [ownerEmail, setOwnerEmail] = useState('')
  const [error, setError] = useState<string>()
  const [submitting, setSubmitting] = useState(false)

  const nameRef = useRef<HTMLInputElement>(null)
  // Whatever had focus before the dialog opened gets it back on close.
  const restoreRef = useRef<Element | null>(null)

  useEffect(() => {
    restoreRef.current = document.activeElement
    nameRef.current?.focus()

    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKeyDown)

    return () => {
      document.removeEventListener('keydown', onKeyDown)
      if (restoreRef.current instanceof HTMLElement) restoreRef.current.focus()
    }
  }, [onClose])

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setError(undefined)
    setSubmitting(true)

    try {
      const site = await api.createSite({
        name: name.trim(),
        domain: domain.trim(),
        // Sent only when given: an empty string is not an absent value.
        ...(ownerEmail.trim() ? { owner_email: ownerEmail.trim() } : {}),
      })
      onCreated(site)
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Could not add the site. Try again.')
      setSubmitting(false)
    }
  }

  return (
    <div
      className={styles.backdrop}
      // A click that starts and ends on the backdrop dismisses; a drag that
      // merely ends there does not, so selecting text in the form is safe.
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
    >
      <div
        className={styles.dialog}
        role="dialog"
        aria-modal="true"
        aria-labelledby="new-site-title"
      >
        <h2 className={styles.title} id="new-site-title">
          Add a site
        </h2>
        <p className={styles.subtitle}>
          Zenith starts collecting as soon as the snippet is on the page.
        </p>

        <form onSubmit={onSubmit} noValidate>
          <div className={styles.field}>
            <label className="label" htmlFor="site-name">
              Name
            </label>
            <input
              ref={nameRef}
              id="site-name"
              className="input"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Acme Marketing"
              required
            />
            <p className={styles.help}>What you'll call it in this console.</p>
          </div>

          <div className={styles.field}>
            <label className="label" htmlFor="site-domain">
              Domain
            </label>
            <input
              id="site-domain"
              className="input"
              value={domain}
              onChange={(e) => setDomain(e.target.value)}
              placeholder="acme.com"
              required
            />
            <p className={styles.help}>
              The site you're measuring, without <code>https://</code>.
            </p>
          </div>

          <div className={styles.field}>
            <label className="label" htmlFor="site-owner">
              Owner email <span className={styles.optional}>optional</span>
            </label>
            <input
              id="site-owner"
              className="input"
              type="email"
              value={ownerEmail}
              onChange={(e) => setOwnerEmail(e.target.value)}
              placeholder="client@acme.com"
            />
            <p className={styles.help}>
              Who receives this site's monthly report. You can add it later.
            </p>
          </div>

          {error && (
            <p className={styles.error} role="alert">
              {error}
            </p>
          )}

          <div className={styles.actions}>
            <button type="button" className="button-secondary" onClick={onClose}>
              Cancel
            </button>
            <button type="submit" className="button-primary" disabled={submitting}>
              {submitting ? 'Adding…' : 'Add site'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
