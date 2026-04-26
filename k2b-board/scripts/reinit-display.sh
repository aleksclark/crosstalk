#!/usr/bin/env bash
set -euo pipefail
SSH="ssh -o ConnectTimeout=5 root@${1:?Usage: $0 <board-ip>}"

echo "--- Stop ct-client ---"
$SSH "systemctl stop app 2>/dev/null || true"

echo "--- Reload fbtft driver (re-init display) ---"
$SSH "
rmmod fb_ili9341 2>/dev/null || true
rmmod fbtft 2>/dev/null || true
sleep 0.5
modprobe fb_ili9341
sleep 1
dmesg | tail -10
"

echo ""
echo "--- Check fb0 exists ---"
$SSH "ls -la /dev/fb0 2>/dev/null || echo 'NO fb0'"

echo ""
echo "--- Write red pattern ---"
$SSH '
python3 -c "
import sys
sys.stdout.buffer.write(b\"\\x00\\xf8\" * (320 * 240))
" > /dev/fb0
echo "Red written"
'

echo ""
echo "--- Check dmesg for errors ---"
$SSH "dmesg | grep -iE 'fbtft|ili|fb0|spi0' | tail -15"

echo ""
echo "Is the display showing RED? If not, try:"
echo "  ssh root@$1 'cat /dev/urandom > /dev/fb0'"
echo "  (should show random colored noise)"
