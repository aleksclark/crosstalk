#!/usr/bin/env bash
# test-compare-audio.sh — Standalone validation of compare-audio.sh.
#
# Runs three acceptance tests:
#   1. Self-correlation: compare a file with itself → score ~1.0
#   2. Silence correlation: compare tone with silence → score ~0.0
#   3. Compressed-version correlation: encode to opus and back → score > 0.9
#
# Exits 0 if all pass, 1 otherwise.
#
# Dependencies: ffmpeg, python3
set -euo pipefail

RED='\033[0;31m'; GRN='\033[0;32m'; CYN='\033[0;36m'; RST='\033[0m'
info()  { echo -e "${CYN}[INFO]${RST}  $*"; }
ok()    { echo -e "${GRN}[PASS]${RST}  $*"; }
fail()  { echo -e "${RED}[FAIL]${RST}  $*"; }
die()   { fail "$*"; exit 2; }

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
COMPARE="$SCRIPT_DIR/compare-audio.sh"
[[ -x "$COMPARE" ]] || die "compare-audio.sh not found or not executable: $COMPARE"

command -v ffmpeg  >/dev/null 2>&1 || die "ffmpeg not found"
command -v python3 >/dev/null 2>&1 || die "python3 not found"

TMPDIR="$(mktemp -d /tmp/ct-compare-test-XXXXXX)"
trap 'rm -rf "$TMPDIR"' EXIT

TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

pass() { ((TESTS_PASSED++)) || true; ((TESTS_RUN++)) || true; ok "$1"; }
fail_test() { ((TESTS_FAILED++)) || true; ((TESTS_RUN++)) || true; fail "$1"; }

# ── Generate test tone (5s, 1kHz, 48kHz mono) ────────────────────────────
TONE="$TMPDIR/tone.wav"
info "Generating 5s 1kHz test tone..."
ffmpeg -y -hide_banner -loglevel error \
  -f lavfi -i "sine=frequency=1000:duration=5" \
  -ar 48000 -ac 1 "$TONE"

# ── Test 1: Self-correlation (~1.0) ──────────────────────────────────────
info "Test 1: Self-correlation (expect ~1.0)..."
SCORE_SELF=$("$COMPARE" "$TONE" "$TONE" "0.99" 2>/dev/null || true)
SCORE_SELF=$(echo "$SCORE_SELF" | tail -1)
info "  Score: $SCORE_SELF"

if python3 -c "import sys; sys.exit(0 if float('${SCORE_SELF}') > 0.99 else 1)" 2>/dev/null; then
  pass "Self-correlation: $SCORE_SELF > 0.99"
else
  fail_test "Self-correlation: $SCORE_SELF (expected > 0.99)"
fi

# ── Test 2: Silence correlation (~0.0) ───────────────────────────────────
SILENCE="$TMPDIR/silence.wav"
info "Test 2: Silence correlation (expect ~0.0)..."
ffmpeg -y -hide_banner -loglevel error \
  -f lavfi -i "anullsrc=r=48000:cl=mono" \
  -t 5 "$SILENCE"

# compare-audio.sh exits non-zero when below threshold; capture output regardless
SCORE_SILENCE=$("$COMPARE" "$TONE" "$SILENCE" "0.01" 2>/dev/null || true)
SCORE_SILENCE=$(echo "$SCORE_SILENCE" | tail -1)
info "  Score: $SCORE_SILENCE"

if python3 -c "import sys; sys.exit(0 if float('${SCORE_SILENCE}') < 0.10 else 1)" 2>/dev/null; then
  pass "Silence correlation: $SCORE_SILENCE < 0.10"
else
  fail_test "Silence correlation: $SCORE_SILENCE (expected < 0.10)"
fi

# ── Test 3: Compressed-version correlation (opus round-trip, > 0.9) ──────
OPUS="$TMPDIR/tone.opus"
DECODED="$TMPDIR/tone-decoded.wav"
info "Test 3: Opus round-trip correlation (expect > 0.9)..."
ffmpeg -y -hide_banner -loglevel error \
  -i "$TONE" -c:a libopus -b:a 64k "$OPUS"
ffmpeg -y -hide_banner -loglevel error \
  -i "$OPUS" -ar 48000 -ac 1 "$DECODED"

SCORE_OPUS=$("$COMPARE" "$TONE" "$DECODED" "0.90" 2>/dev/null || true)
SCORE_OPUS=$(echo "$SCORE_OPUS" | tail -1)
info "  Score: $SCORE_OPUS"

if python3 -c "import sys; sys.exit(0 if float('${SCORE_OPUS}') > 0.90 else 1)" 2>/dev/null; then
  pass "Opus round-trip: $SCORE_OPUS > 0.90"
else
  fail_test "Opus round-trip: $SCORE_OPUS (expected > 0.90)"
fi

# ── Summary ──────────────────────────────────────────────────────────────
echo ""
echo "══════════════════════════════════════════"
echo "  compare-audio.sh validation: ${TESTS_PASSED}/${TESTS_RUN} passed"
echo "══════════════════════════════════════════"

if [[ "$TESTS_FAILED" -eq 0 ]]; then
  ok "All compare-audio tests passed"
  exit 0
else
  fail "${TESTS_FAILED} test(s) failed"
  exit 1
fi
