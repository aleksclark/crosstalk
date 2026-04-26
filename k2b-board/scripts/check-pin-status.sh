#!/usr/bin/env bash
set -euo pipefail
SSH="ssh -o ConnectTimeout=5 root@${1:?Usage: $0 <board-ip>}"
$SSH '
echo "--- gpiochip1 (300b000.pinctrl) GPIO 226 (PH2) and 228 (PH4) status ---"
cat /sys/kernel/debug/gpio 2>/dev/null | grep -E "gpio-22[4-9]|gpio-23[0-9]" || echo "(not found)"

echo ""
echo "--- Try manual GPIO export of PH2 (226) and PH4 (228) ---"
echo 226 > /sys/class/gpio/export 2>&1 || echo "export 226 failed"
echo 228 > /sys/class/gpio/export 2>&1 || echo "export 228 failed"
ls -la /sys/class/gpio/gpio226 2>/dev/null || echo "gpio226 not created"
ls -la /sys/class/gpio/gpio228 2>/dev/null || echo "gpio228 not created"
echo 226 > /sys/class/gpio/unexport 2>/dev/null || true
echo 228 > /sys/class/gpio/unexport 2>/dev/null || true

echo ""
echo "--- pinctrl pin status (PH range = 224-255) ---"
cat /sys/kernel/debug/pinctrl/300b000.pinctrl/pins 2>/dev/null | grep -E "pin 22[4-9]|pin 23[0-9]" || echo "(not found)"

echo ""
echo "--- pinctrl pinmux status ---"
cat /sys/kernel/debug/pinctrl/300b000.pinctrl/pinmux-pins 2>/dev/null | grep -E "pin 22[4-9]|pin 23[0-9]" || echo "(not found)"

echo ""
echo "--- SPI1 pinctrl reference ---"
od -An -tx1 /proc/device-tree/soc/spi@5011000/pinctrl-0 2>/dev/null || echo "(not found)"
'
