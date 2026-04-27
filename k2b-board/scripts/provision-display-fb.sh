#!/usr/bin/env bash
# Provision the ILI9341 SPI display as a kernel framebuffer on K2B.
#
# This replaces the userspace SPI approach with a proper kernel driver
# (tinydrm ili9341) that exposes /dev/fb. Benefits:
#   - Early boot splash via fbcon before ct-client starts
#   - Standard framebuffer interface (mmap /dev/fb)
#   - No GPIO export dance in ExecStartPre
#   - Plymouth/fbcon console support
#
# Wiring (K2B 20-pin header):
#   MSP2401    K2B Pin   Signal           GPIO
#   VCC     →  2         VCC_3V3
#   GND     →  6         GND
#   SCK     →  13        SPI1_CLK  / PH6
#   SDI     →  9         SPI1_MOSI / PH7
#   SDO     →  11        SPI1_MISO / PH8  (optional)
#   CS      →  7         SPI1_CS1  / PH9
#   DC/RS   →  14        GPIO_PC7         (linux gpio 71)
#   RESET   →  12        GPIO_PC12        (linux gpio 76)
#   LED     →  PWM1      GPIO_PH3         (backlight)
#
# Usage: ./provision-display-fb.sh <board-ip>
set -euo pipefail

BOARD_IP="${1:?Usage: $0 <board-ip>}"
SSH="ssh -o ConnectTimeout=5 root@${BOARD_IP}"
SCP="scp -o ConnectTimeout=5"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "=== Provisioning ILI9341 framebuffer on K2B at ${BOARD_IP} ==="

# 1. Install the DT overlay
echo "[1/5] Installing device tree overlay..."
$SSH "mkdir -p /boot/overlay-user"
$SCP "${SCRIPT_DIR}/config/overlays/ili9341-spi1.dts" "root@${BOARD_IP}:/tmp/ili9341-spi1.dts"
$SSH '
# Compile the DTS overlay to DTBO
if command -v armbian-add-overlay &>/dev/null; then
    armbian-add-overlay /tmp/ili9341-spi1.dts
else
    dtc -@ -I dts -O dtb -o /boot/overlay-user/ili9341-spi1.dtbo /tmp/ili9341-spi1.dts
    # Add to armbianEnv.txt if not already there
    if ! grep -q "user_overlays=ili9341-spi1" /boot/armbianEnv.txt; then
        if grep -q "^user_overlays=" /boot/armbianEnv.txt; then
            sed -i "s/^user_overlays=.*/& ili9341-spi1/" /boot/armbianEnv.txt
        else
            echo "user_overlays=ili9341-spi1" >> /boot/armbianEnv.txt
        fi
    fi
fi
echo "Overlay installed"
'

# 2. Remove fbtft blacklist and ensure tinydrm ili9341 can load
echo "[2/5] Configuring kernel modules..."
$SSH '
# Remove the old fbtft blacklist — we WANT the kernel driver now
rm -f /etc/modprobe.d/blacklist-fbtft.conf

# Remove old spidev autoload (kernel driver replaces spidev for this device)
rm -f /etc/modules-load.d/spidev.conf

# Ensure the ili9341 tinydrm module loads at boot
echo ili9341 > /etc/modules-load.d/ili9341.conf

# Configure fbcon to use the ILI9341 framebuffer
# fb0 = HDMI (sun4i-drm), fb1 = ILI9341 (tinydrm)
# Map virtual console 1 to fb1 (the SPI display)
mkdir -p /etc/modprobe.d
cat > /etc/modprobe.d/fbcon.conf <<EOF
options fbcon font=VGA8x8
EOF

echo "Modules configured"
'

# 3. Set up permissions for framebuffer access
echo "[3/5] Setting up permissions..."
$SSH '
# Ensure video group exists and user is in it
groupadd -f video
for u in streamlate app; do
    usermod -aG video "$u" 2>/dev/null || true
done

# udev rule: make /dev/fb* writable by video group
cat > /etc/udev/rules.d/99-framebuffer.rules <<EOF
SUBSYSTEM=="graphics", GROUP="video", MODE="0660"
EOF

# udev rule: make backlight writable by video group
cat > /etc/udev/rules.d/99-backlight.rules <<EOF
SUBSYSTEM=="backlight", ACTION=="add", \
  RUN+="/bin/chgrp video /sys/class/backlight/%k/brightness", \
  RUN+="/bin/chmod g+w /sys/class/backlight/%k/brightness"
EOF

# Remove old GPIO/SPI udev rules (no longer needed)
rm -f /etc/udev/rules.d/99-spidev.rules
rm -f /etc/udev/rules.d/99-gpio.rules

echo "Permissions configured"
'

# 4. Install boot splash service
echo "[4/6] Installing boot splash..."
$SCP "${SCRIPT_DIR}/deploy/ct-splash.sh" "root@${BOARD_IP}:/usr/local/bin/ct-splash.sh"
$SSH "chmod +x /usr/local/bin/ct-splash.sh"
$SCP "${SCRIPT_DIR}/deploy/ct-splash.service" "root@${BOARD_IP}:/etc/systemd/system/ct-splash.service"
$SSH "systemctl daemon-reload && systemctl enable ct-splash.service"
echo "Splash service installed"

# 5. Configure kernel command line for fbcon on SPI display
echo "[5/6] Configuring boot parameters..."
$SSH '
# Enable boot logo on the SPI display
sed -i "s/^bootlogo=.*/bootlogo=true/" /boot/armbianEnv.txt

# Add fbcon parameters via extraargs if not present
if ! grep -q "fbcon=" /boot/armbianEnv.txt; then
    if grep -q "^extraargs=" /boot/armbianEnv.txt; then
        sed -i "s|^extraargs=.*|& fbcon=map:1 fbcon=font:VGA8x8|" /boot/armbianEnv.txt
    else
        echo "extraargs=fbcon=map:1 fbcon=font:VGA8x8" >> /boot/armbianEnv.txt
    fi
fi

echo "Updated armbianEnv.txt:"
cat /boot/armbianEnv.txt
'

# 6. Reboot and verify
echo "[6/6] Rebooting..."
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

echo "--- Kernel version ---"
$SSH "uname -r"

echo "--- Framebuffer devices ---"
$SSH "ls -la /dev/fb* 2>/dev/null || echo 'FAIL: no /dev/fb*'"

echo "--- DRM devices ---"
$SSH "ls -la /dev/dri/* 2>/dev/null || echo '(no DRM devices)'"

echo "--- Loaded modules (ili/drm/fb) ---"
$SSH "lsmod | grep -iE 'ili|drm|fb' || echo '(none)'"

echo "--- Backlight ---"
$SSH "ls /sys/class/backlight/ 2>/dev/null || echo '(no backlight)'"

echo "--- dmesg: ili9341/drm/fb ---"
$SSH "dmesg | grep -iE 'ili|tinydrm|drm.*fb|fb[0-9]' | tail -15"

echo "--- Quick framebuffer test ---"
$SSH '
FB=$(ls /dev/fb* 2>/dev/null | grep -v fb0 | head -1)
if [ -z "$FB" ]; then
    FB=/dev/fb0
fi
if [ -e "$FB" ]; then
    # Write red pattern
    python3 -c "
import sys
sys.stdout.buffer.write(b\"\\x00\\xf8\" * (320 * 240))
" > "$FB" && echo "Red pattern written to $FB" || echo "Write to $FB FAILED"
else
    echo "FAIL: no framebuffer device found"
fi
'

echo ""
echo "=== Done ==="
echo "If you see a framebuffer device and red on the display, the kernel driver is working."
echo "Now deploy ct-client with framebuffer support."
