#!/usr/bin/env bash
# Deploy ct-client to K2B and configure it to connect to a server.
#
# Usage:
#   ./deploy.sh <board-ip> <binary-path> <server-url> <api-token>
#
# Example:
#   ./deploy.sh 192.168.0.109 ../../bin/ct-client-arm64 http://192.168.0.22:8080 ct_abc123...
#
# What this does:
#   1. Stops the running service
#   2. Deploys the binary as /usr/local/bin/app
#   3. Writes /etc/app/crosstalk.json with server URL + token + PipeWire audio devices
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

K2B_SOURCE="${K2B_SOURCE:-alsa_output.platform-snd_aloop.0.analog-stereo.monitor}"
K2B_SINK="${K2B_SINK:-alsa_output.platform-snd_aloop.0.analog-stereo}"
LOG_LEVEL="${K2B_LOG_LEVEL:-debug}"

echo "=== Deploying ct-client to K2B at ${BOARD_IP} ==="

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

# 4. Install systemd unit
echo "[4/5] Installing service unit..."
$SCP "$SERVICE_FILE" "root@${BOARD_IP}:/etc/systemd/system/app.service"
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
