#!/usr/bin/env bash
# Provision the MSP2401 ILI9341 SPI display on a K2B board.
#
# Wiring (K2B 20-pin header):
#   MSP2401    K2B Pin   Signal
#   VCC     →  2         VCC_3V3
#   GND     →  6         GND
#   SCK     →  13        SPI1_CLK  / GPIO_PH6
#   SDI     →  9         SPI1_MOSI / GPIO_PH7
#   SDO     →  11        SPI1_MISO / GPIO_PH8 (optional)
#   CS      →  7         SPI1_CS1  / GPIO_PH9
#   DC/RS   →  14        GPIO_PC7  (PH4/PH2 claimed by uart5/i2c3)
#   RESET   →  12        GPIO_PC12
#   LED     →  PWM1      GPIO_PH3 (backlight)
#
# Approach: Use the stock DTB with spidev@1 on SPI1. The Go ct-client
# drives the ILI9341 directly over /dev/spidev0.1 with GPIO for DC/RST.
# fbtft is blacklisted to prevent it from stealing the SPI device.
#
# Usage: ./provision-display.sh <board-ip>
set -euo pipefail

BOARD_IP="${1:?Usage: $0 <board-ip>}"
SSH="ssh -o ConnectTimeout=5 root@${BOARD_IP}"

echo "=== Provisioning MSP2401 display on K2B at ${BOARD_IP} ==="

# 1. Restore original DTB (ensure spidev@1 is present)
echo "[1/4] Restoring original DTB..."
$SSH '
DTB="/boot/dtb/allwinner/sun50i-h618-kickpi-k2b-v2.dtb"
BACKUP="${DTB}.bak"
if [ -f "${BACKUP}" ]; then
    cp "${BACKUP}" "${DTB}"
    echo "Restored original DTB from backup"
else
    echo "DTB already original"
fi
'

# 2. Configure boot and modules
echo "[2/4] Configuring boot and modules..."
$SSH '
# Clean up old overlay config
sed -i "/^user_overlays=/d" /boot/armbianEnv.txt
sed -i "s/ *spidev1_1//g" /boot/armbianEnv.txt
sed -i "/^overlays= *$/d" /boot/armbianEnv.txt

# Blacklist fbtft so it does not steal the SPI device
cat > /etc/modprobe.d/blacklist-fbtft.conf <<EOF
blacklist fb_ili9341
blacklist fbtft
blacklist panel_ilitek_ili9341
blacklist ili9341
EOF

# Remove old fbtft autoload
rm -f /etc/modules-load.d/fb-ili9341.conf

# Ensure spidev loads
echo spidev > /etc/modules-load.d/spidev.conf

# Remove old display-init service
systemctl disable display-init.service 2>/dev/null || true
rm -f /etc/systemd/system/display-init.service /usr/local/bin/init-display.sh
systemctl daemon-reload 2>/dev/null || true

echo "armbianEnv.txt:"
cat /boot/armbianEnv.txt
'

# 3. Set up permissions
echo "[3/4] Setting up permissions..."
$SSH '
# spidev access
groupadd -f spi
usermod -aG spi streamlate 2>/dev/null || usermod -aG spi app 2>/dev/null || true
cat > /etc/udev/rules.d/99-spidev.rules <<EOF
SUBSYSTEM=="spidev", GROUP="spi", MODE="0660"
EOF

# GPIO access for DC (PC7=71) and RESET (PC12=76)
cat > /etc/udev/rules.d/99-gpio.rules <<EOF
SUBSYSTEM=="gpio", KERNEL=="gpiochip*", ACTION=="add", MODE="0660", GROUP="spi"
SUBSYSTEM=="gpio", ACTION=="add", ATTR{direction}=="*", MODE="0660", GROUP="spi"
EOF

# Video group
usermod -aG video streamlate 2>/dev/null || usermod -aG video app 2>/dev/null || true
'

# 4. Reboot
echo "[4/4] Rebooting..."
$SSH "reboot" 2>/dev/null || true

echo "Waiting for board to go down..."
sleep 5

echo "Waiting for board to come back (up to 90s)..."
TRIES=0
while [ $TRIES -lt 18 ]; do
    if $SSH -o ConnectTimeout=3 "true" 2>/dev/null; then
        break
    fi
    TRIES=$((TRIES + 1))
    sleep 5
done

if ! $SSH "true" 2>/dev/null; then
    echo "ERROR: Board did not come back after reboot."
    exit 1
fi

echo ""
echo "=== Post-reboot verification ==="

echo "--- SPI devices ---"
$SSH "ls -la /dev/spidev* 2>/dev/null || echo 'FAIL: no spidev'"

echo "--- fbtft modules (should be empty) ---"
$SSH "lsmod | grep -iE 'fbtft|ili' && echo 'WARN: fbtft still loaded!' || echo 'OK: fbtft not loaded'"

echo "--- spidev module ---"
$SSH "lsmod | grep spidev || echo 'WARN: spidev not loaded'"

echo "--- GPIO exportable ---"
$SSH "echo 71 > /sys/class/gpio/export 2>/dev/null; echo 76 > /sys/class/gpio/export 2>/dev/null; ls /sys/class/gpio/gpio71 /sys/class/gpio/gpio76 2>/dev/null && echo 'OK: GPIOs accessible' || echo 'WARN: GPIO export failed'; echo 71 > /sys/class/gpio/unexport 2>/dev/null; echo 76 > /sys/class/gpio/unexport 2>/dev/null"

echo "--- App service ---"
$SSH "systemctl status app --no-pager -l 2>/dev/null | head -10 || true"

if $SSH "test -e /dev/spidev0.1" 2>/dev/null; then
    echo ""
    echo "=== PASS: /dev/spidev0.1 available ==="
else
    echo ""
    echo "=== FAIL: no spidev device ==="
    exit 1
fi
