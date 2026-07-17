const ESCAPES: Record<string, string> = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;',
}

/**
 * Escapes a value for interpolation into HTML.
 *
 * The gate page renders the site name, which comes from config the owner
 * wrote. That is not hostile input, but it is interpolated into a page served
 * on their domain, and "trusted input" is how injection holes start.
 */
export function escapeHtml(value: string): string {
  return value.replace(/[&<>"']/g, (char) => ESCAPES[char] ?? char)
}
