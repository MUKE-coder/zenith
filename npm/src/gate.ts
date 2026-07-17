import { escapeHtml } from './html.js'

/**
 * The password gate.
 *
 * One centered card: site name, one password field, one button. Nothing else.
 * It is served on the owner's own domain, so it has to feel calm and
 * trustworthy sitting on any brand -- which means neutral, not Zenith-branded.
 *
 * Self-contained HTML with inline styles rather than the SPA: the gate must
 * render before anyone is authenticated, so it cannot be behind the bundle it
 * is protecting.
 */
export function gatePage(options: {
  siteName: string
  action: string
  error?: string
}): string {
  const { siteName, action, error } = options

  return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex, nofollow">
<meta name="color-scheme" content="light dark">
<title>Analytics — ${escapeHtml(siteName)}</title>
<style>
  :root {
    --bg: #ffffff;
    --surface: #fafafa;
    --border: #e4e4e7;
    --text: #0a0a0a;
    --text-muted: #71717a;
    --accent: #2563eb;
    --accent-hover: #1d4ed8;
    --negative: #dc2626;
  }
  @media (prefers-color-scheme: dark) {
    :root {
      --bg: #0a0a0a;
      --surface: #111113;
      --border: #27272a;
      --text: #fafafa;
      --text-muted: #a1a1aa;
    }
  }
  * { box-sizing: border-box; }
  body {
    margin: 0;
    min-height: 100vh;
    display: grid;
    place-items: center;
    padding: 24px;
    background: var(--bg);
    color: var(--text);
    font-family: ui-sans-serif, system-ui, -apple-system, "Segoe UI", sans-serif;
    font-size: 15px;
    line-height: 1.5;
    -webkit-font-smoothing: antialiased;
  }
  .card {
    width: 100%;
    max-width: 360px;
    padding: 32px;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 8px;
  }
  h1 { margin: 0 0 4px; font-size: 1.5rem; font-weight: 600; line-height: 1.2; }
  .site { margin: 0 0 24px; color: var(--text-muted); }
  label { display: block; margin-bottom: 8px; font-size: 0.8125rem; font-weight: 500; color: var(--text-muted); }
  input {
    width: 100%;
    padding: 8px 12px;
    font-family: inherit;
    font-size: 15px;
    color: var(--text);
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 8px;
  }
  button {
    width: 100%;
    margin-top: 24px;
    padding: 12px;
    font-family: inherit;
    font-size: 15px;
    font-weight: 500;
    color: #fff;
    background: var(--accent);
    border: none;
    border-radius: 6px;
    cursor: pointer;
    transition: background-color 160ms ease-out;
  }
  button:hover { background: var(--accent-hover); }
  .error {
    margin: 16px 0 0;
    padding: 12px;
    font-size: 0.8125rem;
    color: var(--negative);
    background: rgba(220, 38, 38, 0.08);
    border-radius: 6px;
  }
  :focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  @media (prefers-reduced-motion: reduce) { button { transition: none; } }
</style>
</head>
<body>
  <main class="card">
    <h1>Analytics</h1>
    <p class="site">${escapeHtml(siteName)}</p>
    <form method="POST" action="${escapeHtml(action)}">
      <label for="password">Password</label>
      <input id="password" name="password" type="password" autocomplete="current-password" autofocus required>
      ${error ? `<p class="error" role="alert">${escapeHtml(error)}</p>` : ''}
      <button type="submit">View analytics</button>
    </form>
  </main>
</body>
</html>`
}
