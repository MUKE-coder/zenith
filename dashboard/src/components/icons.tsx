import type { ReactNode } from 'react'

/**
 * The console's icon set.
 *
 * Drawn here rather than pulled from a package: the dashboard ships in the
 * core binary, and a dozen glyphs are not worth an icon dependency in the
 * bundle. All of them inherit currentColor and size, so a caller styles them
 * like text.
 */

function Svg({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <svg
      className={className}
      // A default size, so an icon dropped somewhere with no styling is a
      // glyph rather than a 300x150 block. Any CSS rule still overrides it.
      width="18"
      height="18"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.75"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      {children}
    </svg>
  )
}

/**
 * The Zenith mark: a line rising to its highest point.
 *
 * It reads as both a climb and a chart, which is the whole product in one
 * glyph. Filled tile so it holds up at 20px in a sidebar.
 */
export function ZenithMark({ size = 24, className }: { size?: number; className?: string }) {
  return (
    <svg
      className={className}
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      aria-hidden="true"
      focusable="false"
    >
      <rect width="24" height="24" rx="6" fill="var(--accent)" />
      <path
        d="M5.5 16.25 10 10.75l3 3.25 4.75-6"
        stroke="#fff"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <circle cx="17.9" cy="7.9" r="1.9" fill="#fff" />
    </svg>
  )
}

/* ── Navigation ──────────────────────────────────────────────────────────── */

export function IconOverview({ className }: { className?: string }) {
  return (
    <Svg className={className}>
      <path d="M3 21h18" />
      <path d="M6.5 21v-5.5" />
      <path d="M12 21V7" />
      <path d="M17.5 21v-9" />
    </Svg>
  )
}

export function IconEvents({ className }: { className?: string }) {
  return (
    <Svg className={className}>
      <path d="M3 12h4l3-8 4 16 3-8h4" />
    </Svg>
  )
}

export function IconSeo({ className }: { className?: string }) {
  return (
    <Svg className={className}>
      <circle cx="11" cy="11" r="7" />
      <path d="m20 20-3.6-3.6" />
      <path d="M8.5 11.5 10.5 13l3.5-3.5" />
    </Svg>
  )
}

export function IconSetup({ className }: { className?: string }) {
  return (
    <Svg className={className}>
      <rect x="3" y="4" width="18" height="16" rx="2" />
      <path d="m7.5 9.5 2.5 2.5-2.5 2.5" />
      <path d="M13 14.5h3.5" />
    </Svg>
  )
}

export function IconSettings({ className }: { className?: string }) {
  return (
    <Svg className={className}>
      <path d="M4 7h16" />
      <path d="M4 12h16" />
      <path d="M4 17h16" />
      <circle cx="9" cy="7" r="2" fill="currentColor" stroke="none" />
      <circle cx="15" cy="12" r="2" fill="currentColor" stroke="none" />
      <circle cx="8" cy="17" r="2" fill="currentColor" stroke="none" />
    </Svg>
  )
}

/* ── Actions ─────────────────────────────────────────────────────────────── */

export function IconSignOut({ className }: { className?: string }) {
  return (
    <Svg className={className}>
      <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
      <path d="m16 16 4-4-4-4" />
      <path d="M20 12H9" />
    </Svg>
  )
}

export function IconPlus({ className }: { className?: string }) {
  return (
    <Svg className={className}>
      <path d="M12 5v14" />
      <path d="M5 12h14" />
    </Svg>
  )
}

export function IconDownload({ className }: { className?: string }) {
  return (
    <Svg className={className}>
      <path d="M12 3v12" />
      <path d="m7.5 10.5 4.5 4.5 4.5-4.5" />
      <path d="M4 20h16" />
    </Svg>
  )
}

export function IconChevronDown({ className }: { className?: string }) {
  return (
    <Svg className={className}>
      <path d="m6 9.5 6 6 6-6" />
    </Svg>
  )
}

export function IconEye({ className }: { className?: string }) {
  return (
    <Svg className={className}>
      <path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7Z" />
      <circle cx="12" cy="12" r="3" />
    </Svg>
  )
}

export function IconEyeOff({ className }: { className?: string }) {
  return (
    <Svg className={className}>
      <path d="M10.7 6.2A9.9 9.9 0 0 1 12 6c6.5 0 10 6 10 6a17.6 17.6 0 0 1-3 3.6M6.3 7.8A17.6 17.6 0 0 0 2 12s3.5 6 10 6a9.9 9.9 0 0 0 3.6-.7" />
      <path d="M9.9 9.9a3 3 0 0 0 4.2 4.2" />
      <path d="m3 3 18 18" />
    </Svg>
  )
}

/**
 * The sidebar collapse control: a panel with its divider, and an arrow through
 * it. One glyph for both directions -- the CSS flips it -- because two mirrored
 * icons would be two things to keep in step for no gain.
 */
export function IconSidebar({ className }: { className?: string }) {
  return (
    <Svg className={className}>
      <rect x="3" y="4" width="18" height="16" rx="2" />
      <path d="M9 4v16" />
    </Svg>
  )
}

/* ── Empty states ────────────────────────────────────────────────────────── */

export function IconGlobe({ className }: { className?: string }) {
  return (
    <Svg className={className}>
      <circle cx="12" cy="12" r="9" />
      <path d="M3 12h18" />
      <path d="M12 3c2.5 2.4 3.9 5.6 3.9 9S14.5 18.6 12 21c-2.5-2.4-3.9-5.6-3.9-9S9.5 5.4 12 3Z" />
    </Svg>
  )
}

export function IconChart({ className }: { className?: string }) {
  return (
    <Svg className={className}>
      <path d="M3 20h18" />
      <path d="m5 15 4.5-5 3.5 3.5L19 7" />
    </Svg>
  )
}

export function IconClock({ className }: { className?: string }) {
  return (
    <Svg className={className}>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7.5V12l3 2" />
    </Svg>
  )
}

export function IconCursor({ className }: { className?: string }) {
  return (
    <Svg className={className}>
      <path d="m5 4 5.5 15 2.2-6.3L19 10.5 5 4Z" />
    </Svg>
  )
}
