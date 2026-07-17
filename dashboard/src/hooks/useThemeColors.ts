import { useEffect, useState } from 'react'

export type ThemeColors = {
  accent: string
  border: string
  muted: string
  subtle: string
  surface: string
  text: string
}

const FALLBACK: ThemeColors = {
  accent: '#2563EB',
  border: '#E4E4E7',
  muted: '#71717A',
  subtle: '#A1A1AA',
  surface: '#FAFAFA',
  text: '#0A0A0A',
}

function read(): ThemeColors {
  if (typeof window === 'undefined') return FALLBACK

  const style = getComputedStyle(document.documentElement)
  const value = (name: string, fallback: string) =>
    style.getPropertyValue(name).trim() || fallback

  return {
    accent: value('--accent', FALLBACK.accent),
    border: value('--border', FALLBACK.border),
    muted: value('--text-muted', FALLBACK.muted),
    subtle: value('--text-subtle', FALLBACK.subtle),
    surface: value('--surface', FALLBACK.surface),
    text: value('--text', FALLBACK.text),
  }
}

/**
 * Resolves the design tokens to concrete colors, for SVG.
 *
 * Charts are SVG, and `var()` inside a presentation attribute is not reliably
 * supported. Reading the computed values keeps tokens.css the single source of
 * truth rather than duplicating hexes into chart code, at the cost of having
 * to re-read them whenever the theme changes -- which is what the listeners
 * below are for.
 */
export function useThemeColors(): ThemeColors {
  const [colors, setColors] = useState<ThemeColors>(read)

  useEffect(() => {
    const update = () => setColors(read())

    // The OS theme changing under a page that has not opted out.
    const media = window.matchMedia('(prefers-color-scheme: dark)')
    media.addEventListener('change', update)

    // The in-app theme toggle, which stamps data-theme on <html>.
    const observer = new MutationObserver(update)
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['data-theme'],
    })

    return () => {
      media.removeEventListener('change', update)
      observer.disconnect()
    }
  }, [])

  return colors
}
