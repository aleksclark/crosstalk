#!/usr/bin/env bash
# Debug: try to get ILI9341 working as framebuffer via fbtft staging driver.
# The tinydrm overlay approach failed because Armbian's bootscript isn't
# applying user_overlays on this board. Fall back to fbtft which can be
# loaded with explicit parameters.
set -euo pipefail
BOARD_IP="${1:?Usage: $0 <board-ip>}"
SSH="ssh -o ConnectTimeout=5 root@${BOARD_IP}"

echo "=== Attempting fbtft fb_ili9341 with explicit parameters ==="
$SSH '
set -x

# Stop ct-client so it does not fight for SPI
systemctl stop app 2>/dev/null || true

# Unload any previous attempts
rmmod fb_ili9341 2>/dev/null || true
rmmod fbtft 2>/dev/null || true
rmmod ili9341 2>/dev/null || true

# Unbind spidev from the device so fbtft can claim it
if [ -e /sys/bus/spi/drivers/spidev/spi0.1 ]; then
    echo spi0.1 > /sys/bus/spi/drivers/spidev/unbind
    echo "spidev unbound"
fi

# Export GPIOs for DC and RST
for g in 71 76; do
    echo $g > /sys/class/gpio/export 2>/dev/null || true
    echo out > /sys/class/gpio/gpio$g/direction 2>/dev/null || true
done

# Set driver_override to fbtft before loading
echo fb_ili9341 > /sys/bus/spi/devices/spi0.1/driver_override

# Load fbtft with parameters
# gpios: dc=71, reset=76
# speed: 32MHz, rotate: 90
modprobe fbtft
modprobe fb_ili9341

# Try to bind
echo spi0.1 > /sys/bus/spi/drivers/fb_ili9341/bind 2>/dev/null || true

sleep 1

echo ""
echo "=== Results ==="
ls -la /dev/fb* 2>/dev/null || echo "no /dev/fb*"
dmesg | grep -iE "fbtft|fb_ili|fb0|fb1" | tail -15
cat /sys/bus/spi/devices/spi0.1/driver_override 2>/dev/null || true
readlink /sys/bus/spi/devices/spi0.1/driver 2>/dev/null || echo "no driver"
'
