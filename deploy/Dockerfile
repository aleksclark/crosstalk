# syntax=docker/dockerfile:1

# ── Stage 1: build the frontend SPAs ───────────────────────────────
FROM node:22-bookworm AS webbuild

RUN npm install -g pnpm@10.10.0

WORKDIR /src
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY packages/ ./packages/
COPY apps/ ./apps/
COPY api/ ./api/

RUN pnpm install --no-frozen-lockfile
RUN pnpm --filter @crosstalk/admin build \
    && pnpm --filter @crosstalk/broadcast build \
    && pnpm --filter @crosstalk/translator build

# ── Stage 2: build the Go server (embeds the SPA dist output) ──────
FROM golang:1.25-bookworm AS build

RUN apt-get update && apt-get install -y --no-install-recommends gcc sqlite3

WORKDIR /app
COPY server/go.mod server/go.sum ./server/
COPY proto/gen/go/go.mod proto/gen/go/go.sum ./proto/gen/go/
RUN cd server && go mod download

COPY server/ ./server/
COPY proto/ ./proto/

# Overlay the built SPAs into the embed directories (replacing placeholders).
COPY --from=webbuild /src/apps/admin/dist/ ./server/web/admin/
COPY --from=webbuild /src/apps/broadcast/dist/ ./server/web/broadcast/
COPY --from=webbuild /src/apps/translator/dist/ ./server/web/translator/

RUN cd server && CGO_ENABLED=1 go build -o /app/ct-server ./cmd/ct-server

# ── Stage 3: runtime ───────────────────────────────────────────────
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates sqlite3 curl \
    && rm -rf /var/lib/apt/lists/*

COPY --from=build /app/ct-server /usr/local/bin/ct-server
COPY deploy/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

EXPOSE 8080
EXPOSE 5000/udp

ENTRYPOINT ["entrypoint.sh"]
