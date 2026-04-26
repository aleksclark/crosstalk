#!/usr/bin/env bash
set -euo pipefail
SSH="ssh -o ConnectTimeout=5 root@${1:?Usage: $0 <board-ip>}"
$SSH '
echo "--- #gpio-cells ---"
cat /proc/device-tree/soc/pinctrl@300b000/#gpio-cells 2>/dev/null | xxd
echo ""
echo "--- existing gpio references in DTB ---"
DTB="/boot/dtb/allwinner/sun50i-h618-kickpi-k2b-v2.dtb.bak"
[ -f "$DTB" ] || DTB="/boot/dtb/allwinner/sun50i-h618-kickpi-k2b-v2.dtb"
dtc -I dtb -O dts "$DTB" 2>/dev/null | grep "gpios\s*=" | head -10
'
