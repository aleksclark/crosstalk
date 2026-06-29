#!/usr/bin/env bash
set -euo pipefail
BOARD_IP="${1:?Usage: $0 <board-ip>}"
SSH="ssh -o ConnectTimeout=5 root@${BOARD_IP}"

echo "=== armbianEnv.txt ==="
$SSH "cat /boot/armbianEnv.txt"

echo ""
echo "=== SPI1 children in live DT ==="
$SSH "ls /proc/device-tree/soc/spi@5011000/"

echo ""
echo "=== spidev@1 compatible ==="
$SSH "xxd /proc/device-tree/soc/spi@5011000/spidev@1/compatible 2>/dev/null || echo 'no spidev@1'"

echo ""
echo "=== ili9341@1 node? ==="
$SSH "ls /proc/device-tree/soc/spi@5011000/ili9341@1/ 2>/dev/null || echo 'no ili9341 node'"

echo ""
echo "=== dmesg overlay ==="
$SSH "dmesg | grep -iE 'overlay|dtbo|user_over' | head -20 || echo '(nothing)'"

echo ""
echo "=== SPI bus devices ==="
$SSH "ls -la /sys/bus/spi/devices/"

echo ""
echo "=== spi0.1 modalias and driver ==="
$SSH "cat /sys/bus/spi/devices/spi0.1/modalias 2>/dev/null || echo 'no modalias'"
$SSH "readlink /sys/bus/spi/devices/spi0.1/driver 2>/dev/null || echo 'no driver bound'"

echo ""
echo "=== Try manual unbind spidev + bind ili9341 ==="
$SSH '
echo spi0.1 > /sys/bus/spi/drivers/spidev/unbind 2>/dev/null || echo "unbind failed"
echo "adafruit,yx240qv29" > /sys/bus/spi/devices/spi0.1/driver_override 2>/dev/null || echo "override failed"
echo spi0.1 > /sys/bus/spi/drivers/ili9341/bind 2>/dev/null || echo "bind to ili9341 failed"
sleep 1
ls -la /dev/fb* 2>/dev/null || echo "still no fb"
dmesg | tail -10
'
