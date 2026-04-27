#!/usr/bin/env bash
# Deploy ct-client to K2B and configure it to connect to a server.
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
#   ./deploy.sh 192.168.0.109 ../../bin/ct-client-arm64 http://192.168.0.22:8080 ct_abc123...
#
# What this does:
#   1. Stops the running service
#   2. Deploys the binary as /usr/local/bin/ct-client
#   3. Writes /etc/app/crosstalk.json with server URL + token + audio devices
#   4. Installs/updates the systemd unit (sets CROSSTALK_CONFIG + XDG_RUNTIME_DIR)
#   5. Restarts the service
set -euo pipefail

BOARD_IP="${1:?Usage: $0 <board-ip> <binary-path> <server-url> <api-token>}"
BINARY="${2:?Usage: $0 <board-ip> <binary-path> <server-url> <api-token>}"
SERVER_URL="${3:?Usage: $0 <board-ip> <binary-path> <server-url> <api-token>}"
API_TOKEN="${4:?Usage: $0 <board-ip> <binary-path> <server-url> <api-token>}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SERVICE_FILE="${SCRIPT_DIR}/deploy/app.service"

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
    # Physical audio via USB adapter (C-Media, ALSA card 3) or onboard codec.
    # Use ALSA hw: devices directly to bypass PipeWire resampler distortion.
    K2B_SOURCE="${K2B_SOURCE:-hw:3,0}"
    K2B_SINK="${K2B_SINK:-hw:3,0}"
fi

echo "=== Deploying ct-client to K2B at ${BOARD_IP} ==="
echo "    Audio mode:  ${AUDIO_MODE}"
echo "    Source:      ${K2B_SOURCE}"
echo "    Sink:        ${K2B_SINK}"
echo "    Display:     ${USE_DISPLAY}"

# 1. Stop existing service
echo "[1/5] Stopping service..."
$SSH "systemctl stop app.service 2>/dev/null || true"
sleep 1

# 2. Deploy binary
echo "[2/5] Deploying binary..."
$SCP "$BINARY" "root@${BOARD_IP}:/usr/local/bin/ct-client"
$SSH "chmod +x /usr/local/bin/ct-client"

# 3. Write client config
echo "[3/5] Writing config..."
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

# 4. Install systemd unit (update USE_DISPLAY based on env)
echo "[4/5] Installing service unit..."
if [ "$USE_DISPLAY" = "true" ]; then
    $SCP "$SERVICE_FILE" "root@${BOARD_IP}:/etc/systemd/system/app.service"
else
    # Copy service file but disable display
    $SCP "$SERVICE_FILE" "root@${BOARD_IP}:/etc/systemd/system/app.service"
    $SSH "sed -i 's/USE_DISPLAY=true/USE_DISPLAY=false/' /etc/systemd/system/app.service"
fi
$SSH "systemctl daemon-reload"

# 5. Start service
echo "[5/5] Starting service..."
$SSH "systemctl reset-failed app.service 2>/dev/null; systemctl restart app.service"
sleep 3

echo ""
echo "=== Deploy complete ==="
$SSH "systemctl status app --no-pager -l" || true
echo ""
echo "Logs: ssh root@${BOARD_IP} journalctl -u app -f"
