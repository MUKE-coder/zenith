# Base image
FROM node:22-alpine AS base
RUN apk add --no-cache libc6-compat
RUN corepack enable && corepack prepare pnpm@10.12.1 --activate
ENV PNPM_HOME="/pnpm"
ENV PATH="$PNPM_HOME:$PATH"

# Dependencies
FROM base AS deps
WORKDIR /app
COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile --ignore-scripts

# Builder
FROM base AS builder
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .

# Dummy URL — prisma generate reads schema only, never connects
ENV DATABASE_URL="postgresql://build:build@localhost:5432/build"

# NEXT_PUBLIC_* vars (and any value read at module scope during static
# prerendering, like Zenith's site key) are baked in at `next build` time —
# ARG/ENV declared in another stage (e.g. deps) never carries over here,
# each FROM starts a fresh stage. Must be declared in THIS stage.
ARG NEXT_PUBLIC_API_BASE_URL
ENV NEXT_PUBLIC_API_BASE_URL=$NEXT_PUBLIC_API_BASE_URL
ARG ZENITH_URL
ARG ZENITH_SITE_KEY
ENV ZENITH_URL=$ZENITH_URL
ENV ZENITH_SITE_KEY=$ZENITH_SITE_KEY

RUN pnpm db:generate
RUN pnpm build

# Migrator — dedicated stage with full node_modules for running db push/seed
# Used by the migrate and seed services in docker-compose.prod.yml
FROM base AS migrator
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .

ENV DATABASE_URL="postgresql://build:build@localhost:5432/build"
RUN pnpm db:generate

# Runner — lean production image
FROM base AS runner
WORKDIR /app
ENV NODE_ENV=production

RUN addgroup --system --gid 1001 nodejs
RUN adduser --system --uid 1001 nextjs

RUN mkdir .next
RUN chown nextjs:nodejs .next

COPY --from=builder /app/public ./public
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static

USER nextjs

EXPOSE 3000
ENV PORT=3000
ENV HOSTNAME="0.0.0.0"

CMD ["node", "server.js"]
