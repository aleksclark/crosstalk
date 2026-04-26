#!/usr/bin/env bash
# Find the pio phandle value in the DTB.
# Usage: ./find-phandle.sh <board-ip>
set -euo pipefail
BOARD_IP="${1:?Usage: $0 <board-ip>}"
SSH="ssh -o ConnectTimeout=5 root@${BOARD_IP}"

$SSH '
DTB="/boot/dtb/allwinner/sun50i-h618-kickpi-k2b-v2.dtb.bak"
[ -f "$DTB" ] || DTB="/boot/dtb/allwinner/sun50i-h618-kickpi-k2b-v2.dtb"

DTS=$(dtc -I dtb -O dts "$DTB" 2>/dev/null)

echo "--- pio (pinctrl@300b000) phandle ---"
echo "$DTS" | grep -A2 "300b000" | grep phandle | head -3

echo "--- spidev@1 block ---"
echo "$DTS" | grep -B2 -A10 "spidev@1"

echo "--- spi@5011000 block (first 20 lines) ---"
echo "$DTS" | grep -A20 "spi@5011000 {"  | head -25

echo "--- all phandle assignments (first 20) ---"
echo "$DTS" | grep "phandle = " | head -20
'
