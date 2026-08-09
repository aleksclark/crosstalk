#!/usr/bin/env bash
# Deploy ct-abc (Audio Booth Connector) to K2B and configure it to connect to a server.
# Historical name "ct-client" is accepted as input binary; on-device path is ct-abc.
#
# Usage:
#   ./deploy.sh <board-ip> <binary-path> <server-url> <api-token>
#
# Environment variables:
#   K2B_SOURCE       — PipeWire source name (default: physical audiocodec mic)
#   K2B_SINK         — PipeWire sink name (default: physical audiocodec output)
#   K2B_LOG_LEVEL    — log level (default: debug)
#   K2B_USE_DISPLAY  — enable SPI display (default: true)
#   K2B_AUDIO_MODE   — "physical" or "loopback" (default: physical)
#
# Example:
#   ./deploy.sh 192.168.0.109 ../../bin/ct-abc-arm64 http://192.168.0.22:8080 ct_abc123...
#
# What this does:
#   1. Stops the running service
#   2. Deploys the binary as /usr/local/bin/ct-abc (atomic temp+checksum+rename)
#   3. Installs ct-app-setup.sh helper + app.service unit
#   4. Writes /etc/app/crosstalk.json with server URL + token + audio devices
#   5. Restarts the service
set -euo pipefail

BOARD_IP="${1:?Usage: $0 <board-ip> <binary-path> <server-url> <api-token>}"
BINARY="${2:?Usage: $0 <board-ip> <binary-path> <server-url> <api-token>}"
SERVER_URL="${3:?Usage: $0 <board-ip> <binary-path> <server-url> <api-token>}"
API_TOKEN="${4:?Usage: $0 <board-ip> <binary-path> <server-url> <api-token>}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SERVICE_FILE="${SCRIPT_DIR}/deploy/app.service"
SETUP_HELPER="${SCRIPT_DIR}/deploy/ct-app-setup.sh"
REMOTE_BIN="/usr/local/bin/ct-abc"

SSH="ssh -o ConnectTimeout=5 root@${BOARD_IP}"
SCP="scp -o ConnectTimeout=5"

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

echo "=== Deploying ct-abc to K2B at ${BOARD_IP} ==="
echo "    Audio mode:  ${AUDIO_MODE}"
echo "    Source:      ${K2B_SOURCE}"
echo "    Sink:        ${K2B_SINK}"
echo "    Display:     ${USE_DISPLAY}"

LOCAL_SHA="$(sha256sum "$BINARY" | awk '{print $1}')"
STAGE="/usr/local/bin/.ct-abc.$$.new"

# 1. Stop existing service
echo "[1/6] Stopping service..."
$SSH "systemctl stop app.service 2>/dev/null || true"
sleep 1

# 2. Deploy binary atomically
echo "[2/6] Deploying binary to ${REMOTE_BIN} (sha256=${LOCAL_SHA})..."
$SCP "$BINARY" "root@${BOARD_IP}:${STAGE}"
$SSH "set -euo pipefail
  GOT=\$(sha256sum '${STAGE}' | awk '{print \$1}')
  [ \"\$GOT\" = '${LOCAL_SHA}' ]
  chmod 0755 '${STAGE}'
  mv -f '${STAGE}' '${REMOTE_BIN}'
  chmod +x '${REMOTE_BIN}'
  # Compat symlink for older docs/scripts that still look for ct-client
  ln -sfn ct-abc /usr/local/bin/ct-client
"

# 3. Install checked setup helper
echo "[3/6] Installing ct-app-setup.sh..."
$SSH "mkdir -p /usr/local/lib/crosstalk"
$SCP "$SETUP_HELPER" "root@${BOARD_IP}:/usr/local/lib/crosstalk/ct-app-setup.sh"
$SSH "chmod +x /usr/local/lib/crosstalk/ct-app-setup.sh"

# 4. Write client config
echo "[4/6] Writing config..."
$SSH "mkdir -p /etc/app"
$SSH "cat > /etc/app/crosstalk.json" <<EOCFG
{
  "server_url": "${SERVER_URL}",
  "token": "${API_TOKEN}",
  "source_name": "${K2B_SOURCE}",
  "sink_name": "${K2B_SINK}",
  "log_level": "${LOG_LEVEL}"
}
EOCFG

# 5. Install systemd unit (update USE_DISPLAY based on env)
echo "[5/6] Installing service unit..."
if [ "$USE_DISPLAY" = "true" ]; then
    $SCP "$SERVICE_FILE" "root@${BOARD_IP}:/etc/systemd/system/app.service"
else
    # Copy service file but disable display
    $SCP "$SERVICE_FILE" "root@${BOARD_IP}:/etc/systemd/system/app.service"
    $SSH "sed -i 's/USE_DISPLAY=true/USE_DISPLAY=false/' /etc/systemd/system/app.service"
fi
$SSH "systemctl daemon-reload"

# 6. Start service
echo "[6/6] Starting service..."
$SSH "systemctl reset-failed app.service 2>/dev/null; systemctl restart app.service"
sleep 3

echo ""
echo "=== Deploy complete ==="
$SSH "systemctl status app --no-pager -l" || true
echo ""
echo "Logs: ssh root@${BOARD_IP} journalctl -u app -f"
