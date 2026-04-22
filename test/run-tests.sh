#!/usr/bin/env bash
# run-tests.sh — Entrypoint for the integration test container.
# Runs Go integration tests, then Playwright browser tests.
set -euo pipefail

CT_SERVER_URL="${CT_SERVER_URL:-http://server:8080}"

echo "=== Integration Test Runner ==="
echo "Server URL: ${CT_SERVER_URL}"

# ── Wait for server to be ready ──────────────────────────────────────
echo "Waiting for server to be healthy..."
for i in $(seq 1 60); do
  if wget -qO- "${CT_SERVER_URL}/" >/dev/null 2>&1; then
    echo "Server is ready (attempt ${i})"
    break
  fi
  if [ "$i" -eq 60 ]; then
    echo "ERROR: Server did not become ready in 60 seconds"
    exit 1
  fi
  sleep 1
done

# ── Run Playwright browser tests ────────────────────────────────────
echo ""
echo "=== Running Playwright browser tests ==="
cd /app/playwright
CT_SERVER_URL="${CT_SERVER_URL}" npx playwright test --reporter=list
PLAYWRIGHT_EXIT=$?

if [ $PLAYWRIGHT_EXIT -ne 0 ]; then
  echo "FAIL: Playwright tests failed"
  exit $PLAYWRIGHT_EXIT
fi

echo ""
echo "=== All integration tests passed ==="
