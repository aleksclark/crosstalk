#!/usr/bin/env bash
# Deploy ct-abc (Audio Booth Connector) to K2B and configure it to connect to a server.
# Historical name "ct-client" is accepted as input binary; on-device path is ct-abc.
#
# Usage:
#   CT_ABC_TOKEN=ct_... ./deploy.sh <board-ip> <binary-path> <server-url>
#   CT_ABC_TOKEN_FILE=/path/to/token ./deploy.sh <board-ip> <binary-path> <server-url>
#   ./deploy.sh --rollback <board-ip> [stamp]
#
# Environment variables:
#   CT_ABC_TOKEN / CT_ABC_TOKEN_FILE — API token (required for deploy; never pass on argv)
#   K2B_SOURCE       — PipeWire source name (default: physical audiocodec mic)
#   K2B_SINK         — PipeWire sink name (default: physical audiocodec output)
#   K2B_LOG_LEVEL    — log level (default: debug)
#   K2B_USE_DISPLAY  — enable SPI display (default: true)
#   K2B_AUDIO_MODE   — "physical" or "loopback" (default: physical)
#
# Example:
#   CT_ABC_TOKEN=ct_abc123... ./deploy.sh 192.168.0.109 ../../bin/ct-abc-arm64 http://192.168.0.22:8080
#   ./deploy.sh --rollback 192.168.0.109
#
# What this does (deploy):
#   1. Backs up live binary + unit + helper + config under /root/crosstalk-rollback/$STAMP
#   2. Stops the running service
#   3. Deploys the binary as /usr/local/bin/ct-abc (atomic temp+checksum+rename)
#   4. Installs ct-app-setup.sh helper + app.service unit
#   5. Writes /etc/app/crosstalk.json with server URL + token + audio devices
#   6. Restarts the service (restores stamp on failure)
#
# Rollback:
#   Restores binary + unit + helper + config from the newest (or named) stamp and restarts.
set -euo pipefail

MODE="deploy"
if [[ "${1:-}" == "--rollback" ]]; then
  MODE="rollback"
  shift
fi

BOARD_IP="${1:?Usage: $0 [--rollback] <board-ip> [binary-path server-url | stamp]}"
ARG2="${2:-}"
ARG3="${3:-}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SERVICE_FILE="${SCRIPT_DIR}/deploy/app.service"
SETUP_HELPER="${SCRIPT_DIR}/deploy/ct-app-setup.sh"
REMOTE_BIN="/usr/local/bin/ct-abc"
REMOTE_UNIT="/etc/systemd/system/app.service"
REMOTE_CFG="/etc/app/crosstalk.json"
REMOTE_HELPER="/usr/local/lib/crosstalk/ct-app-setup.sh"
ROLLBACK_ROOT="/root/crosstalk-rollback"

SSH=(ssh -o ConnectTimeout=8 -o BatchMode=yes "root@${BOARD_IP}")
SCP=(scp -o ConnectTimeout=8)

remote() {
  "${SSH[@]}" "$@"
}

sha256_local() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

resolve_token() {
  if [[ -n "${CT_ABC_TOKEN:-}" ]]; then
    printf '%s' "${CT_ABC_TOKEN}"
    return 0
  fi
  if [[ -n "${CT_ABC_TOKEN_FILE:-}" ]]; then
    if [[ ! -f "${CT_ABC_TOKEN_FILE}" ]]; then
      echo "CT_ABC_TOKEN_FILE not found: ${CT_ABC_TOKEN_FILE}" >&2
      exit 1
    fi
    # Trim trailing newlines only; keep internal content intact.
    tr -d '\r' < "${CT_ABC_TOKEN_FILE}" | sed -e 's/[[:space:]]*$//'
    return 0
  fi
  echo "API token required via CT_ABC_TOKEN or CT_ABC_TOKEN_FILE (do not pass on argv)" >&2
  exit 1
}

backup_live() {
  local stamp="$1"
  remote "set -euo pipefail
    STAMP='${stamp}'
    DEST='${ROLLBACK_ROOT}/'\"\$STAMP\"
    mkdir -p \"\$DEST\"
    if [ -f '${REMOTE_BIN}' ]; then
      cp -a '${REMOTE_BIN}' \"\$DEST/ct-abc\"
      sha256sum '${REMOTE_BIN}' > \"\$DEST/ct-abc.sha256\"
    fi
    if [ -f '${REMOTE_UNIT}' ]; then
      cp -a '${REMOTE_UNIT}' \"\$DEST/app.service\"
      sha256sum '${REMOTE_UNIT}' > \"\$DEST/app.service.sha256\"
    fi
    if [ -f '${REMOTE_CFG}' ]; then
      cp -a '${REMOTE_CFG}' \"\$DEST/crosstalk.json\"
      sha256sum '${REMOTE_CFG}' > \"\$DEST/crosstalk.json.sha256\"
    fi
    if [ -f '${REMOTE_HELPER}' ]; then
      cp -a '${REMOTE_HELPER}' \"\$DEST/ct-app-setup.sh\"
      sha256sum '${REMOTE_HELPER}' > \"\$DEST/ct-app-setup.sh.sha256\"
    fi
    {
      echo \"stamp=\$STAMP\"
      echo \"when=\$(date -u +%Y-%m-%dT%H:%M:%SZ)\"
      echo \"hostname=\$(hostname)\"
      systemctl is-active app.service 2>/dev/null || true
      systemctl is-enabled app.service 2>/dev/null || true
    } > \"\$DEST/pre-state.txt\"
    ls -la \"\$DEST\"
    echo \"BACKUP_OK \$DEST\"
  "
}

restore_from() {
  local stamp="$1"
  remote "set -euo pipefail
    SRC='${ROLLBACK_ROOT}/${stamp}'
    if [ ! -d \"\$SRC\" ]; then
      echo \"rollback stamp not found: \$SRC\" >&2
      exit 2
    fi
    systemctl stop app.service 2>/dev/null || true
    if [ -f \"\$SRC/ct-abc\" ]; then
      install -m 0755 \"\$SRC/ct-abc\" '${REMOTE_BIN}.restoring'
      if [ -f \"\$SRC/ct-abc.sha256\" ]; then
        WANT=\$(awk '{print \$1}' \"\$SRC/ct-abc.sha256\")
        GOT=\$(sha256sum '${REMOTE_BIN}.restoring' | awk '{print \$1}')
        [ \"\$WANT\" = \"\$GOT\" ] || { echo \"binary checksum mismatch on restore\" >&2; exit 3; }
      fi
      mv -f '${REMOTE_BIN}.restoring' '${REMOTE_BIN}'
      chmod +x '${REMOTE_BIN}'
      ln -sfn ct-abc /usr/local/bin/ct-client
    fi
    if [ -f \"\$SRC/app.service\" ]; then
      install -m 0644 \"\$SRC/app.service\" '${REMOTE_UNIT}'
    fi
    if [ -f \"\$SRC/crosstalk.json\" ]; then
      mkdir -p /etc/app
      # Match deploy path: app:audio can read config; root-only 0600 breaks User=app.
      install -o app -g audio -m 0640 \"\$SRC/crosstalk.json\" '${REMOTE_CFG}'
    fi
    if [ -f \"\$SRC/ct-app-setup.sh\" ]; then
      mkdir -p /usr/local/lib/crosstalk
      install -m 0755 \"\$SRC/ct-app-setup.sh\" '${REMOTE_HELPER}'
    fi
    systemctl daemon-reload
    systemctl reset-failed app.service 2>/dev/null || true
    systemctl restart app.service
    sleep 1
    systemctl is-active app.service || true
    echo \"ROLLBACK_OK ${stamp}\"
  "
}

if [[ "$MODE" == "rollback" ]]; then
  STAMP="${ARG2:-}"
  if [[ -z "$STAMP" ]]; then
    STAMP="$(remote "ls -1d '${ROLLBACK_ROOT}'/[0-9]*T*Z 2>/dev/null | xargs -n1 basename 2>/dev/null | sort | tail -1")"
  fi
  if [[ -z "$STAMP" ]]; then
    echo "No rollback stamps found on ${BOARD_IP}:${ROLLBACK_ROOT}" >&2
    exit 2
  fi
  echo "=== Rolling back ct-abc on ${BOARD_IP} from stamp ${STAMP} ==="
  restore_from "$STAMP"
  remote "systemctl status app --no-pager -l" || true
  exit 0
fi

BINARY="${ARG2:?Usage: $0 <board-ip> <binary-path> <server-url>}"
SERVER_URL="${ARG3:?Usage: $0 <board-ip> <binary-path> <server-url>}"
if [[ ! -f "$BINARY" ]]; then
  echo "binary not found: $BINARY" >&2
  exit 1
fi

# Reject legacy 4th argv token to prevent secrets on process lists.
if [[ -n "${4:-}" ]]; then
  echo "ERROR: API token must not be passed as argv. Use CT_ABC_TOKEN or CT_ABC_TOKEN_FILE." >&2
  exit 1
fi

API_TOKEN="$(resolve_token)"
if [[ -z "$API_TOKEN" ]]; then
  echo "API token is empty" >&2
  exit 1
fi

AUDIO_MODE="${K2B_AUDIO_MODE:-physical}"
USE_DISPLAY="${K2B_USE_DISPLAY:-true}"
LOG_LEVEL="${K2B_LOG_LEVEL:-debug}"

# Audio device names depend on mode
if [ "$AUDIO_MODE" = "loopback" ]; then
    K2B_SOURCE="${K2B_SOURCE:-alsa_output.platform-snd_aloop.0.analog-stereo.monitor}"
    K2B_SINK="${K2B_SINK:-alsa_output.platform-snd_aloop.0.analog-stereo}"
else
    # Physical audio via USB adapter (C-Media Y-247A, ALSA card id "Device").
    # Reference the card by stable id (CARD=Device) rather than index, since
    # ALSA card numbers can change across reboots/hotplug. Use plughw: for
    # format conversion and full-duplex concurrent access.
    K2B_SOURCE="${K2B_SOURCE:-plughw:CARD=Device,DEV=0}"
    K2B_SINK="${K2B_SINK:-plughw:CARD=Device,DEV=0}"
fi

LOCAL_SHA="$(sha256_local "$BINARY")"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
STAGE="/usr/local/bin/.ct-abc.${STAMP}.new"

echo "=== Deploying ct-abc to K2B at ${BOARD_IP} ==="
echo "    binary:  ${BINARY}"
echo "    sha256:  ${LOCAL_SHA}"
echo "    stamp:   ${STAMP}"
echo "    server:  ${SERVER_URL}"
echo "    Audio:   ${AUDIO_MODE}"
echo "    Source:  ${K2B_SOURCE}"
echo "    Sink:    ${K2B_SINK}"
echo "    Display: ${USE_DISPLAY}"

echo "[1/7] Capturing pre-state backup under ${ROLLBACK_ROOT}/${STAMP}..."
backup_live "$STAMP"

echo "[2/7] Stopping service..."
remote "systemctl stop app.service 2>/dev/null || true"
sleep 1

echo "[3/7] Atomic binary deploy (temp + checksum + rename)..."
"${SCP[@]}" "$BINARY" "root@${BOARD_IP}:${STAGE}"
if ! remote "set -euo pipefail
  GOT=\$(sha256sum '${STAGE}' | awk '{print \$1}')
  WANT='${LOCAL_SHA}'
  if [ \"\$GOT\" != \"\$WANT\" ]; then
    echo \"checksum mismatch: got=\$GOT want=\$WANT\" >&2
    rm -f '${STAGE}'
    exit 4
  fi
  chmod 0755 '${STAGE}'
  mv -f '${STAGE}' '${REMOTE_BIN}'
  chmod +x '${REMOTE_BIN}'
  ln -sfn ct-abc /usr/local/bin/ct-client
  FINAL=\$(sha256sum '${REMOTE_BIN}' | awk '{print \$1}')
  [ \"\$FINAL\" = \"\$WANT\" ]
  echo \"BINARY_OK \$FINAL\"
"; then
  echo "Binary deploy failed — attempting restore from ${STAMP}" >&2
  restore_from "$STAMP" || true
  exit 5
fi

echo "[4/7] Installing ct-app-setup.sh..."
if ! remote "mkdir -p /usr/local/lib/crosstalk"; then
  echo "Setup dir failed — attempting restore from ${STAMP}" >&2
  restore_from "$STAMP" || true
  exit 6
fi
"${SCP[@]}" "$SETUP_HELPER" "root@${BOARD_IP}:/usr/local/lib/crosstalk/ct-app-setup.sh"
remote "chmod +x /usr/local/lib/crosstalk/ct-app-setup.sh"

echo "[5/7] Writing config (token via env/file, not argv)..."
# Stream JSON over SSH stdin so the token never appears in remote argv/ps.
if ! remote "set -euo pipefail
  mkdir -p /etc/app
  cat > '${REMOTE_CFG}.new'
  chown app:audio '${REMOTE_CFG}.new'
  chmod 0640 '${REMOTE_CFG}.new'
  mv -f '${REMOTE_CFG}.new' '${REMOTE_CFG}'
" <<EOCFG
{
  "server_url": "${SERVER_URL}",
  "token": "${API_TOKEN}",
  "source_name": "${K2B_SOURCE}",
  "sink_name": "${K2B_SINK}",
  "log_level": "${LOG_LEVEL}"
}
EOCFG
then
  echo "Config write failed — attempting restore from ${STAMP}" >&2
  restore_from "$STAMP" || true
  exit 7
fi

echo "[6/7] Installing service unit..."
"${SCP[@]}" "$SERVICE_FILE" "root@${BOARD_IP}:${REMOTE_UNIT}.new"
if ! remote "set -euo pipefail
  mv -f '${REMOTE_UNIT}.new' '${REMOTE_UNIT}'
  if [ '${USE_DISPLAY}' != 'true' ]; then
    sed -i 's/USE_DISPLAY=true/USE_DISPLAY=false/' '${REMOTE_UNIT}'
  fi
  systemctl daemon-reload
"; then
  echo "Unit install failed — attempting restore from ${STAMP}" >&2
  restore_from "$STAMP" || true
  exit 8
fi

echo "[7/7] Starting service..."
if ! remote "set -euo pipefail
  systemctl reset-failed app.service 2>/dev/null || true
  systemctl restart app.service
  sleep 3
  systemctl is-active app.service
"; then
  echo "Service failed to start — attempting restore from ${STAMP}" >&2
  restore_from "$STAMP" || true
  exit 9
fi

echo ""
echo "=== Deploy complete ==="
remote "systemctl status app --no-pager -l" || true
echo ""
echo "Backup:   ssh root@${BOARD_IP} ls -la ${ROLLBACK_ROOT}/${STAMP}"
echo "Rollback: $0 --rollback ${BOARD_IP} ${STAMP}"
echo "Logs:     ssh root@${BOARD_IP} journalctl -u app -f"
