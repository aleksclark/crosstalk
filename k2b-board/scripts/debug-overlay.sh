#!/usr/bin/env bash
# Deep debug: decompile overlays and inspect DT nodes.
# Usage: ./debug-overlay.sh <board-ip>
set -euo pipefail

BOARD_IP="${1:?Usage: $0 <board-ip>}"
SSH="ssh -o ConnectTimeout=5 root@${BOARD_IP}"

echo "--- Decompile tft35_spi overlay (reference) ---"
$SSH "dtc -I dtb -O dts /boot/dtb/allwinner/overlay/sun50i-h616-tft35_spi.dtbo 2>/dev/null" || echo "(failed)"

echo ""
echo "--- Decompile our overlay ---"
$SSH "dtc -I dtb -O dts /boot/overlay-user/ili9341-spi1.dtbo 2>/dev/null" || echo "(failed)"

echo ""
echo "--- Decompile spidev1_0 overlay (reference) ---"
$SSH "dtc -I dtb -O dts /boot/dtb/allwinner/overlay/sun50i-h616-spidev1_0.dtbo 2>/dev/null" || echo "(failed)"

echo ""
echo "--- SPI1 DT children ---"
$SSH "ls -la /proc/device-tree/soc/spi@5011000/"

echo ""
echo "--- spidev@1 compatible & reg ---"
$SSH "cat /proc/device-tree/soc/spi@5011000/spidev@1/compatible 2>/dev/null; echo ''"
$SSH "cat /proc/device-tree/soc/spi@5011000/spidev@1/reg 2>/dev/null | xxd 2>/dev/null; echo ''"
$SSH "cat /proc/device-tree/soc/spi@5011000/spidev@1/status 2>/dev/null; echo ''"

echo ""
echo "--- Check for ili9341 node anywhere in DT ---"
$SSH "find /proc/device-tree -name '*ili*' 2>/dev/null || echo '(none)'"

echo ""
echo "--- Boot log: overlay loading ---"
$SSH "dmesg | grep -iE 'overlay|user_overlay|dtbo|ili9341|fbtft.*spi|fb_ili' | head -20 || echo '(nothing)'"

echo ""
echo "--- U-Boot env (raw) ---"
$SSH "cat /proc/device-tree/chosen/bootargs 2>/dev/null; echo ''"
