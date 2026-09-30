# syntax=docker/dockerfile:1.7
#
# Build optimisation notes:
#  - bookworm-slim (glibc) вместо alpine (musl): Next.js загружает нативный
#    @next/swc-linux-x64-gnu бинарь. Alpine вынуждал fallback на WASM
#    (@next/swc-wasm-nodejs) — каждый next build занимал 20+ мин вместо 2-3.
#  - Cache mount on /root/.npm survives between builds → ~150s install → ~10s.
#  - npm fetch retry env + locked cache mount make CI deploys more resilient
#    against transient registry resets and cache cleanup races.
#  - Runner stage reuses node_modules from deps and `npm prune --omit=dev`
#    instead of a second full `npm ci` → ~150s saved.
#  - Layer order: copy package*.json BEFORE source so deps cache survives
#    code changes (unchanged lockfile = cached deps layer).
#  - BuildKit cache mount on /app/.next/cache persists Next.js SWC/webpack
#    incremental compilation artefacts inside Docker → subsequent builds only
#    recompile changed pages (warm cache: ~2-3 min vs ~24 min cold).
#  - NEXT_TELEMETRY_DISABLED=1 in the build stage eliminates the telemetry
#    network call during `next build`, saving a few seconds and removing a
#    spurious network dependency.
FROM node:22-bookworm-slim AS deps
WORKDIR /app
ENV NPM_CONFIG_FETCH_RETRIES=5 \
    NPM_CONFIG_FETCH_RETRY_MINTIMEOUT=20000 \
    NPM_CONFIG_FETCH_RETRY_MAXTIMEOUT=120000 \
    NPM_CONFIG_AUDIT=false \
    NPM_CONFIG_FUND=false
COPY package.json package-lock.json* ./
RUN --mount=type=cache,id=mindstrata-web-npm,target=/root/.npm,sharing=locked \
    for attempt in 1 2 3; do \
      npm ci --prefer-offline --no-audit --fund=false && break; \
      if [ "$attempt" = "3" ]; then exit 1; fi; \
      npm cache verify || true; \
      sleep $((attempt * 10)); \
    done

FROM node:22-bookworm-slim AS build
WORKDIR /app
ARG NEXT_PUBLIC_API_BASE_URL=http://localhost:18080
ENV NEXT_PUBLIC_API_BASE_URL=$NEXT_PUBLIC_API_BASE_URL \
    NEXT_TELEMETRY_DISABLED=1
COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN --mount=type=cache,id=mindstrata-web-next,target=/app/.next/cache,sharing=locked \
    UV_THREADPOOL_SIZE=$(nproc) npm run build

FROM node:22-bookworm-slim AS runner
WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
# standalone: копируем только то что нужно серверу (server.js + minimal node_modules).
# npm prune не нужен — node_modules уже минимальны благодаря output:'standalone'.
COPY --from=build /app/.next/standalone ./
COPY --from=build /app/.next/static ./.next/static
COPY --from=build /app/public ./public
EXPOSE 3000
CMD ["node", "server.js"]
