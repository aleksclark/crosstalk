#!/usr/bin/env bash
# Deploy the WiFi captive-portal provisioning binary (ct-netcfg) to a K2B board.
#
# Usage:
#   ./deploy-netcfg.sh <board-ip> <binary-path>
#   ./deploy-netcfg.sh --rollback <board-ip> [stamp]
#
# Example:
#   task build:netcfg:arm64
#   ./deploy-netcfg.sh 192.168.0.109 ../../bin/ct-netcfg-arm64
#   ./deploy-netcfg.sh --rollback 192.168.0.109
#
# What this does (deploy):
#   1. Backs up the live binary + unit under /root/crosstalk-rollback/$STAMP
#   2. Stages the new binary to a temp path, verifies checksum, atomic rename
#   3. Installs/updates ct-netcfg.service, enables + restarts the unit
#   4. On failure, restores the previous binary/unit from the backup
#
# Rollback:
#   Restores binary + unit from the newest (or named) stamp and restarts.
set -euo pipefail

MODE="deploy"
if [[ "${1:-}" == "--rollback" ]]; then
  MODE="rollback"
  shift
fi

BOARD_IP="${1:?Usage: $0 [--rollback] <board-ip> [binary-path|stamp]}"
ARG2="${2:-}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SERVICE_FILE="${SCRIPT_DIR}/deploy/ct-netcfg.service"
REMOTE_BIN="/usr/local/bin/ct-netcfg"
REMOTE_UNIT="/etc/systemd/system/ct-netcfg.service"
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

backup_live() {
  local stamp="$1"
  remote "set -euo pipefail
    STAMP='${stamp}'
    DEST='${ROLLBACK_ROOT}/'\"\$STAMP\"
    mkdir -p \"\$DEST\"
    if [ -f '${REMOTE_BIN}' ]; then
      cp -a '${REMOTE_BIN}' \"\$DEST/ct-netcfg\"
      sha256sum '${REMOTE_BIN}' > \"\$DEST/ct-netcfg.sha256\"
    fi
    if [ -f '${REMOTE_UNIT}' ]; then
      cp -a '${REMOTE_UNIT}' \"\$DEST/ct-netcfg.service\"
      sha256sum '${REMOTE_UNIT}' > \"\$DEST/ct-netcfg.service.sha256\"
    fi
    {
      echo \"stamp=\$STAMP\"
      echo \"when=\$(date -u +%Y-%m-%dT%H:%M:%SZ)\"
      echo \"hostname=\$(hostname)\"
      systemctl is-active ct-netcfg.service 2>/dev/null || true
      systemctl is-enabled ct-netcfg.service 2>/dev/null || true
      nmcli -t -f DEVICE,TYPE,STATE,CONNECTION dev status 2>/dev/null || true
      ip -br a 2>/dev/null || true
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
    systemctl stop ct-netcfg.service 2>/dev/null || true
    if [ -f \"\$SRC/ct-netcfg\" ]; then
      install -m 0755 \"\$SRC/ct-netcfg\" '${REMOTE_BIN}.restoring'
      if [ -f \"\$SRC/ct-netcfg.sha256\" ]; then
        WANT=\$(awk '{print \$1}' \"\$SRC/ct-netcfg.sha256\")
        GOT=\$(sha256sum '${REMOTE_BIN}.restoring' | awk '{print \$1}')
        [ \"\$WANT\" = \"\$GOT\" ] || { echo \"binary checksum mismatch on restore\" >&2; exit 3; }
      fi
      mv -f '${REMOTE_BIN}.restoring' '${REMOTE_BIN}'
      chmod +x '${REMOTE_BIN}'
    fi
    if [ -f \"\$SRC/ct-netcfg.service\" ]; then
      install -m 0644 \"\$SRC/ct-netcfg.service\" '${REMOTE_UNIT}'
    fi
    systemctl daemon-reload
    systemctl reset-failed ct-netcfg.service 2>/dev/null || true
    systemctl enable ct-netcfg.service
    systemctl restart ct-netcfg.service
    sleep 1
    systemctl is-active ct-netcfg.service || true
    echo \"ROLLBACK_OK ${stamp}\"
  "
}

if [[ "$MODE" == "rollback" ]]; then
  STAMP="${ARG2:-}"
  if [[ -z "$STAMP" ]]; then
    STAMP="$(remote "ls -1 '${ROLLBACK_ROOT}' 2>/dev/null | sort | tail -1")"
  fi
  if [[ -z "$STAMP" ]]; then
    echo "No rollback stamps found on ${BOARD_IP}:${ROLLBACK_ROOT}" >&2
    exit 2
  fi
  echo "=== Rolling back ct-netcfg on ${BOARD_IP} from stamp ${STAMP} ==="
  restore_from "$STAMP"
  remote "systemctl status ct-netcfg --no-pager -l" || true
  exit 0
fi

BINARY="${ARG2:?Usage: $0 <board-ip> <binary-path>}"
if [[ ! -f "$BINARY" ]]; then
  echo "binary not found: $BINARY" >&2
  exit 1
fi

LOCAL_SHA="$(sha256_local "$BINARY")"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
STAGE="/usr/local/bin/.ct-netcfg.${STAMP}.new"

echo "=== Deploying ct-netcfg to K2B at ${BOARD_IP} ==="
echo "    binary: ${BINARY}"
echo "    sha256: ${LOCAL_SHA}"
echo "    stamp:  ${STAMP}"

echo "[1/5] Capturing pre-state backup under ${ROLLBACK_ROOT}/${STAMP}..."
backup_live "$STAMP"

echo "[2/5] Stopping service (frees the running binary)..."
remote "systemctl stop ct-netcfg.service 2>/dev/null || true"

echo "[3/5] Atomic binary deploy (temp + checksum + rename)..."
# Stage to a unique temp name, verify hash on the board, then rename into place.
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
  # prove final path hash
  FINAL=\$(sha256sum '${REMOTE_BIN}' | awk '{print \$1}')
  [ \"\$FINAL\" = \"\$WANT\" ]
  echo \"BINARY_OK \$FINAL\"
"; then
  echo "Binary deploy failed — attempting restore from ${STAMP}" >&2
  restore_from "$STAMP" || true
  exit 5
fi

echo "[4/5] Installing service unit..."
"${SCP[@]}" "$SERVICE_FILE" "root@${BOARD_IP}:${REMOTE_UNIT}.new"
if ! remote "set -euo pipefail
  mv -f '${REMOTE_UNIT}.new' '${REMOTE_UNIT}'
  systemctl daemon-reload
  systemctl enable ct-netcfg.service
"; then
  echo "Unit install failed — attempting restore from ${STAMP}" >&2
  restore_from "$STAMP" || true
  exit 6
fi

echo "[5/5] Starting service..."
if ! remote "set -euo pipefail
  systemctl reset-failed ct-netcfg.service 2>/dev/null || true
  systemctl restart ct-netcfg.service
  sleep 2
  systemctl is-active ct-netcfg.service
"; then
  echo "Service failed to start — attempting restore from ${STAMP}" >&2
  restore_from "$STAMP" || true
  exit 7
fi

echo ""
echo "=== Deploy complete ==="
remote "systemctl status ct-netcfg --no-pager -l" || true
echo ""
echo "Backup:  ssh root@${BOARD_IP} ls -la ${ROLLBACK_ROOT}/${STAMP}"
echo "Rollback: $0 --rollback ${BOARD_IP} ${STAMP}"
echo "Logs:     ssh root@${BOARD_IP} journalctl -u ct-netcfg -f"
