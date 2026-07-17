import { useEffect, useMemo, useRef, useState } from 'react'

import type { Site } from '../lib/types'
import styles from './SiteSwitcher.module.css'

type Props = {
  sites: Site[]
  selected: Site | undefined
  onSelect: (site: Site) => void
}

/** Searching only earns its keep once the list is long enough to scan. */
const SEARCH_THRESHOLD = 7

/**
 * The developer's site switcher.
 *
 * Rendered only for developers -- an owner has one site, and a switcher would
 * tell them other clients exist.
 */
export function SiteSwitcher({ sites, selected, onSelect }: Props) {
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const wrapRef = useRef<HTMLDivElement>(null)
  const searchRef = useRef<HTMLInputElement>(null)

  // A dropdown that ignores Escape and outside clicks is a trap.
  useEffect(() => {
    if (!open) return

    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false)
    }
    const onPointerDown = (e: PointerEvent) => {
      if (!wrapRef.current?.contains(e.target as Node)) setOpen(false)
    }

    document.addEventListener('keydown', onKeyDown)
    document.addEventListener('pointerdown', onPointerDown)
    return () => {
      document.removeEventListener('keydown', onKeyDown)
      document.removeEventListener('pointerdown', onPointerDown)
    }
  }, [open])

  useEffect(() => {
    if (open) searchRef.current?.focus()
    else setSearch('')
  }, [open])

  const filtered = useMemo(() => {
    const needle = search.trim().toLowerCase()
    if (!needle) return sites
    return sites.filter(
      (s) =>
        s.name.toLowerCase().includes(needle) || s.domain.toLowerCase().includes(needle),
    )
  }, [sites, search])

  const showSearch = sites.length >= SEARCH_THRESHOLD

  return (
    <div className={styles.wrap} ref={wrapRef}>
      <button
        type="button"
        className={styles.trigger}
        onClick={() => setOpen((v) => !v)}
        aria-haspopup="listbox"
        aria-expanded={open}
      >
        <span className={styles.triggerText}>{selected?.name ?? 'Select a site'}</span>
        <span className={styles.chevron} aria-hidden="true">
          ▼
        </span>
      </button>

      {open && (
        <div className={styles.menu} role="listbox" aria-label="Sites">
          {showSearch && (
            <input
              ref={searchRef}
              type="search"
              className={styles.search}
              placeholder="Search sites"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              aria-label="Search sites"
            />
          )}

          {filtered.length === 0 ? (
            <p className={styles.none}>No sites match “{search}”.</p>
          ) : (
            filtered.map((site) => (
              <button
                key={site.id}
                type="button"
                role="option"
                aria-selected={site.id === selected?.id}
                className={`${styles.option} ${site.id === selected?.id ? styles.optionActive : ''}`}
                onClick={() => {
                  onSelect(site)
                  setOpen(false)
                }}
              >
                <span className={styles.optionName}>{site.name}</span>
                <span className={styles.optionDomain}>{site.domain}</span>
              </button>
            ))
          )}
        </div>
      )}
    </div>
  )
}
