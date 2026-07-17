# Zenith website

The marketing site and documentation for [Zenith](https://github.com/MUKE-coder/zenith) —
privacy-first, self-hosted web analytics. Built with Next.js 16, Tailwind 4, TypeScript, and
Shiki (for docs code highlighting).

## Develop

```bash
npm install
npm run dev
```

Open [http://localhost:3000](http://localhost:3000).

## Deploy with Docker

The site is self-contained — it needs no environment variables and no database.

```bash
# Build and run in one step
docker compose up --build

# Or by hand
docker build -t zenith/website .
docker run -p 3000:3000 zenith/website
```

Then open [http://localhost:3000](http://localhost:3000).

The image is built from Next.js [`output: "standalone"`](https://nextjs.org/docs/app/api-reference/config/next-config-js/output),
so it ships only the minimal server and its traced dependencies — no full `node_modules`, no
source. It runs as an unprivileged user and listens on port 3000 (`PORT` overrides it).

To publish it, push the image to any registry and run it on any container host — Fly.io, Cloud
Run, a VPS, Kubernetes, wherever:

```bash
docker build -t your-registry/zenith-website .
docker push your-registry/zenith-website
```

## Structure

| Path | What |
| --- | --- |
| `app/page.tsx` | Landing page |
| `app/docs/**` | Documentation (MDX) |
| `components/` | Shared UI — nav, footer, the ridgeline chart, docs sidebar/TOC |
| `lib/site.ts` | Site content (features, stats, links) |
| `lib/docs.ts` | Docs navigation tree |
| `app/globals.css` | Theme tokens — one blue accent, dark-first with a light variant |

The whole site's accent color runs through the `--accent` CSS variable in `globals.css`; change
it there to re-theme everything.
