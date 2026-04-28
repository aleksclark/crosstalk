#!/usr/bin/env bash
set -euo pipefail
BOARD_IP="${1:?Usage: $0 <board-ip>}"
SSH="ssh -o ConnectTimeout=5 root@${BOARD_IP}"

echo "=== Live DT SPI1 node ==="
$SSH '
echo "--- SPI1 children ---"
ls /proc/device-tree/soc/spi@5011000/
echo ""

for child in /proc/device-tree/soc/spi@5011000/*/; do
    name=$(basename "$child")
    echo "--- Node: $name ---"
    if [ -f "$child/compatible" ]; then
        echo -n "  compatible: "; cat "$child/compatible" 2>/dev/null; echo ""
    fi
    if [ -f "$child/reg" ]; then
        echo -n "  reg: "; xxd -p "$child/reg" 2>/dev/null; echo ""
    fi
    if [ -f "$child/dc-gpios" ]; then
        echo -n "  dc-gpios: "; xxd -p "$child/dc-gpios" 2>/dev/null; echo ""
    fi
    if [ -f "$child/reset-gpios" ]; then
        echo -n "  reset-gpios: "; xxd -p "$child/reset-gpios" 2>/dev/null; echo ""
    fi
done

echo ""
echo "--- SPI bus devices ---"
ls -la /sys/bus/spi/devices/ 2>/dev/null || echo "(none)"

echo ""
echo "--- Full dmesg spi ---"
dmesg | grep -i spi | head -20

echo ""
echo "--- SPI1 status ---"
cat /proc/device-tree/soc/spi@5011000/status 2>/dev/null; echo ""

echo ""
echo "--- DTB check: decompile and grep ---"
dtc -I dtb -O dts /boot/dtb/allwinner/sun50i-h618-kickpi-k2b-v2.dtb 2>/dev/null | grep -A 15 "spi@5011000" | head -40
'
