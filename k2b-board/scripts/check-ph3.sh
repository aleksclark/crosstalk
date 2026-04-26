#!/usr/bin/env bash
set -euo pipefail
SSH="ssh -o ConnectTimeout=5 root@${1:?Usage: $0 <board-ip>}"
$SSH '
echo "--- PH3 (pin 227) pinmux ---"
cat /sys/kernel/debug/pinctrl/300b000.pinctrl/pinmux-pins | grep "pin 227 "

echo ""
echo "--- uart5 DT status ---"
cat /proc/device-tree/soc/serial@5001400/status 2>/dev/null; echo ""

echo ""
echo "--- Is uart5 actually in use? ---"
ls -la /dev/ttyS* 2>/dev/null
dmesg | grep -i "5001400\|uart5\|ttyS" | head -5

echo ""
echo "--- Try export GPIO 227 ---"
echo 227 > /sys/class/gpio/export 2>&1 || echo "export failed"
ls /sys/class/gpio/gpio227 2>/dev/null && echo "OK" || echo "not exported"
echo 227 > /sys/class/gpio/unexport 2>/dev/null || true
'
