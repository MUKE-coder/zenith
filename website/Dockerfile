# Zenith website — production image.
#
# Multi-stage build on Next.js standalone output: the final image ships only
# the minimal server and its traced dependencies, not the full node_modules or
# the source. See node_modules/next/dist/docs for the standalone-output guide.

# ─── deps: install dependencies against the lockfile ─────────────────────────
FROM node:22-alpine AS deps
WORKDIR /app
COPY package.json package-lock.json ./
# Reproducible install from the lockfile; no audit/fund noise.
RUN npm ci

# ─── builder: produce the standalone build ───────────────────────────────────
FROM node:22-alpine AS builder
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .
# Next telemetry is opt-out; keep the build quiet and offline.
ENV NEXT_TELEMETRY_DISABLED=1
RUN npm run build

# ─── runner: the minimal production image ────────────────────────────────────
FROM node:22-alpine AS runner
WORKDIR /app

ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
# The standalone server reads these; bind all interfaces so the container is
# reachable from outside.
ENV PORT=3000
ENV HOSTNAME=0.0.0.0

# Run unprivileged — nothing here needs root.
RUN addgroup --system --gid 1001 nodejs \
  && adduser --system --uid 1001 nextjs

# The standalone output already contains the traced node_modules and server.js.
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
# standalone does NOT include these two — copy them in so server.js can serve
# static assets and public files without a separate CDN.
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static
COPY --from=builder --chown=nextjs:nodejs /app/public ./public

USER nextjs
EXPOSE 3000

# server.js is Next's minimal standalone entrypoint.
CMD ["node", "server.js"]
