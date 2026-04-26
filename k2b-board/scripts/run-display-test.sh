#!/usr/bin/env bash
set -euo pipefail
BOARD_IP="${1:?Usage: $0 <board-ip>}"
SSH="ssh -o ConnectTimeout=5 root@${BOARD_IP}"
SCP="scp -o ConnectTimeout=5"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

BIN="${SCRIPT_DIR}/bin/display-test-arm64"
if [ ! -f "$BIN" ]; then
    echo "Build first: cd cli && GOOS=linux GOARCH=arm64 go build -o ../bin/display-test-arm64 ./cmd/display-test/"
    exit 1
fi

echo "Deploying display-test..."
$SSH "systemctl stop app 2>/dev/null || true"
$SCP "$BIN" "root@${BOARD_IP}:/tmp/display-test"
$SSH "chmod +x /tmp/display-test"

echo "Exporting GPIOs and running test..."
$SSH "
echo 71 > /sys/class/gpio/export 2>/dev/null || true
echo out > /sys/class/gpio/gpio71/direction 2>/dev/null || true
echo 76 > /sys/class/gpio/export 2>/dev/null || true
echo out > /sys/class/gpio/gpio76/direction 2>/dev/null || true
/tmp/display-test
"
