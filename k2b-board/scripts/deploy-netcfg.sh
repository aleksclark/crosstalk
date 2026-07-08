#!/usr/bin/env bash
# Deploy the WiFi captive-portal provisioning binary (ct-netcfg) to a K2B board.
#
# Usage:
#   ./deploy-netcfg.sh <board-ip> <binary-path>
#
# Example:
#   task build:netcfg:arm64
#   ./deploy-netcfg.sh 192.168.0.109 ../../bin/ct-netcfg-arm64
#
# What this does:
#   1. Deploys the binary as /usr/local/bin/ct-netcfg
#   2. Installs/updates the ct-netcfg.service systemd unit
#   3. Enables + restarts the service
set -euo pipefail

BOARD_IP="${1:?Usage: $0 <board-ip> <binary-path>}"
BINARY="${2:?Usage: $0 <board-ip> <binary-path>}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SERVICE_FILE="${SCRIPT_DIR}/deploy/ct-netcfg.service"

SSH="ssh -o ConnectTimeout=5 root@${BOARD_IP}"
SCP="scp -o ConnectTimeout=5"

echo "=== Deploying ct-netcfg to K2B at ${BOARD_IP} ==="

echo "[1/3] Deploying binary..."
$SCP "$BINARY" "root@${BOARD_IP}:/usr/local/bin/ct-netcfg"
$SSH "chmod +x /usr/local/bin/ct-netcfg"

echo "[2/3] Installing service unit..."
$SCP "$SERVICE_FILE" "root@${BOARD_IP}:/etc/systemd/system/ct-netcfg.service"
$SSH "systemctl daemon-reload && systemctl enable ct-netcfg"

echo "[3/3] Restarting service..."
$SSH "systemctl reset-failed ct-netcfg.service 2>/dev/null; systemctl restart ct-netcfg.service"
sleep 2

echo ""
echo "=== Deploy complete ==="
$SSH "systemctl status ct-netcfg --no-pager -l" || true
echo ""
echo "Logs: ssh root@${BOARD_IP} journalctl -u ct-netcfg -f"
