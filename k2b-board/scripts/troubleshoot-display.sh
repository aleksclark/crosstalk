#!/usr/bin/env bash
# Diagnose SPI display setup on K2B.
# Usage: ./troubleshoot-display.sh <board-ip>
set -euo pipefail

BOARD_IP="${1:?Usage: $0 <board-ip>}"
SSH="ssh -o ConnectTimeout=5 root@${BOARD_IP}"

echo "=== K2B Display Troubleshooting ==="
echo "Board: ${BOARD_IP}"
echo ""

echo "--- 1. armbianEnv.txt ---"
$SSH "cat /boot/armbianEnv.txt"
echo ""

echo "--- 2. Overlay files ---"
$SSH "ls -la /boot/overlay-user/ 2>/dev/null || echo '(no /boot/overlay-user)'"
echo ""

echo "--- 3. Available kernel overlays matching spi ---"
$SSH "ls /boot/dtb/allwinner/overlay/ 2>/dev/null | grep -i spi || echo '(none found)'"
echo ""

echo "--- 4. Loaded kernel modules (spi/fb/ili/fbtft) ---"
$SSH "lsmod | grep -iE 'spi|fb|ili|fbtft' || echo '(no matching modules loaded)'"
echo ""

echo "--- 5. Available kernel modules (fbtft/ili) ---"
$SSH "find /lib/modules/\$(uname -r) -name '*fbtft*' -o -name '*ili*' -o -name '*fb_ili*' 2>/dev/null || echo '(none found)'"
echo ""

echo "--- 6. SPI devices ---"
$SSH "ls -la /dev/spidev* 2>/dev/null || echo '(no /dev/spidev*)'"
echo ""

echo "--- 7. Framebuffer devices ---"
$SSH "ls -la /dev/fb* 2>/dev/null || echo '(no /dev/fb*)'"
echo ""

echo "--- 8. dmesg: SPI ---"
$SSH "dmesg | grep -iE 'spi' | tail -20 || echo '(nothing)'"
echo ""

echo "--- 9. dmesg: framebuffer/panel/ili/fbtft ---"
$SSH "dmesg | grep -iE 'ili|fbtft|panel|fb[0-9]|framebuffer' | tail -20 || echo '(nothing)'"
echo ""

echo "--- 10. dmesg: overlay ---"
$SSH "dmesg | grep -iE 'overlay|dtb' | tail -20 || echo '(nothing)'"
echo ""

echo "--- 11. Device tree SPI1 node ---"
$SSH "
if [ -d /proc/device-tree/soc/spi@5011000 ]; then
    echo 'SPI1 node exists at /proc/device-tree/soc/spi@5011000'
    cat /proc/device-tree/soc/spi@5011000/status 2>/dev/null && echo '' || echo '(no status)'
    ls /proc/device-tree/soc/spi@5011000/ 2>/dev/null
elif [ -d /proc/device-tree/soc@3000000/spi@5011000 ]; then
    echo 'SPI1 node exists at /proc/device-tree/soc@3000000/spi@5011000'
    cat /proc/device-tree/soc@3000000/spi@5011000/status 2>/dev/null && echo '' || echo '(no status)'
    ls /proc/device-tree/soc@3000000/spi@5011000/ 2>/dev/null
else
    echo 'SPI1 node not found in device tree. Searching...'
    find /proc/device-tree -name '*spi*' -maxdepth 4 2>/dev/null | head -20 || echo '(nothing)'
fi
"
echo ""

echo "--- 12. PWM status ---"
$SSH "
ls /sys/class/pwm/ 2>/dev/null || echo '(no /sys/class/pwm)'
ls /sys/class/backlight/ 2>/dev/null || echo '(no /sys/class/backlight)'
"
echo ""

echo "--- 13. Kernel version & config check ---"
$SSH "
uname -r
zcat /proc/config.gz 2>/dev/null | grep -iE 'CONFIG_FB_TFT|CONFIG_SPI_SUN|CONFIG_DRM_PANEL_ILITEK|CONFIG_FB_ILI' || \
  grep -riE 'CONFIG_FB_TFT|CONFIG_SPI_SUN|CONFIG_DRM_PANEL_ILITEK|CONFIG_FB_ILI' /boot/config-\$(uname -r) 2>/dev/null || \
  echo '(kernel config not available)'
"
echo ""

echo "--- 14. Try loading fbtft manually ---"
$SSH "
modprobe fbtft 2>&1 || echo 'fbtft module not available'
modprobe fbtft_device 2>&1 || echo 'fbtft_device module not available'
modprobe fb_ili9341 2>&1 || echo 'fb_ili9341 module not available'
modprobe panel-ilitek-ili9341 2>&1 || echo 'panel-ilitek-ili9341 module not available'
"
echo ""

echo "=== Done ==="
