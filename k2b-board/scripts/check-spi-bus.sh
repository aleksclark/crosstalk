#!/usr/bin/env bash
set -euo pipefail
SSH="ssh -o ConnectTimeout=5 root@${1:?Usage: $0 <board-ip>}"

echo "--- Which SPI bus is fb_ili9341 on? ---"
$SSH "ls -la /sys/class/graphics/fb0/device 2>/dev/null"

echo ""
echo "--- SPI bus devices ---"
$SSH "ls -la /sys/bus/spi/devices/"

echo ""
echo "--- SPI controllers ---"
$SSH "ls /sys/class/spi_master/"

echo ""
echo "--- SPI0 DT node ---"
$SSH "
if [ -d /proc/device-tree/soc/spi@5010000 ]; then
    echo 'spi@5010000 (SPI0):'
    cat /proc/device-tree/soc/spi@5010000/status 2>/dev/null; echo ''
    ls /proc/device-tree/soc/spi@5010000/ | head -20
fi
"

echo ""
echo "--- SPI1 DT node ---"
$SSH "
if [ -d /proc/device-tree/soc/spi@5011000 ]; then
    echo 'spi@5011000 (SPI1):'
    cat /proc/device-tree/soc/spi@5011000/status 2>/dev/null; echo ''
    ls /proc/device-tree/soc/spi@5011000/
fi
"

echo ""
echo "--- What is spi0? ---"
$SSH "
cat /sys/class/spi_master/spi0/device/of_node/name 2>/dev/null; echo ''
od -An -tx1 /sys/class/spi_master/spi0/device/of_node/reg 2>/dev/null | head -1
"

echo ""
echo "--- dmesg SPI ---"
$SSH "dmesg | grep -i spi | head -20"
