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
#   CT_SERVER_PORT              server port (default: first free port, preferring 18080)
#   CT_ADMIN_PASSWORD           admin password (default admin-e2e-pass)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

ADMIN_URL="${CT_TEST_DATABASE_ADMIN_URL:-postgres://postgres:postgres@127.0.0.1:55432/postgres?sslmode=disable}"
# Prefer an explicit port; otherwise pick a free high port so a foreign :8080
# listener (common on shared workstations) cannot hijack the SPA suite.
if [ -n "${CT_SERVER_PORT:-}" ]; then
  PORT="$CT_SERVER_PORT"
else
  PORT="$(python3 - <<'PY'
import socket
for port in (18080, 18081, 18082, 28080, 0):
    s = socket.socket()
    try:
        s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        s.bind(("127.0.0.1", port))
        print(s.getsockname()[1])
        s.close()
        break
    except OSError:
        s.close()
PY
)"
fi
# Single source of truth for admin password (server seed + Playwright helpers).
export CT_ADMIN_PASSWORD="${CT_ADMIN_PASSWORD:-admin-e2e-pass}"
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
(cd server && CGO_ENABLED=1 go run ./cmd/gen-openapi ../api/openapi.json)
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
(cd server && CGO_ENABLED=1 go build -o "$ROOT/bin/ct-server-e2e" ./cmd/ct-server)

echo "==> Creating database $DBNAME"
psql_admin -c "CREATE DATABASE \"$DBNAME\";" >/dev/null
DB_URL="$(echo "$ADMIN_URL" | sed -E "s#/[^/?]+(\\?|\$)#/$DBNAME\\1#")"

echo "==> Starting ct-server on :$PORT"
CT_ADDR=":$PORT" \
  CT_DATABASE_URL="$DB_URL" \
  CT_JWT_SECRET="e2e-secret-not-default" \
  CT_ADMIN_PASSWORD="$CT_ADMIN_PASSWORD" \
  CT_INSTANCE_ID="e2e-$$" \
  CT_TEST_MODE=1 \
  CT_UDP_MUX_PORT="${CT_UDP_MUX_PORT:-0}" \
  "$ROOT/bin/ct-server-e2e" >"$SERVER_LOG" 2>&1 &
SERVER_PID=$!

echo "==> Waiting for the server"
ready=0
for i in $(seq 1 60); do
  if ! kill -0 "$SERVER_PID" 2>/dev/null; then
    echo "ct-server exited early; log:" >&2
    cat "$SERVER_LOG" >&2 || true
    exit 1
  fi
  if (exec 3<>"/dev/tcp/127.0.0.1/$PORT") 2>/dev/null; then
    exec 3>&- 3<&-
    ready=1
    break
  fi
  sleep 0.5
done
if [ "$ready" != 1 ]; then
  echo "ct-server did not become ready on :$PORT" >&2
  cat "$SERVER_LOG" >&2 || true
  exit 1
fi

# Prove we are talking to CrossTalk, not a foreign listener on the same port.
health="$(curl -fsS "http://127.0.0.1:$PORT/admin/" | head -c 400 || true)"
if ! printf '%s' "$health" | rg -qi 'crosstalk|admin|root'; then
  echo "unexpected response from :$PORT (not CrossTalk admin SPA?)" >&2
  printf '%s\n' "$health" >&2
  exit 1
fi

echo "==> Running Playwright suite against http://127.0.0.1:$PORT"
cd test/playwright
if [ ! -d node_modules ]; then
  npm install
fi
CT_SERVER_URL="http://127.0.0.1:$PORT" \
  CT_ADMIN_PASSWORD="$CT_ADMIN_PASSWORD" \
  ./node_modules/.bin/playwright test "$@"
