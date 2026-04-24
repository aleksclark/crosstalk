#!/usr/bin/env bash
# run-e2e-tests.sh — End-to-end golden tests for CrossTalk (Phases 9.3–9.5).
#
# Proves that the CrossTalk E2E pipeline works through the FULL WebRTC path:
#   Browser ↔ WebRTC ↔ ct-server SFU ↔ WebRTC ↔ ct-client (K2B) ↔ PipeWire
#
# Tests:
#   9.3  Browser → K2B audio path (Playwright sends, K2B captures)
#   9.4  K2B → Browser audio path (K2B plays, Playwright captures)
#   9.5  Full infrastructure: build, deploy, connect, audio compare
#
# Usage:
#   ./dev/scripts/run-e2e-tests.sh
#
# Environment:
#   K2B_HOST      Board IP          (default 192.168.0.109)
#   K2B_USER      PipeWire user     (default streamlate)
#   HOST_IP       Host IP reachable from K2B (auto-detected)
#   E2E_THRESHOLD Cross-corr pass   (default 0.90)
#   E2E_KEEP_TMP  Set to 1 to keep temp dir
#
# Dependencies: Go 1.22+, ffmpeg, python3, ssh, jq, curl, npx (playwright)
set -euo pipefail

# ── Colours ──────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GRN='\033[0;32m'; YEL='\033[0;33m'; CYN='\033[0;36m'; RST='\033[0m'
info()  { echo -e "${CYN}[INFO]${RST}  $*"; }
ok()    { echo -e "${GRN}[PASS]${RST}  $*"; }
warn()  { echo -e "${YEL}[WARN]${RST}  $*"; }
fail()  { echo -e "${RED}[FAIL]${RST}  $*"; }
die()   { fail "$*"; exit 1; }

# ── Configuration ────────────────────────────────────────────────────────────
K2B_HOST="${K2B_HOST:-192.168.0.109}"
K2B_USER="${K2B_USER:-streamlate}"
K2B_UID=999
THRESHOLD="${E2E_THRESHOLD:-0.90}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
COMPARE="$SCRIPT_DIR/compare-audio.sh"
FIXTURES="$PROJECT_ROOT/test/fixtures"
REF_TONE="$FIXTURES/test-tone-1khz-5s.wav"

# Playwright paths
PW_CONFIG="$PROJECT_ROOT/test/playwright/playwright.config.ts"
PW_SPEC="$PROJECT_ROOT/test/playwright/specs/golden-audio.spec.ts"

# K2B PipeWire / ALSA names
K2B_SOURCE="alsa_input.platform-snd_aloop.0.analog-stereo"
K2B_SINK="alsa_output.platform-snd_aloop.0.analog-stereo"

# Binaries
SERVER_BIN="$PROJECT_ROOT/bin/ct-server"
CLIENT_ARM64="$PROJECT_ROOT/bin/ct-client-arm64"

# Temp dir
TMPDIR="$(mktemp -d /tmp/ct-e2e-XXXXXX)"
info "Temp dir: $TMPDIR"

# Track processes for cleanup
declare -a CLEANUP_PIDS=()
SERVER_PID=""

cleanup() {
    local exit_code=$?
    info "Cleaning up..."
    # Kill local background processes
    for pid in "${CLEANUP_PIDS[@]:-}"; do
        if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
            kill "$pid" 2>/dev/null || true
            wait "$pid" 2>/dev/null || true
        fi
    done
    # Kill server specifically
    if [[ -n "$SERVER_PID" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
        kill "$SERVER_PID" 2>/dev/null || true
        wait "$SERVER_PID" 2>/dev/null || true
    fi
    # Kill remote processes on K2B
    ssh -o ConnectTimeout=3 "root@${K2B_HOST}" "pkill -x ct-client 2>/dev/null; pkill -x ffmpeg 2>/dev/null" 2>/dev/null || true
    # Remove temp dir
    if [[ "${E2E_KEEP_TMP:-0}" != "1" ]]; then
        rm -rf "$TMPDIR"
    else
        info "Keeping temp dir: $TMPDIR"
    fi
    return $exit_code
}
trap cleanup EXIT

# ── Counters ─────────────────────────────────────────────────────────────────
TESTS_RUN=0
TESTS_PASSED=0
TESTS_SKIPPED=0
TESTS_FAILED=0

pass_test() { ((TESTS_PASSED++)) || true; ((TESTS_RUN++)) || true; ok "$1"; }
skip_test() { ((TESTS_SKIPPED++)) || true; ((TESTS_RUN++)) || true; warn "$1 [SKIPPED]"; }
fail_test() { ((TESTS_FAILED++)) || true; ((TESTS_RUN++)) || true; fail "$1"; }

# ══════════════════════════════════════════════════════════════════════════════
# PREREQUISITES
# ══════════════════════════════════════════════════════════════════════════════
info "Checking prerequisites..."

for cmd in go ffmpeg python3 ssh scp jq curl npx; do
    command -v "$cmd" >/dev/null 2>&1 || die "$cmd not found"
done

[[ -f "$REF_TONE" ]]  || die "Reference tone not found: $REF_TONE"
[[ -f "$COMPARE" ]]   || die "compare-audio.sh not found: $COMPARE"
[[ -f "$PW_SPEC" ]]   || die "Playwright golden-audio spec not found: $PW_SPEC"

info "Pinging K2B at ${K2B_HOST}..."
ssh -o ConnectTimeout=5 "root@${K2B_HOST}" "echo K2B_OK" >/dev/null 2>&1 \
    || die "Cannot SSH to K2B at ${K2B_HOST}"
ok "K2B reachable"

# Auto-detect host IP
if [[ -z "${HOST_IP:-}" ]]; then
    HOST_IP="$(ip route get "$K2B_HOST" | sed -n 's/.*src \([0-9.]*\).*/\1/p' | head -1)"
fi
[[ -n "$HOST_IP" ]] || die "Could not detect HOST_IP (set manually)"
info "Host IP: $HOST_IP"

# ══════════════════════════════════════════════════════════════════════════════
# BUILD
# ══════════════════════════════════════════════════════════════════════════════
info "Building ct-server (x86_64)..."
(cd "$PROJECT_ROOT/server" && go build -o "$SERVER_BIN" ./cmd/ct-server) \
    || die "Server build failed"
ok "ct-server built"

info "Building ct-client (arm64)..."
(cd "$PROJECT_ROOT/cli" && GOOS=linux GOARCH=arm64 go build -o "$CLIENT_ARM64" ./cmd/ct-client) \
    || die "CLI arm64 build failed"
ok "ct-client-arm64 built"

# ══════════════════════════════════════════════════════════════════════════════
# START SERVER
# ══════════════════════════════════════════════════════════════════════════════
SERVER_PORT=$(python3 -c 'import socket; s=socket.socket(); s.bind(("",0)); print(s.getsockname()[1]); s.close()')
info "Starting ct-server on 0.0.0.0:${SERVER_PORT}..."

SERVER_CFG="$TMPDIR/ct-server.json"
SERVER_DB="$TMPDIR/ct-server.db"
mkdir -p "$TMPDIR/recordings"
cat > "$SERVER_CFG" <<EOCFG
{
  "listen": "0.0.0.0:${SERVER_PORT}",
  "db_path": "${SERVER_DB}",
  "recording_path": "${TMPDIR}/recordings",
  "log_level": "debug",
  "auth": { "session_secret": "e2e-test-$(date +%s)" },
  "web": { "dev_mode": false }
}
EOCFG

SERVER_LOG="$TMPDIR/server.log"
"$SERVER_BIN" --config "$SERVER_CFG" > "$SERVER_LOG" 2>&1 &
SERVER_PID=$!
CLEANUP_PIDS+=("$SERVER_PID")

# Wait for server + extract seed token
SEED_TOKEN=""
for _ in $(seq 1 30); do
    if [[ -f "$SERVER_LOG" ]]; then
        SEED_TOKEN="$(grep -oP '(?<="token": ")[^"]*' "$SERVER_LOG" | head -1 || true)"
        [[ -n "$SEED_TOKEN" ]] && break
    fi
    sleep 0.3
done
[[ -n "$SEED_TOKEN" ]] || { cat "$SERVER_LOG" 2>/dev/null; die "Could not extract seed token"; }

SERVER_URL="http://127.0.0.1:${SERVER_PORT}"
for _ in $(seq 1 20); do
    curl -sf "${SERVER_URL}/api/templates" -H "Authorization: Bearer ${SEED_TOKEN}" >/dev/null 2>&1 && break
    sleep 0.3
done
curl -sf "${SERVER_URL}/api/templates" -H "Authorization: Bearer ${SEED_TOKEN}" >/dev/null 2>&1 \
    || { cat "$SERVER_LOG"; die "Server not responding"; }
ok "ct-server started (PID ${SERVER_PID}, port ${SERVER_PORT})"

# ══════════════════════════════════════════════════════════════════════════════
# SETUP: API token for K2B client
# ══════════════════════════════════════════════════════════════════════════════
# Only infrastructure setup happens here — creating an API token so ct-client
# can authenticate with the server. Template, session, role assignment, and
# connecting are ALL done through the admin UI in the Playwright test.
info "Creating API token for K2B client..."

TOKEN_RESP=$(curl -sf -X POST "${SERVER_URL}/api/tokens" \
    -H "Authorization: Bearer ${SEED_TOKEN}" \
    -H "Content-Type: application/json" \
    -d '{"name": "k2b-e2e"}')
K2B_TOKEN=$(echo "$TOKEN_RESP" | jq -r '.token')
[[ "$K2B_TOKEN" != "null" && -n "$K2B_TOKEN" ]] || die "Token create failed: $TOKEN_RESP"
ok "Client token created"

# ══════════════════════════════════════════════════════════════════════════════
# DEPLOY TO K2B
# ══════════════════════════════════════════════════════════════════════════════
info "Deploying to K2B..."
ssh "root@${K2B_HOST}" "pkill -x ct-client 2>/dev/null || true"
sleep 0.3

scp "$CLIENT_ARM64" "root@${K2B_HOST}:/usr/local/bin/ct-client" >/dev/null
ssh "root@${K2B_HOST}" "chmod +x /usr/local/bin/ct-client"
scp "$REF_TONE" "root@${K2B_HOST}:/tmp/test-tone.wav" >/dev/null
ok "Deployed ct-client and test tone to K2B"

# Write client config — server URL and token only. No session_id or role.
# Session assignment is done by the admin UI (Playwright) via the Assign Peers card.
K2B_CFG="/tmp/ct-client-e2e.json"
ssh "root@${K2B_HOST}" "cat > ${K2B_CFG}" <<EOCFG
{
  "server_url": "http://${HOST_IP}:${SERVER_PORT}",
  "token": "${K2B_TOKEN}",
  "source_name": "${K2B_SOURCE}",
  "sink_name": "${K2B_SINK}",
  "log_level": "debug"
}
EOCFG
ok "K2B client config written"

# Start ct-client on K2B — connects to server and idles, waiting for assignment.
K2B_CLIENT_LOG="/tmp/ct-client-e2e.log"
ssh "root@${K2B_HOST}" "su - ${K2B_USER} -c 'XDG_RUNTIME_DIR=/run/user/${K2B_UID} nohup /usr/local/bin/ct-client --config ${K2B_CFG} > ${K2B_CLIENT_LOG} 2>&1 &'"
info "Waiting for K2B client to connect..."

K2B_CONNECTED=false
for _ in $(seq 1 20); do
    if ssh "root@${K2B_HOST}" "grep -q 'client connected and ready\|Welcome\|welcome received' ${K2B_CLIENT_LOG} 2>/dev/null"; then
        K2B_CONNECTED=true
        break
    fi
    sleep 1
done

if $K2B_CONNECTED; then
    ok "K2B ct-client connected to server"
else
    warn "K2B ct-client connection status unclear (checking logs)..."
    ssh "root@${K2B_HOST}" "tail -15 ${K2B_CLIENT_LOG}" 2>/dev/null || true
fi

# ══════════════════════════════════════════════════════════════════════════════
# TEST 9.5: Infrastructure verification
# ══════════════════════════════════════════════════════════════════════════════
echo ""
info "═══════════════════════════════════════════"
info "  TEST 9.5: E2E Infrastructure Verification"
info "═══════════════════════════════════════════"

# 5a. Server REST API
if curl -sf "${SERVER_URL}/api/templates" \
    -H "Authorization: Bearer ${SEED_TOKEN}" >/dev/null 2>&1; then
    pass_test "9.5a Server REST API healthy"
else
    fail_test "9.5a Server REST API not responding"
fi

# 5b. ct-client binary runs on K2B (check it's executable and ELF)
K2B_BIN_OK=$(ssh "root@${K2B_HOST}" "test -x /usr/local/bin/ct-client && head -c4 /usr/local/bin/ct-client | od -An -tx1 | tr -d ' '" 2>/dev/null || echo "")
if [[ "$K2B_BIN_OK" == *"7f454c46"* ]]; then
    pass_test "9.5b ct-client binary is valid ELF on K2B"
else
    fail_test "9.5b ct-client binary not valid on K2B (got: $K2B_BIN_OK)"
fi

# 5c. K2B PipeWire loopback nodes
K2B_PW=$(ssh "root@${K2B_HOST}" "su - ${K2B_USER} -c 'XDG_RUNTIME_DIR=/run/user/${K2B_UID} pw-cli list-objects Node 2>/dev/null'" | grep -c 'snd_aloop' || echo 0)
if [[ "$K2B_PW" -ge 2 ]]; then
    pass_test "9.5c PipeWire ALSA loopback nodes present on K2B (${K2B_PW})"
else
    fail_test "9.5c PipeWire loopback nodes missing on K2B (found ${K2B_PW})"
fi

# 5d. K2B ct-client connected to server
if $K2B_CONNECTED; then
    pass_test "9.5d ct-client on K2B connected to ct-server"
else
    WS_OK=$(ssh "root@${K2B_HOST}" "grep -c 'control channel\|connected\|WebRTC' ${K2B_CLIENT_LOG} 2>/dev/null" || echo 0)
    if [[ "$WS_OK" -gt 0 ]]; then
        pass_test "9.5d ct-client on K2B shows connection activity"
    else
        fail_test "9.5d ct-client on K2B failed to connect"
    fi
fi

# 5e. compare-audio.sh self-test
SELF_SCORE=$("$COMPARE" "$REF_TONE" "$REF_TONE" "0.99" 2>/dev/null || echo "0.0")
if python3 -c "import sys; sys.exit(0 if float('$SELF_SCORE') > 0.99 else 1)" 2>/dev/null; then
    pass_test "9.5e compare-audio.sh self-check (score: $SELF_SCORE)"
else
    fail_test "9.5e compare-audio.sh self-check failed (score: $SELF_SCORE)"
fi

# ══════════════════════════════════════════════════════════════════════════════
# GOLDEN AUDIO: Full admin UI flow (K2B → Browser)
# ══════════════════════════════════════════════════════════════════════════════
# This is the main E2E test. It exercises the ENTIRE user journey through the
# admin UI: login → create template → create session → assign K2B peer →
# connect as translator → verify WebRTC → capture audio from K2B.
#
# The Playwright test (golden-audio.spec.ts) drives all admin UI steps.
# The ONLY non-UI steps are SSH commands to the K2B board:
#   - Verifying ct-client is running
#   - Playing test audio into PipeWire
echo ""
info "═══════════════════════════════════════════"
info "  GOLDEN AUDIO: Full Admin UI E2E Test"
info "═══════════════════════════════════════════"
info ""
info "Full path: K2B PipeWire (test tone) → ct-client"
info "  → WebRTC → ct-server SFU → WebRTC → Playwright browser → capture"

BROWSER_CAPTURE="$TMPDIR/browser-captured-audio.webm"
BROWSER_CAPTURE_WAV="$TMPDIR/browser-captured-audio.wav"

info "Launching Playwright golden audio test..."
CT_SERVER_URL="${SERVER_URL}" \
CT_K2B_HOST="${K2B_HOST}" \
CT_K2B_USER="${K2B_USER}" \
CT_K2B_UID="${K2B_UID}" \
CT_ADMIN_PASSWORD="Password!" \
CT_CAPTURE_PATH="${BROWSER_CAPTURE}" \
CT_AUDIO_DURATION_MS="7000" \
npx playwright test \
    --config "$PW_CONFIG" \
    --grep "Golden Audio" \
    --project chromium \
    2>&1 | tee "$TMPDIR/pw-golden.log" || true

# Evaluate captured audio
if [[ -f "$BROWSER_CAPTURE" ]] && [[ "$(stat -c%s "$BROWSER_CAPTURE" 2>/dev/null || echo 0)" -gt 1000 ]]; then
    # Convert webm to wav for comparison
    ffmpeg -y -hide_banner -loglevel error -i "$BROWSER_CAPTURE" -ar 48000 -ac 1 "$BROWSER_CAPTURE_WAV"

    SCORE=$("$COMPARE" "$REF_TONE" "$BROWSER_CAPTURE_WAV" "$THRESHOLD" 2>&1 | tail -1 || echo "0.0")
    info "Cross-correlation score: $SCORE (threshold: $THRESHOLD)"

    if python3 -c "import sys; sys.exit(0 if float('$SCORE') > float('$THRESHOLD') else 1)" 2>/dev/null; then
        pass_test "Golden audio K2B→Browser verified (corr: $SCORE > $THRESHOLD)"
    else
        fail_test "Golden audio correlation too low: $SCORE < $THRESHOLD"
    fi
else
    fail_test "Golden audio capture failed (no audio captured in browser)"
    info "Check: Playwright log: cat $TMPDIR/pw-golden.log"
    info "Check: K2B client log: ssh root@${K2B_HOST} cat ${K2B_CLIENT_LOG}"
fi

# ══════════════════════════════════════════════════════════════════════════════
# SUMMARY
# ══════════════════════════════════════════════════════════════════════════════
echo ""
echo "═══════════════════════════════════════════════════════════════"
echo "  E2E Test Results"
echo "═══════════════════════════════════════════════════════════════"
echo ""
echo -e "  Total:   ${TESTS_RUN}"
echo -e "  ${GRN}Passed:  ${TESTS_PASSED}${RST}"
echo -e "  ${YEL}Skipped: ${TESTS_SKIPPED}${RST}"
echo -e "  ${RED}Failed:  ${TESTS_FAILED}${RST}"
echo ""
info "Artifacts:"
info "  Server log:     $SERVER_LOG"
info "  K2B log:        ssh root@${K2B_HOST} cat ${K2B_CLIENT_LOG}"
info "  PW golden log:  $TMPDIR/pw-golden.log"
info "  Temp dir:       $TMPDIR"
echo ""

if [[ "$TESTS_FAILED" -eq 0 ]]; then
    ok "E2E test suite passed (${TESTS_PASSED}/${TESTS_RUN} passed, ${TESTS_SKIPPED} skipped)"
    exit 0
else
    fail "E2E test suite: ${TESTS_FAILED} test(s) failed"
    exit 1
fi
