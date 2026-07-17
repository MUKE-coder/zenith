# Zenith — Start Here (instructions for the AI)

You are building **Zenith**, a privacy-first, multi-site web analytics and SEO auditing platform
that a developer self-hosts once and shares with clients via domain-native dashboards.

## Read these first, in this order

1. **`project-description.md`** — the full vision, architecture, tech stack, and the definition of
   done. This is the source of truth for *what* Zenith is and *why* each decision was made. When
   an implementation choice is ambiguous, resolve it in favor of what this document describes.
2. **`phases.md`** — the ordered build plan. This is your task list. Work phase by phase, top to
   bottom. Do not jump ahead: each phase assumes the previous ones are solid and runnable.
3. **`style-guide.md`** — the visual system. Every piece of UI you build must conform to it:
   Geist + Geist Mono, neutral-first palette with a single blue accent, 4px spacing grid,
   hairline borders over shadows, the clean linear Vercel-grade look. Do not invent new colors,
   fonts, or spacing values.

## How to work

- **Follow the phases in order.** Start at Phase 0 and complete it before Phase 1. Within a phase,
  complete tasks top to bottom.
- **Mark progress in `phases.md`.** When a task is genuinely done — it works end to end, not just
  compiles — change its `[ ]` to `[x]`. Use `[~]` for a task you're mid-way through. Keep this file
  current; it is the shared record of where the build stands.
- **Ship runnable increments.** Every phase must end with something you can actually run and
  verify. Prefer a thin working slice over broad half-built scaffolding.
- **Respect the architecture.** Two Go services (`core` and `audit-worker`), DuckDB for events,
  SQLite for app data, storage behind `EventStore`/`AppStore` interfaces, the npm package as the
  domain-native proxy, a prebuilt SPA dashboard, Docker Compose to run it all. Do not collapse the
  audit worker into core, and do not put app/config data in DuckDB or events in SQLite.
- **Privacy is a hard constraint.** Tracking is cookieless with a daily-rotating visitor hash. No
  cookies, no fingerprinting, no raw IP storage, no persistent visitor identifier. Never add
  tracking that would require a consent banner.
- **Security basics always.** Hash passwords (bcrypt/argon2), never log secrets, expire JWTs,
  validate all input, scope every data query by site and enforce role.

## Before you start each phase

Briefly state: which phase you're on, the tasks you're about to do, and any assumption you're
making. Then build. Keep explanations short — the code and the checked-off tasks are the output.

## When you finish a phase

Summarize what now works, confirm the phase's tasks are checked off in `phases.md`, and note the
next phase. Then continue unless told otherwise.

## Ground rules

- If something in these files is ambiguous or seems to conflict, prefer `project-description.md`,
  then ask a short clarifying question rather than guessing on anything structural.
- Match complexity to the vision: this is a polished, minimal, precise product. Elegance is doing
  the chosen thing well, not adding more.
- Don't add features that aren't in `project-description.md` or `phases.md`. The non-goals and the
  "Later (post-v1)" list are deliberately out of scope for now.

Begin with **Phase 0**.
