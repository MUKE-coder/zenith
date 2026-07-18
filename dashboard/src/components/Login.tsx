import { useState } from 'react'
import type { FormEvent } from 'react'

import { useAuth } from '../lib/auth'
import styles from './Login.module.css'

/**
 * The developer console's sign-in.
 *
 * A split view: a brand panel that states what Zenith is, and the sign-in card
 * itself. On narrow screens the brand panel drops away and the card stands
 * alone, so the form is never crowded on a phone.
 */
export function Login() {
  const { signIn } = useAuth()

  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [error, setError] = useState<string>()
  const [submitting, setSubmitting] = useState(false)

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setError(undefined)
    setSubmitting(true)

    try {
      await signIn(email, password)
    } catch (err: unknown) {
      // The server's message already says what to do, in the right voice.
      setError(err instanceof Error ? err.message : 'Something went wrong. Try again.')
      setSubmitting(false)
    }
  }

  return (
    <div className={styles.wrap}>
      <aside className={styles.brandPanel}>
        <div className={styles.brandInner}>
          <div className={styles.panelBrand}>
            <span className={styles.mark} aria-hidden="true" />
            Zenith
          </div>

          <div className={styles.pitch}>
            <h2 className={styles.pitchTitle}>Analytics that stay yours.</h2>
            <p className={styles.pitchText}>
              Cookieless, self-hosted, and native to your own domain. Every pageview,
              referrer, and country in one calm console — with no third party in the middle.
            </p>
          </div>

          <div className={styles.preview} aria-hidden="true">
            <div className={styles.previewHead}>
              <span className={styles.previewLabel}>Unique visitors · 7d</span>
              <span className={styles.livePill}>
                <span className={styles.liveDot} />
                Live
              </span>
            </div>
            <div className={styles.previewValueRow}>
              <span className={styles.previewValue}>12,480</span>
              <span className={styles.previewDelta}>+18.2%</span>
            </div>
            <svg
              className={styles.spark}
              viewBox="0 0 320 72"
              preserveAspectRatio="none"
              fill="none"
            >
              <polyline
                points="0,56 40,50 80,54 120,38 160,44 200,26 240,30 280,14 320,20"
                stroke="currentColor"
                strokeWidth="2.5"
                strokeLinecap="round"
                strokeLinejoin="round"
              />
            </svg>
          </div>
        </div>
      </aside>

      <main className={styles.formPanel}>
        <div className={styles.card}>
          <div className={styles.cardBrand}>
            <span className={styles.mark} aria-hidden="true" />
            Zenith
          </div>

          <h1 className={styles.title}>Welcome back</h1>
          <p className={styles.subtitle}>Sign in to your analytics console.</p>

          <form onSubmit={onSubmit} noValidate>
            <div className={styles.field}>
              <label className="label" htmlFor="email">
                Email
              </label>
              <div className={styles.inputWrap}>
                <MailIcon />
                <input
                  id="email"
                  className={`input ${styles.withIcon}`}
                  type="email"
                  placeholder="you@example.com"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  autoComplete="username"
                  autoFocus
                  required
                />
              </div>
            </div>

            <div className={styles.field}>
              <label className="label" htmlFor="password">
                Password
              </label>
              <div className={styles.inputWrap}>
                <LockIcon />
                <input
                  id="password"
                  className={`input ${styles.withIcon} ${styles.withToggle}`}
                  type={showPassword ? 'text' : 'password'}
                  placeholder="••••••••"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  autoComplete="current-password"
                  required
                />
                <button
                  type="button"
                  className={styles.toggle}
                  onClick={() => setShowPassword((v) => !v)}
                  aria-label={showPassword ? 'Hide password' : 'Show password'}
                  aria-pressed={showPassword}
                  tabIndex={-1}
                >
                  {showPassword ? <EyeOffIcon /> : <EyeIcon />}
                </button>
              </div>
            </div>

            {error && (
              // Announced, because a sighted user sees it appear and a screen
              // reader user otherwise would not.
              <p className={styles.error} role="alert">
                {error}
              </p>
            )}

            <button
              type="submit"
              className={`button-primary ${styles.submit}`}
              disabled={submitting}
            >
              {submitting ? 'Signing in…' : 'Sign in'}
            </button>
          </form>
        </div>
      </main>
    </div>
  )
}

/* Inline icons — the console carries no icon dependency, so these are local. */

function MailIcon() {
  return (
    <svg className={styles.inputIcon} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <rect x="3" y="5" width="18" height="14" rx="2" />
      <path d="m3 7.5 9 6 9-6" />
    </svg>
  )
}

function LockIcon() {
  return (
    <svg className={styles.inputIcon} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <rect x="4" y="10" width="16" height="11" rx="2" />
      <path d="M8 10V7a4 4 0 0 1 8 0v3" />
    </svg>
  )
}

function EyeIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7Z" />
      <circle cx="12" cy="12" r="3" />
    </svg>
  )
}

function EyeOffIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M10.7 6.2A9.9 9.9 0 0 1 12 6c6.5 0 10 6 10 6a17.6 17.6 0 0 1-3 3.6M6.3 7.8A17.6 17.6 0 0 0 2 12s3.5 6 10 6a9.9 9.9 0 0 0 3.6-.7" />
      <path d="M9.9 9.9a3 3 0 0 0 4.2 4.2" />
      <path d="m3 3 18 18" />
    </svg>
  )
}
