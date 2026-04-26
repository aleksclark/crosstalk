#!/usr/bin/env bash
set -euo pipefail
SSH="ssh -o ConnectTimeout=5 root@${1:?Usage: $0 <board-ip>}"

echo "--- SPI1 pin mux (PH5-PH9 = pins 229-233) ---"
$SSH "cat /sys/kernel/debug/pinctrl/300b000.pinctrl/pinmux-pins | grep -E 'pin (229|230|231|232|233) '"

echo ""
echo "--- SPI1 pinctrl-0 phandles ---"
$SSH "od -An -tx1 /proc/device-tree/soc/spi@5011000/pinctrl-0 2>/dev/null"

echo ""
echo "--- Resolve those phandles to pin groups ---"
$SSH '
DTB="/boot/dtb/allwinner/sun50i-h618-kickpi-k2b-v2.dtb"
DTS=$(dtc -I dtb -O dts "$DTB" 2>/dev/null)

# Find pinctrl groups referenced by spi@5011000
echo "$DTS" | grep -A3 "spi1-pins\|spi1_pins\|spi1-cs" | head -20
echo "---"
# Find all SPI pin groups
echo "$DTS" | grep -B1 -A5 "function.*=.*\"spi1\"" | head -30
'
