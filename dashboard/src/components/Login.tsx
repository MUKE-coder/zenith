import { useState } from 'react'
import type { FormEvent } from 'react'

import { useAuth } from '../lib/auth'
import styles from './Login.module.css'

/**
 * The developer console's sign-in.
 *
 * The owner's password gate is the same card with one field, served on their
 * own domain by the npm proxy in phase 7.
 */
export function Login() {
  const { signIn } = useAuth()

  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
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
    <main className={styles.wrap}>
      <div className={styles.card}>
        <div className={styles.brand}>
          <span className={styles.mark} aria-hidden="true" />
          Zenith
        </div>

        <h1 className={styles.title}>Sign in</h1>
        <p className={styles.subtitle}>Your analytics console.</p>

        <form onSubmit={onSubmit} noValidate>
          <div className={styles.field}>
            <label className="label" htmlFor="email">
              Email
            </label>
            <input
              id="email"
              className="input"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              autoComplete="username"
              autoFocus
              required
            />
          </div>

          <div className={styles.field}>
            <label className="label" htmlFor="password">
              Password
            </label>
            <input
              id="password"
              className="input"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="current-password"
              required
            />
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
  )
}
