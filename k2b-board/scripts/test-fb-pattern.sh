#!/usr/bin/env bash
set -euo pipefail
SSH="ssh -o ConnectTimeout=5 root@${1:?Usage: $0 <board-ip>}"

echo "--- Writing bright red test pattern to fb0 ---"
$SSH '
# 320x240 at 16bpp = 153600 bytes
# RGB565 red = 0xF800, little-endian = 0x00 0xF8
python3 -c "
import sys
# Bright red in RGB565 LE
red = b\"\\x00\\xf8\" * (320 * 240)
sys.stdout.buffer.write(red)
" > /dev/fb0 && echo "Red pattern written" || echo "Write FAILED"
'

echo "Check if the display shows solid RED now."
echo ""
echo "--- Writing bright white ---"
$SSH '
python3 -c "
import sys
white = b\"\\xff\\xff\" * (320 * 240)
sys.stdout.buffer.write(white)
" > /dev/fb0 && echo "White pattern written" || echo "Write FAILED"
'
echo "Check if display shows solid WHITE now."
