#!/usr/bin/env bash
set -euo pipefail
SSH="ssh -o ConnectTimeout=5 root@${1:?Usage: $0 <board-ip>}"
$SSH '
echo "--- PC pin mux status (port C = pins 64-95) ---"
cat /sys/kernel/debug/pinctrl/300b000.pinctrl/pinmux-pins 2>/dev/null | grep -E "pin (6[4-9]|[78][0-9]|9[0-5]) " || echo "(none claimed)"

echo ""
echo "--- PI pin mux (port I = pins 256-287) ---"
cat /sys/kernel/debug/pinctrl/300b000.pinctrl/pinmux-pins 2>/dev/null | grep -E "pin (25[6-9]|2[67][0-9]|28[0-7]) " || echo "(none claimed)"

echo ""  
echo "--- Try export PC7 (71) and PC12 (76) ---"
echo 71 > /sys/class/gpio/export 2>&1; echo "PC7 (71): $?"
echo 76 > /sys/class/gpio/export 2>&1; echo "PC12 (76): $?"
ls /sys/class/gpio/gpio71 2>/dev/null && echo "PC7 OK" || echo "PC7 FAIL"
ls /sys/class/gpio/gpio76 2>/dev/null && echo "PC12 OK" || echo "PC12 FAIL"
echo 71 > /sys/class/gpio/unexport 2>/dev/null || true
echo 76 > /sys/class/gpio/unexport 2>/dev/null || true

echo ""
echo "--- Full list of free PH/PC/PG pins ---"
cat /sys/kernel/debug/pinctrl/300b000.pinctrl/pinmux-pins 2>/dev/null | grep "UNCLAIMED" | grep -E "P[CGH]" || echo "(none)"
'
