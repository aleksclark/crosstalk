#!/usr/bin/env bash
# run-e2e.sh — Full SPA-driven end-to-end test with real audio.
#
# Builds the three SPAs, embeds them into ct-server, starts the server against a
# fresh PostgreSQL database, and runs the Playwright suite (admin + translator +
# broadcast flows, including the golden real-audio test).
#
# Requirements: Go, pnpm, a reachable PostgreSQL, and Playwright's Chromium.
#
# Env:
#   CT_TEST_DATABASE_ADMIN_URL  maintenance DSN for create/drop DB
#                               (default postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable)
#   CT_SERVER_PORT              server port (default 8080)
#   CT_ADMIN_PASSWORD           admin password (default admin)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

ADMIN_URL="${CT_TEST_DATABASE_ADMIN_URL:-postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable}"
PORT="${CT_SERVER_PORT:-8080}"
export CT_ADMIN_PASSWORD="${CT_ADMIN_PASSWORD:-admin}"
DBNAME="ct_e2e_$$"
SERVER_PID=""
SERVER_LOG="${CT_E2E_SERVER_LOG:-/tmp/ct-e2e-server.log}"

psql_admin() { psql "$ADMIN_URL" -v ON_ERROR_STOP=1 -qtA "$@"; }

cleanup() {
  set +e
  cd "$ROOT"
  [ -n "$SERVER_PID" ] && kill "$SERVER_PID" 2>/dev/null
  [ -n "$SERVER_PID" ] && wait "$SERVER_PID" 2>/dev/null
  # Restore the embed placeholders so the working tree stays clean.
  git checkout -- server/web/admin server/web/broadcast server/web/translator 2>/dev/null
  git clean -fdq server/web/admin server/web/broadcast server/web/translator 2>/dev/null
  psql_admin -c "DROP DATABASE IF EXISTS \"$DBNAME\" WITH (FORCE);" >/dev/null 2>&1
  echo "--- server log tail ---"
  tail -n 40 "$SERVER_LOG" 2>/dev/null || true
}
trap cleanup EXIT

echo "==> Generating OpenAPI spec + building SPAs"
(cd server && go run ./cmd/gen-openapi ../api/openapi.json)
pnpm install --no-frozen-lockfile >/dev/null
pnpm --filter @crosstalk/admin build >/dev/null
pnpm --filter @crosstalk/translator build >/dev/null
pnpm --filter @crosstalk/broadcast build >/dev/null

echo "==> Embedding SPAs into the server"
for app in admin broadcast translator; do
  rm -rf "server/web/$app"
  cp -r "apps/$app/dist" "server/web/$app"
done

echo "==> Building ct-server"
(cd server && CGO_ENABLED=0 go build -o "$ROOT/bin/ct-server-e2e" ./cmd/ct-server)

echo "==> Creating database $DBNAME"
psql_admin -c "CREATE DATABASE \"$DBNAME\";" >/dev/null
DB_URL="$(echo "$ADMIN_URL" | sed -E "s#/[^/?]+(\?|\$)#/$DBNAME\1#")"

echo "==> Starting ct-server on :$PORT"
CT_ADDR=":$PORT" \
  CT_DATABASE_URL="$DB_URL" \
  CT_JWT_SECRET="e2e-secret" \
  CT_UDP_MUX_PORT="${CT_UDP_MUX_PORT:-5000}" \
  "$ROOT/bin/ct-server-e2e" >"$SERVER_LOG" 2>&1 &
SERVER_PID=$!

echo "==> Waiting for the server"
for i in $(seq 1 60); do
  if (exec 3<>"/dev/tcp/localhost/$PORT") 2>/dev/null; then
    exec 3>&- 3<&-
    break
  fi
  sleep 0.5
done

echo "==> Running Playwright suite"
cd test/playwright
if [ ! -d node_modules ]; then
  npm install
fi
CT_SERVER_URL="http://localhost:$PORT" ./node_modules/.bin/playwright test "$@"
