#!/usr/bin/env bash
set -euo pipefail
SSH="ssh -o ConnectTimeout=5 root@${1:?Usage: $0 <board-ip>}"

echo "--- GPIO status after fbtft probe ---"
$SSH "cat /sys/kernel/debug/gpio 2>/dev/null | grep -E 'gpio-(71|76|PC)' || echo '(no matches)'"

echo ""
echo "--- All claimed GPIOs ---"
$SSH "cat /sys/kernel/debug/gpio 2>/dev/null | grep -v 'unused' || echo '(none)'"

echo ""
echo "--- pinctrl pinmux for PC7 and PC12 ---"
$SSH "cat /sys/kernel/debug/pinctrl/300b000.pinctrl/pinmux-pins 2>/dev/null | grep -E 'pin (71|76) '"

echo ""
echo "--- fbtft debug: enable verbose ---"
$SSH "
echo 7 > /proc/sys/kernel/printk
rmmod fb_ili9341 2>/dev/null || true
rmmod fbtft 2>/dev/null || true
sleep 0.3
modprobe fbtft debug=7 2>/dev/null || modprobe fbtft
modprobe fb_ili9341
sleep 1
dmesg | tail -30
"
