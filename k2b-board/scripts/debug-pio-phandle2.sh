#!/usr/bin/env bash
set -euo pipefail
BOARD_IP="${1:?Usage: $0 <board-ip>}"
SSH="ssh -o ConnectTimeout=5 root@${BOARD_IP}"

$SSH '
# Get the numeric phandle for pinctrl@300b000 (pio)
PIO_PH=$(hexdump -e "1/4 \"0x%02x\n\"" /proc/device-tree/soc/pinctrl@300b000/phandle 2>/dev/null)
echo "pio phandle = $PIO_PH"

# Also check: what does the original DTB decompile show for existing gpio references?
DTB="/boot/dtb/allwinner/sun50i-h618-kickpi-k2b-v2.dtb.orig"
echo ""
echo "=== All gpio references in DTB ==="
dtc -I dtb -O dts "$DTB" 2>/dev/null | grep "gpios\|gpio-" | head -20

echo ""
echo "=== Decompiled DTB - full SPI section ==="
dtc -I dtb -O dts "$DTB" 2>/dev/null | sed -n "/spi@5011000/,/^[[:space:]]*};/p" | head -25
'
