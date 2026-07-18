import type { ReactNode } from 'react'

import styles from './Panel.module.css'

type PanelProps = {
  title: string
  action?: ReactNode
  children: ReactNode
}

/** A titled surface. Every panel on the dashboard is one of these. */
export function Panel({ title, action, children }: PanelProps) {
  return (
    <section className={styles.panel}>
      <header className={styles.header}>
        <h2 className={styles.title}>{title}</h2>
        {action && <div className={styles.action}>{action}</div>}
      </header>
      <div className={styles.body}>{children}</div>
    </section>
  )
}

type EmptyStateProps = {
  title: string
  hint?: string
  /** A glyph for the state. Sized and coloured by the wrapper. */
  icon?: ReactNode
  /** The way out: the one thing to do next, as a real control. */
  action?: ReactNode
}

/**
 * What a panel shows when there is nothing to show.
 *
 * The hint says what to do next, and where there is a single next step the
 * action makes it clickable. An empty state that only says "No data" tells the
 * reader they have a problem without telling them how to solve it.
 */
export function EmptyState({ title, hint, icon, action }: EmptyStateProps) {
  return (
    <div className={styles.empty}>
      {icon && (
        <span className={styles.emptyIcon} aria-hidden="true">
          {icon}
        </span>
      )}
      <p className={styles.emptyTitle}>{title}</p>
      {hint && <p className={styles.emptyHint}>{hint}</p>}
      {action && <div className={styles.emptyAction}>{action}</div>}
    </div>
  )
}

export function ErrorState({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div className={styles.error}>
      <p className={styles.errorMessage}>{message}</p>
      {onRetry && (
        <button type="button" className="button-secondary" onClick={onRetry}>
          Try again
        </button>
      )}
    </div>
  )
}

/** A quiet placeholder in the shape of the content it stands in for. */
export function Skeleton({ width = '100%', height = 16 }: { width?: string; height?: number }) {
  return <div className={styles.skeleton} style={{ width, height }} aria-hidden="true" />
}

/**
 * Rows of skeleton, sized like the table they precede.
 *
 * Widths vary because a stack of identical bars reads as a graphic; uneven
 * ones read as text that has not arrived.
 */
export function SkeletonRows({ rows = 5 }: { rows?: number }) {
  const widths = ['92%', '78%', '85%', '64%', '71%', '58%', '80%']

  return (
    <div className={styles.skeletonRows} aria-busy="true" aria-label="Loading">
      {Array.from({ length: rows }, (_, i) => (
        <div key={i} className={`${styles.skeleton} ${styles.skeletonRow}`} style={{ width: widths[i % widths.length] }} />
      ))}
    </div>
  )
}
