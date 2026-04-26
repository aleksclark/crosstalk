#!/usr/bin/env bash
# Quick post-reboot check for display.
# Usage: ./check-display.sh <board-ip>
set -euo pipefail
BOARD_IP="${1:?Usage: $0 <board-ip>}"
SSH="ssh -o ConnectTimeout=10 root@${BOARD_IP}"

echo "--- FB devices ---"
$SSH "ls -la /dev/fb* 2>/dev/null || echo 'no /dev/fb*'"

echo "--- display-init service ---"
$SSH "systemctl status display-init --no-pager -l 2>/dev/null | head -20 || echo '(not found)'"

echo "--- dmesg: spi/fbtft/ili ---"
$SSH "dmesg | grep -iE 'spi|fbtft|ili|fb[0-9]|driver_override|init-display' | tail -25"

echo "--- SPI bus devices ---"
$SSH "ls /sys/bus/spi/devices/ 2>/dev/null || echo '(none)'"

echo "--- SPI1.1 details ---"
$SSH "
if [ -d /sys/bus/spi/devices/spi1.1 ]; then
    echo 'driver_override:'; cat /sys/bus/spi/devices/spi1.1/driver_override 2>/dev/null; echo ''
    echo 'driver:'; ls -la /sys/bus/spi/devices/spi1.1/driver 2>/dev/null || echo '(no driver bound)'
    echo 'modalias:'; cat /sys/bus/spi/devices/spi1.1/modalias 2>/dev/null; echo ''
else
    echo 'spi1.1 does not exist'
fi
"

echo "--- App service ---"
$SSH "systemctl status app --no-pager -l 2>/dev/null | head -10 || true"
