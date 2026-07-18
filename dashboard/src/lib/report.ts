import type { AuditCheck, AuditDetail, AuditPage } from './types'

/**
 * Turns an audit into a document you can act on somewhere else.
 *
 * The console is for reading a result; this is for taking it away -- into an
 * editor, an issue tracker, or a model you ask to fix the findings. So it
 * leads with what is wrong and how often, and orders pages worst-first, which
 * is the order anyone would actually work through them.
 */
export function auditMarkdown(detail: AuditDetail): string {
  const { audit, pages } = detail
  const host = hostOf(pages)

  // Worst first: that is the order the work gets done in.
  const ranked = [...pages].sort((a, b) => a.score - b.score)

  const errors = countBySeverity(pages, 'error')
  const warnings = countBySeverity(pages, 'warning')

  const out: string[] = []

  out.push(`# SEO audit — ${host}`)
  out.push('')
  out.push(`- **Site score:** ${audit.score} / 100`)
  out.push(`- **Pages audited:** ${pages.length}`)
  if (audit.finished_at) out.push(`- **Finished:** ${new Date(audit.finished_at).toISOString()}`)
  out.push(`- **Findings:** ${errors} ${plural(errors, 'error')}, ${warnings} ${plural(warnings, 'warning')}`)
  out.push('')

  const grouped = groupByCheck(pages)
  if (grouped.length > 0) {
    out.push('## What to fix first')
    out.push('')
    out.push('Ordered by how many pages each problem affects.')
    out.push('')
    out.push('| Pages | Severity | Problem |')
    out.push('| --- | --- | --- |')
    for (const g of grouped) {
      out.push(`| ${g.pages} | ${g.severity} | ${escapePipes(g.message)} |`)
    }
    out.push('')
  }

  out.push('## Pages')
  out.push('')

  for (const page of ranked) {
    const failing = page.checks.checks.filter((c) => c.severity !== 'ok')

    out.push(`### ${page.score} — ${pathOf(page.url)}`)
    out.push('')
    out.push(page.url)
    out.push('')

    if (page.checks.title) out.push(`- **Title:** ${page.checks.title}`)
    if (page.checks.description) out.push(`- **Description:** ${page.checks.description}`)

    const v = page.checks.vitals
    out.push(
      `- **Vitals:** TTFB ${ms(v.ttfb_ms)} · FCP ${ms(v.fcp_ms)} · LCP ${ms(v.lcp_ms)} · ` +
        `CLS ${v.cls.toFixed(3)} · Load ${ms(v.load_ms)}`,
    )
    out.push('')

    if (failing.length === 0) {
      out.push('No issues found.')
      out.push('')
      continue
    }

    for (const severity of ['error', 'warning'] as const) {
      const items = failing.filter((c) => c.severity === severity)
      if (items.length === 0) continue

      out.push(`**${severity === 'error' ? 'Errors' : 'Warnings'}**`)
      out.push('')
      for (const check of items) {
        out.push(`- ${check.message}`)
        // Details carry the specifics -- which links broke, what the title
        // actually said -- and are the part a fix needs.
        if (check.detail) {
          for (const line of check.detail.split('\n')) {
            if (line.trim()) out.push(`  - ${line.trim()}`)
          }
        }
      }
      out.push('')
    }
  }

  return out.join('\n')
}

type Grouped = { id: string; message: string; severity: string; pages: number }

/** How many pages each distinct check fails on. */
function groupByCheck(pages: AuditPage[]): Grouped[] {
  const seen = new Map<string, Grouped>()

  for (const page of pages) {
    for (const check of page.checks.checks) {
      if (check.severity === 'ok') continue

      const existing = seen.get(check.id)
      if (existing) {
        existing.pages += 1
      } else {
        seen.set(check.id, {
          id: check.id,
          message: check.message,
          severity: check.severity,
          pages: 1,
        })
      }
    }
  }

  return [...seen.values()].sort(
    (a, b) => b.pages - a.pages || (a.severity === 'error' ? -1 : 1),
  )
}

function countBySeverity(pages: AuditPage[], severity: AuditCheck['severity']): number {
  return pages.reduce(
    (total, page) => total + page.checks.checks.filter((c) => c.severity === severity).length,
    0,
  )
}

function hostOf(pages: AuditPage[]): string {
  const first = pages[0]?.url
  if (!first) return 'site'
  try {
    return new URL(first).host
  } catch {
    return 'site'
  }
}

function pathOf(url: string): string {
  try {
    return new URL(url).pathname || '/'
  } catch {
    return url
  }
}

function ms(value: number): string {
  return value >= 1000 ? `${(value / 1000).toFixed(2)}s` : `${Math.round(value)}ms`
}

function plural(n: number, word: string): string {
  return n === 1 ? word : `${word}s`
}

/** A pipe inside a table cell would end the column early. */
function escapePipes(value: string): string {
  return value.replace(/\|/g, '\\|')
}

/**
 * Hands a generated file to the browser.
 *
 * The object URL is revoked afterwards: without it every download leaks the
 * whole report until the tab closes.
 */
export function downloadFile(filename: string, content: string, mime: string): void {
  const url = URL.createObjectURL(new Blob([content], { type: mime }))
  const link = document.createElement('a')

  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)

  URL.revokeObjectURL(url)
}

/** A filename that sorts by date and says what it is. */
export function auditFilename(detail: AuditDetail, extension: string): string {
  const host = hostOf(detail.pages).replace(/[^a-z0-9.-]/gi, '-')
  const when = (detail.audit.finished_at ?? detail.audit.requested_at ?? '').slice(0, 10)
  return `seo-audit-${host}${when ? `-${when}` : ''}.${extension}`
}
