# Zenith — Style Guide

The look is **clean, linear, Vercel-grade**: precise spacing, quiet surfaces, one confident
accent, data that reads instantly. This is a developer tool that clients also see, so it must
feel both technical and trustworthy. Restraint is the aesthetic — the polish comes from spacing,
type, and alignment being correct, not from decoration.

Every screen answers one question at a glance. If a panel doesn't help the reader understand
their traffic, it doesn't belong.

---

## 1. Principles

1. **Precision over ornament.** Generous, consistent spacing and perfect alignment do the work
   that borders and shadows do in lesser UIs.
2. **One accent, spent carefully.** Color is reserved for meaning (a metric, a positive/negative
   delta, a primary action). Everything else is neutral.
3. **Data is the hero.** Numbers are large and legible; chrome recedes. Charts are calm, not busy.
4. **Same-origin, native feel.** The owner's dashboard must look like it belongs to their site —
   neutral enough to sit comfortably on any brand.
5. **Quiet motion.** Transitions are short and functional. No decorative animation.

---

## 2. Typography

- **Display / UI / body: Geist** (`Geist`, sans-serif). One family carries almost everything —
  this is deliberate and part of the linear look.
- **Numbers / code / data: Geist Mono.** Use for metrics, tabular figures, keys, snippets, and
  anything that should align in columns. Enable tabular figures (`font-variant-numeric:
  tabular-nums`) wherever numbers update.

Type scale (rem, 16px base):

| Role            | Size    | Weight | Notes                                   |
|-----------------|---------|--------|-----------------------------------------|
| Display metric  | 2.25    | 600    | The big number on a summary card, mono  |
| H1 page title   | 1.5     | 600    | One per screen                          |
| H2 section      | 1.125   | 600    | Panel headers                           |
| Body            | 0.9375  | 400    | Default                                 |
| Label / caption | 0.8125  | 500    | Uppercase eyebrows, tracking +0.04em    |
| Data / mono     | 0.875   | 400    | Geist Mono, tabular-nums                |

Rules: sentence case everywhere except small uppercase eyebrow labels. Line-height 1.5 for body,
1.2 for large numbers. Never more than two weights (400, 600) plus 500 for labels.

---

## 3. Color

Neutral-first, one accent. Ship **light and dark**; dark is the developer's default, light is the
common owner default.

### Light
| Token              | Hex       | Use                                  |
|--------------------|-----------|--------------------------------------|
| `--bg`             | `#FFFFFF` | App background                       |
| `--surface`        | `#FAFAFA` | Cards, panels                        |
| `--surface-2`      | `#F4F4F5` | Nested / hover surfaces              |
| `--border`         | `#E4E4E7` | Hairline borders, dividers           |
| `--text`           | `#0A0A0A` | Primary text, big numbers           |
| `--text-muted`     | `#71717A` | Labels, secondary text              |
| `--text-subtle`    | `#A1A1AA` | Captions, disabled                  |

### Dark
| Token              | Hex       | Use                                  |
|--------------------|-----------|--------------------------------------|
| `--bg`             | `#0A0A0A` | App background                       |
| `--surface`        | `#111113` | Cards, panels                        |
| `--surface-2`      | `#18181B` | Nested / hover surfaces              |
| `--border`         | `#27272A` | Hairline borders, dividers           |
| `--text`           | `#FAFAFA` | Primary text, big numbers           |
| `--text-muted`     | `#A1A1AA` | Labels, secondary text              |
| `--text-subtle`    | `#71717A` | Captions, disabled                  |

### Accent & semantic (both themes)
| Token              | Hex       | Use                                  |
|--------------------|-----------|--------------------------------------|
| `--accent`         | `#2563EB` | Primary actions, active nav, key data|
| `--accent-hover`   | `#1D4ED8` | Accent hover                         |
| `--positive`       | `#16A34A` | Upward deltas, healthy scores        |
| `--negative`       | `#DC2626` | Downward deltas, errors, broken links|
| `--warning`        | `#D97706` | Audit warnings, degraded metrics     |

The accent is a single confident blue — the "zenith / sky / high point" cue, and it reads as a
serious developer tool rather than one of the warm-cream AI-default palettes. Use accent sparingly:
primary button, active nav item, the focused data series. Never color whole panels.

---

## 4. Spacing & layout

- **4px base grid.** Spacing steps: 4, 8, 12, 16, 24, 32, 48, 64.
- **Card padding:** 24px. **Section gap:** 32px. **Page gutter:** 32px desktop, 16px mobile.
- **Max content width:** ~1280px, centered, with a fixed left sidebar (240px) in the console.
- **Radius:** 8px on cards and inputs, 6px on buttons, 4px on small chips. Consistent and modest.
- **Borders over shadows.** Use 1px `--border` hairlines to separate. Shadows only for true
  overlays (menus, modals), and then very soft.
- **Grid:** summary cards in a responsive 4-up (desktop) → 2-up (tablet) → 1-up (mobile) grid.

Whitespace is the primary layout tool. When unsure, add space, not lines.

---

## 5. Components

**Summary card** — surface, 24px padding, hairline border, 8px radius. Uppercase eyebrow label
(muted), big mono number (`--text`), delta chip below (positive/negative color + arrow).

**Charts** — calm and minimal. Thin lines (1.5–2px), no heavy gridlines (use faint `--border`
horizontals only), muted axis labels, tooltip on hover. Primary series in `--accent`; comparison
series in a muted neutral. No gradients under areas unless very subtle and single-hue.

**Site switcher** — a compact dropdown in the top bar (developer only). Shows site name + domain;
searchable when the list is long.

**Buttons** — Primary: solid `--accent`, white text, 6px radius. Secondary: `--surface-2` fill,
`--text`, hairline border. Ghost: text-only for low-emphasis actions. One primary action per view.

**Tables** (top pages, referrers, audit results) — dense but breathable: 12px row padding, hairline
row dividers, mono for numeric columns with tabular-nums, right-aligned numbers.

**Audit report** — per-page rows with a score pill (green ≥90, amber 50–89, red <50), and an
expandable list of issues. Severity uses the semantic colors, never decoratively.

**Password gate (owner view)** — a single centered card: site name, one password field, one
primary button. Nothing else. Must feel calm and trustworthy on any host site.

**Empty & loading states** — every panel has both. Empty states say what to do next in the
interface's voice ("No data yet. Install the snippet to start collecting."), never a shrug.
Loading uses quiet skeletons, not spinners where avoidable.

---

## 6. Motion

- Transitions 120–200ms, ease-out. Hover/focus feedback only, plus content fade-in on load.
- No parallax, no bouncing, no decorative looping animation.
- Respect `prefers-reduced-motion`: disable non-essential transitions entirely.

---

## 7. Voice (UI copy)

- Sentence case, plain verbs, active voice. Buttons name the exact outcome ("Run audit," "Send
  report," "Add site").
- An action keeps its name through the flow: "Publish" → toast "Published."
- Errors explain what happened and how to fix it, in the interface's voice, no apologies, never
  vague. "Couldn't reach the sitemap at that URL. Check the domain and try again."
- Name things by what the user controls, not how the system is built. "Monthly report," not
  "cron job." "Tracking snippet," not "collector payload."

---

## 8. Accessibility (quality floor, non-negotiable)

- Text contrast meets WCAG AA against its surface.
- Visible keyboard focus on every interactive element (2px `--accent` ring, 2px offset).
- Full keyboard navigation; logical tab order; labelled form fields.
- Reduced motion respected. Responsive down to 360px.

---

## 9. The one signature

**The daily traffic ridgeline.** The primary chart is not a generic filled area — it's a crisp
single-hue line with a faint accent glow beneath, and the current day is marked by a subtle
vertical accent tick. It reads like an altitude profile climbing toward a peak — the "zenith."
This is the one place boldness is spent; everything around it stays quiet and disciplined.
