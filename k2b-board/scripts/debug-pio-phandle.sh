#!/usr/bin/env bash
set -euo pipefail
BOARD_IP="${1:?Usage: $0 <board-ip>}"
SSH="ssh -o ConnectTimeout=5 root@${BOARD_IP}"

$SSH '
# Find the pio phandle value from the decompiled DTB
DTB="/boot/dtb/allwinner/sun50i-h618-kickpi-k2b-v2.dtb.orig"
dtc -I dtb -O dts "$DTB" 2>/dev/null | grep -B5 "pio\b" | head -20

echo "=== pio phandle ==="
dtc -I dtb -O dts "$DTB" 2>/dev/null | grep -A2 "pinctrl@" | head -10

echo "=== Full SPI node with phandles ==="
dtc -I dtb -O dts "$DTB" 2>/dev/null | grep -A 30 "spi@5011000 {" | head -35

echo "=== Phandle for pio ==="
xxd -p /proc/device-tree/soc/pinctrl@300b000/phandle 2>/dev/null || echo "(not found)"
# Try alternate path
xxd -p /proc/device-tree/soc/pio/phandle 2>/dev/null || echo "(no /soc/pio)"
# Generic search
for p in /proc/device-tree/soc/pinctrl*/phandle; do
    echo "  $p: $(xxd -p "$p" 2>/dev/null)"
done
'
