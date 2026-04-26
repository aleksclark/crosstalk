#!/usr/bin/env bash
set -euo pipefail
BOARD_IP="${1:?Usage: $0 <board-ip>}"
SSH="ssh -o ConnectTimeout=5 root@${BOARD_IP}"

$SSH '
DTB="/boot/dtb/allwinner/sun50i-h618-kickpi-k2b-v2.dtb.bak"
[ -f "$DTB" ] || DTB="/boot/dtb/allwinner/sun50i-h618-kickpi-k2b-v2.dtb"
DTS=$(dtc -I dtb -O dts "$DTB" 2>/dev/null)

echo "--- pinctrl@300b000 phandle ---"
echo "$DTS" | grep -A30 "pinctrl@300b000 {" | grep phandle | head -1

echo "--- existing gpio-specifier in spidev@1 area (for reference) ---"
echo "$DTS" | grep -B5 -A20 "spidev@1"
'
