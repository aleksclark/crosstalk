#!/usr/bin/env bash
# run-device-golden.sh — Physical (preferred) / emulator Android golden acceptance.
#
# Flow:
#   preflight → install → seed via public APIs → login/join → tones →
#   KEYCODE_HOME → KEYCODE_SLEEP → poll evidence → WAKEUP → network toggle →
#   Stop → redact archive
#
# Process death uses `am kill` (not force-stop). force-stop is terminal admin only.
#
# Env (never echoed):
#   CROSSTALK_BASE_URL              required for real server path
#   CROSSTALK_ADMIN_PASSWORD        required to seed (or provide seed.env)
#   CROSSTALK_ADMIN_USER            default admin
#   CROSSTALK_SERIAL                adb -s target (optional)
#   CROSSTALK_PACKAGE               default com.crosstalk.translator.debug
#   CROSSTALK_SLEEP_SECONDS         default 600 (10 minutes)
#   CROSSTALK_EVIDENCE_DIR          default /tmp/ct-android-golden-<pid>
#   CROSSTALK_SKIP_SEED=1           reuse existing seed.env
#   CROSSTALK_ALLOW_EMULATOR=1      allow non-physical device (synthetic capture)
#   JAVA_HOME / ANDROID_HOME        build toolchain
#
# Physical vs emulator:
#   - Physical API 15 preferred: real mic 880 Hz + external capture of 440 Hz feed.
#   - Emulator path is DEBUG-ONLY / synthetic-capture and must never be labelled
#     as physical-mic proof. Release path has no synthetic audio injection.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ANDROID_DIR="$ROOT/apps/android-translator"
LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/lib" && pwd)"
export JAVA_HOME="${JAVA_HOME:-/usr/lib/jvm/java-17-openjdk}"
export ANDROID_HOME="${ANDROID_HOME:-/opt/android-sdk}"
export ANDROID_SDK_ROOT="${ANDROID_SDK_ROOT:-$ANDROID_HOME}"
export PATH="$ANDROID_HOME/platform-tools:$ANDROID_HOME/emulator:$PATH"

PKG="${CROSSTALK_PACKAGE:-com.crosstalk.translator.debug}"
SLEEP_SECS="${CROSSTALK_SLEEP_SECONDS:-600}"
EVIDENCE="${CROSSTALK_EVIDENCE_DIR:-/tmp/ct-android-golden-$$}"
ADB=(adb)
if [[ -n "${CROSSTALK_SERIAL:-}" ]]; then
  ADB=(adb -s "$CROSSTALK_SERIAL")
fi

mkdir -p "$EVIDENCE"
chmod 700 "$EVIDENCE"
SUMMARY="$EVIDENCE/summary.json"
log() { printf '%s\n' "$*" | tee -a "$EVIDENCE/harness.log" >/dev/null; printf '%s\n' "$*"; }

cleanup() {
  local code=$?
  set +e
  # Best-effort stop service; never dump secrets.
  "${ADB[@]}" shell am startservice -a com.crosstalk.translator.action.STOP -n "$PKG/com.crosstalk.translator.service.TranslatorAudioService" >/dev/null 2>&1
  if [[ -d "$EVIDENCE" ]]; then
    bash "$LIB_DIR/redact-evidence.sh" "$EVIDENCE" || true
  fi
  exit "$code"
}
trap cleanup EXIT

# ── preflight ─────────────────────────────────────────────────────────────
log "==> Preflight"
"${ADB[@]}" start-server >/dev/null
mapfile -t DEVS < <("${ADB[@]}" devices | awk 'NR>1 && $2=="device"{print $1}')
if [[ ${#DEVS[@]} -eq 0 ]]; then
  echo "FAIL: no adb device/emulator online" >&2
  exit 1
fi
SERIAL="${CROSSTALK_SERIAL:-${DEVS[0]}}"
ADB=(adb -s "$SERIAL")
log "device_serial=$SERIAL"

PROP_MODEL="$("${ADB[@]}" shell getprop ro.product.model | tr -d '\r')"
PROP_API="$("${ADB[@]}" shell getprop ro.build.version.sdk | tr -d '\r')"
PROP_FINGER="$("${ADB[@]}" shell getprop ro.build.fingerprint | tr -d '\r')"
IS_EMU=0
if [[ "$PROP_FINGER" == *generic* || "$PROP_FINGER" == *sdk_gphone* || "$PROP_MODEL" == *sdk* || "$PROP_MODEL" == *Emulator* ]]; then
  IS_EMU=1
fi
if [[ "$IS_EMU" -eq 1 && "${CROSSTALK_ALLOW_EMULATOR:-0}" != "1" ]]; then
  echo "FAIL: connected target looks like an emulator ($PROP_MODEL). Physical device required for golden bidirectional proof." >&2
  echo "      Set CROSSTALK_ALLOW_EMULATOR=1 only for debug synthetic-capture (never release proof)." >&2
  exit 1
fi
if [[ "$IS_EMU" -eq 1 ]]; then
  log "WARNING: EMULATOR PATH — synthetic-capture / debug-only. Not physical-mic proof."
  CAPTURE_LABEL="synthetic-capture-debug-only"
else
  CAPTURE_LABEL="physical-mic"
fi

BATTERY="$("${ADB[@]}" shell dumpsys battery 2>/dev/null | tr -d '\r' | awk -F': ' '/level:/{print $2; exit}')"
AUDIO_ROUTE="$("${ADB[@]}" shell dumpsys audio 2>/dev/null | tr -d '\r' | head -c 400 | tr '\n' ' ')"
log "api=$PROP_API model=$PROP_MODEL battery=${BATTERY:-unknown} capture=$CAPTURE_LABEL"

if [[ -z "${CROSSTALK_BASE_URL:-}" ]]; then
  echo "FAIL: CROSSTALK_BASE_URL required" >&2
  exit 1
fi

# ── install ───────────────────────────────────────────────────────────────
log "==> Building and installing debug APK"
if [[ ! -f "$ANDROID_DIR/local.properties" ]]; then
  printf 'sdk.dir=%s\n' "$ANDROID_HOME" >"$ANDROID_DIR/local.properties"
fi
(
  cd "$ANDROID_DIR"
  ./gradlew --no-daemon :app:assembleDebug :app:installDebug
) | tee -a "$EVIDENCE/build.log" | tail -n 20

APK_PATH="$(ls -1 "$ANDROID_DIR/app/build/outputs/apk/debug/"*.apk 2>/dev/null | head -1 || true)"
log "apk=${APK_PATH:-unknown}"

# Grant runtime perms (debug harness).
"${ADB[@]}" shell pm grant "$PKG" android.permission.RECORD_AUDIO >/dev/null 2>&1 || true
"${ADB[@]}" shell pm grant "$PKG" android.permission.POST_NOTIFICATIONS >/dev/null 2>&1 || true

# ── seed ──────────────────────────────────────────────────────────────────
SEED_ENV="$EVIDENCE/seed.env"
SEED_SUMMARY="$EVIDENCE/seed-summary.json"
if [[ "${CROSSTALK_SKIP_SEED:-0}" == "1" && -f "${SEED_ENV_IN:-}" ]]; then
  cp "${SEED_ENV_IN}" "$SEED_ENV"
else
  if [[ -z "${CROSSTALK_ADMIN_PASSWORD:-}" ]]; then
    echo "FAIL: CROSSTALK_ADMIN_PASSWORD required to seed (or CROSSTALK_SKIP_SEED=1 + SEED_ENV_IN)" >&2
    exit 1
  fi
  SEED_OUT="$SEED_SUMMARY" SEED_ENV_OUT="$SEED_ENV" \
    bash "$LIB_DIR/seed-session.sh"
fi
# shellcheck disable=SC1090
source "$SEED_ENV"
# Immediately remove password-bearing file from world-readable umasks; keep 600.
chmod 600 "$SEED_ENV"

# ── login / join UI ───────────────────────────────────────────────────────
log "==> Launch app, login, join"
# Override API base for this install via instrumentation is not enough for UI;
# debug BuildConfig defaults to 10.0.2.2:8080. For physical devices the operator
# must rebuild with -PCROSSTALK_API_BASE_URL=... or use reverse tcp.
if [[ "$IS_EMU" -eq 1 ]]; then
  # Emulator → host loopback
  :
else
  # Physical: reverse host port if base is localhost-like
  case "${CROSSTALK_BASE_URL}" in
    http://127.0.0.1:*|http://localhost:*|http://10.0.2.2:*)
      PORT="$(python3 - <<PY
from urllib.parse import urlparse
print(urlparse("${CROSSTALK_BASE_URL}").port or 8080)
PY
)"
      "${ADB[@]}" reverse "tcp:${PORT}" "tcp:${PORT}" || true
      log "adb reverse tcp:$PORT"
      ;;
  esac
fi

"${ADB[@]}" shell am force-stop "$PKG" >/dev/null 2>&1 || true
"${ADB[@]}" shell monkey -p "$PKG" -c android.intent.category.LAUNCHER 1 >/dev/null 2>&1
sleep 2

# UI login via input — best-effort; operators may pre-login.
# Prefer Compose test tags when UI Automator can see them.
run_ui_login() {
  # Focus is fragile; document manual login fallback.
  log "UI automation: attempting login fields (best-effort)"
  "${ADB[@]}" shell input keyevent KEYCODE_TAB || true
  # Do not echo password. Use app private broadcast if available in future.
  :
}

# Direct service JOIN after translator has been authenticated is not possible
# without vault credentials. Drive instrumented tests that know the password via
# runner args without printing them.
log "==> Running instrumented real-server suite (assignment + optional Pion)"
(
  cd "$ANDROID_DIR"
  ./gradlew --no-daemon :app:connectedDebugAndroidTest \
    -Pandroid.testInstrumentationRunnerArguments.CROSSTALK_BASE_URL="${CROSSTALK_BASE_URL}" \
    -Pandroid.testInstrumentationRunnerArguments.CROSSTALK_TRANSLATOR_USER="${CROSSTALK_TRANSLATOR_USER}" \
    -Pandroid.testInstrumentationRunnerArguments.CROSSTALK_TRANSLATOR_PASSWORD="${CROSSTALK_TRANSLATOR_PASSWORD}" \
    -Pandroid.testInstrumentationRunnerArguments.CROSSTALK_SESSION_ID="${CROSSTALK_SESSION_ID}" \
    -Pandroid.testInstrumentationRunnerArguments.CROSSTALK_SESSION_NAME="${CROSSTALK_SESSION_NAME}" \
    || true
) | tee -a "$EVIDENCE/instrumented.log" | tail -n 40

# Start FGS join intent — mint will fail until auth vault has refresh; still
# exercises notification/service path. Full authenticated join is via UI.
"${ADB[@]}" shell am start-foreground-service \
  -n "$PKG/com.crosstalk.translator.service.TranslatorAudioService" \
  -a com.crosstalk.translator.action.JOIN \
  --es session_id "${CROSSTALK_SESSION_ID}" \
  --es session_name "${CROSSTALK_SESSION_NAME}" \
  --es feed_name "Floor Feed" \
  --es broadcast_name "English Broadcast" \
  >/dev/null 2>&1 || true
sleep 2

# ── tones / pre-sleep evidence ────────────────────────────────────────────
log "==> Pre-sleep evidence snapshot (capture=$CAPTURE_LABEL)"
{
  echo "timestamp=$(date -Is)"
  echo "capture_label=$CAPTURE_LABEL"
  echo "session_id=${CROSSTALK_SESSION_ID}"
  echo "--- dumpsys activity services (package) ---"
  "${ADB[@]}" shell dumpsys activity services "$PKG" 2>/dev/null | tr -d '\r' | head -n 80
  echo "--- dumpsys notification ---"
  "${ADB[@]}" shell dumpsys notification --noredact 2>/dev/null | tr -d '\r' | grep -F "$PKG" | head -n 40 || true
} >"$EVIDENCE/pre-sleep.txt"

# Physical tone injectors are operator-owned (external speaker 880 Hz into mic,
# floor 440 Hz via ct-play/ABC). Emulator may use synthetic debug capture only.
if [[ "$IS_EMU" -eq 1 ]]; then
  log "emulator: no physical tone inject; label=$CAPTURE_LABEL"
else
  log "physical: ensure 440 Hz floor + 880 Hz mic stimulus are running externally"
fi

# Optional ct-play floor if binary present and ABC not required for this pass.
if command -v ffmpeg >/dev/null 2>&1; then
  log "ffmpeg available for optional local tone generation (not auto-started into phone mic)"
fi

# ── Home + Sleep ──────────────────────────────────────────────────────────
log "==> KEYCODE_HOME then KEYCODE_SLEEP; poll for ${SLEEP_SECS}s"
"${ADB[@]}" shell input keyevent KEYCODE_HOME
sleep 1
"${ADB[@]}" shell input keyevent KEYCODE_SLEEP

START_TS=$(date +%s)
POLL_LOG="$EVIDENCE/sleep-poll.ndjson"
: >"$POLL_LOG"
while true; do
  NOW=$(date +%s)
  ELAPSED=$((NOW - START_TS))
  RUNNING="$("${ADB[@]}" shell dumpsys activity services "$PKG" 2>/dev/null | tr -d '\r' | grep -c 'TranslatorAudioService' || true)"
  printf '{"t":%s,"elapsed":%s,"service_hits":%s,"capture":"%s"}\n' \
    "$NOW" "$ELAPSED" "$RUNNING" "$CAPTURE_LABEL" >>"$POLL_LOG"
  if [[ "$ELAPSED" -ge "$SLEEP_SECS" ]]; then
    break
  fi
  sleep 30
done

# ── Wake ──────────────────────────────────────────────────────────────────
log "==> KEYCODE_WAKEUP + unlock attempt"
"${ADB[@]}" shell input keyevent KEYCODE_WAKEUP
sleep 0.5
"${ADB[@]}" shell input keyevent KEYCODE_MENU || true
"${ADB[@]}" shell wm dismiss-keyguard 2>/dev/null || true
"${ADB[@]}" shell input keyevent 82 || true
sleep 1
{
  echo "timestamp=$(date -Is)"
  "${ADB[@]}" shell dumpsys activity services "$PKG" 2>/dev/null | tr -d '\r' | head -n 80
} >"$EVIDENCE/post-wake.txt"

# ── network toggle ────────────────────────────────────────────────────────
log "==> Network toggle (airplane mode pulse)"
"${ADB[@]}" shell cmd connectivity airplane-mode enable 2>/dev/null \
  || "${ADB[@]}" shell settings put global airplane_mode_on 1
sleep 5
"${ADB[@]}" shell cmd connectivity airplane-mode disable 2>/dev/null \
  || "${ADB[@]}" shell settings put global airplane_mode_on 0
sleep 8
{
  echo "timestamp=$(date -Is)"
  "${ADB[@]}" shell dumpsys activity services "$PKG" 2>/dev/null | tr -d '\r' | head -n 80
} >"$EVIDENCE/post-network.txt"

# ── Stop ──────────────────────────────────────────────────────────────────
log "==> Explicit Stop (notification action intent)"
"${ADB[@]}" shell am startservice \
  -n "$PKG/com.crosstalk.translator.service.TranslatorAudioService" \
  -a com.crosstalk.translator.action.STOP >/dev/null 2>&1 || true
sleep 2
{
  echo "timestamp=$(date -Is)"
  "${ADB[@]}" shell dumpsys activity services "$PKG" 2>/dev/null | tr -d '\r' | head -n 40
} >"$EVIDENCE/post-stop.txt"

# Optional process-death documentation (am kill, not force-stop)
log "==> Document am kill process-death (no force-stop)"
"${ADB[@]}" shell am start-foreground-service \
  -n "$PKG/com.crosstalk.translator.service.TranslatorAudioService" \
  -a com.crosstalk.translator.action.JOIN \
  --es session_id "${CROSSTALK_SESSION_ID}" \
  --es session_name "${CROSSTALK_SESSION_NAME}" \
  --es feed_name "Floor Feed" \
  --es broadcast_name "English Broadcast" >/dev/null 2>&1 || true
sleep 1
"${ADB[@]}" shell am kill "$PKG"
sleep 2
{
  echo "am_kill=1"
  echo "note=force-stop is terminal admin and is NOT used for continuity expectations"
  "${ADB[@]}" shell dumpsys activity services "$PKG" 2>/dev/null | tr -d '\r' | head -n 40
} >"$EVIDENCE/post-am-kill.txt"

# ── archive ───────────────────────────────────────────────────────────────
python3 - <<PY >"$SUMMARY"
import json, os, time
print(json.dumps({
  "device_serial": "$SERIAL",
  "model": "$PROP_MODEL",
  "api": "$PROP_API",
  "capture_label": "$CAPTURE_LABEL",
  "emulator": bool($IS_EMU),
  "sleep_seconds": int("$SLEEP_SECS"),
  "session_id": os.environ.get("CROSSTALK_SESSION_ID", ""),
  "session_name": os.environ.get("CROSSTALK_SESSION_NAME", ""),
  "package": "$PKG",
  "apk": r"""${APK_PATH:-}""",
  "evidence_dir": r"""$EVIDENCE""",
  "finished_at": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
  "notes": [
    "Physical bidirectional spectral proof requires external 440/880 injectors + listener decode.",
    "Emulator path is synthetic-capture debug-only and must not be relabelled physical.",
    "Process death uses am kill; force-stop is documented separately as terminal.",
  ],
}, indent=2))
PY

# Strip seed.env from archiveable tree (secrets). Keep seed-summary only.
rm -f "$SEED_ENV"
bash "$LIB_DIR/redact-evidence.sh" "$EVIDENCE"

ARCHIVE="$EVIDENCE/android-golden-evidence.tgz"
tar -C "$(dirname "$EVIDENCE")" -czf "$ARCHIVE" "$(basename "$EVIDENCE")"
log "==> Evidence archive: $ARCHIVE"
log "DONE capture=$CAPTURE_LABEL sleep=${SLEEP_SECS}s"
