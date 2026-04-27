#!/usr/bin/env bash
# Deploy the updated ct-client (framebuffer version) and service to K2B.
# Reads the existing config from the board to preserve server URL + token.
set -euo pipefail
BOARD_IP="${1:?Usage: $0 <board-ip> <binary-path>}"
BINARY="${2:?Usage: $0 <board-ip> <binary-path>}"
SSH="ssh -o ConnectTimeout=5 root@${BOARD_IP}"
SCP="scp -o ConnectTimeout=5"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "=== Deploying framebuffer ct-client to ${BOARD_IP} ==="

# 1. Stop
echo "[1/4] Stopping service..."
$SSH "systemctl stop app.service 2>/dev/null || true"
sleep 1

# 2. Deploy binary
echo "[2/4] Deploying binary..."
$SCP "$BINARY" "root@${BOARD_IP}:/usr/local/bin/ct-client"
$SSH "chmod +x /usr/local/bin/ct-client"

# 3. Install updated service file
echo "[3/4] Installing service unit..."
$SCP "${SCRIPT_DIR}/deploy/app.service" "root@${BOARD_IP}:/etc/systemd/system/app.service"
$SCP "${SCRIPT_DIR}/deploy/ct-splash.sh" "root@${BOARD_IP}:/usr/local/bin/ct-splash.sh"
$SSH "chmod +x /usr/local/bin/ct-splash.sh; systemctl daemon-reload"

# 4. Start
echo "[4/4] Starting service..."
$SSH "systemctl reset-failed app.service 2>/dev/null; systemctl restart app.service"
sleep 3

echo ""
echo "=== Deploy complete ==="
$SSH "systemctl status app --no-pager -l" || true
echo ""
$SSH "journalctl -u app --no-pager -l -n 15"
