#!/usr/bin/env bash
# Deploy a new binary to the K2B and restart the service.
# Usage: ./deploy.sh <board-ip> <binary-path>
set -euo pipefail

BOARD_IP="${1:?Usage: $0 <board-ip> <binary-path>}"
BINARY="${2:?Usage: $0 <board-ip> <binary-path>}"
SSH="ssh -i ~/.ssh/id_rsa root@${BOARD_IP}"
SCP="scp -i ~/.ssh/id_rsa"

echo "Deploying $(basename "$BINARY") to ${BOARD_IP}..."
$SCP "$BINARY" "root@${BOARD_IP}:/usr/local/bin/app"
$SSH "chmod +x /usr/local/bin/app && systemctl restart app"
echo "Done. Service restarted."
$SSH "systemctl status app --no-pager -l" || true
