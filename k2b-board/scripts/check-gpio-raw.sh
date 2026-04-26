#!/usr/bin/env bash
set -euo pipefail
SSH="ssh -o ConnectTimeout=5 root@${1:?Usage: $0 <board-ip>}"
$SSH '
echo "--- pinctrl@300b000 properties ---"
ls /proc/device-tree/soc/pinctrl@300b000/ | head -30

echo ""
echo "--- #gpio-cells value ---"
od -An -tx1 /proc/device-tree/soc/pinctrl@300b000/#gpio-cells 2>/dev/null || echo "(not found)"

echo ""
echo "--- gpio-controller present? ---"
ls -la /proc/device-tree/soc/pinctrl@300b000/gpio-controller 2>/dev/null || echo "(not found)"

echo ""
echo "--- ili9341 dc-gpios raw ---"
od -An -tx1 /proc/device-tree/soc/spi@5011000/ili9341@1/dc-gpios 2>/dev/null || echo "(not found)"

echo ""
echo "--- ili9341 reset-gpios raw ---"
od -An -tx1 /proc/device-tree/soc/spi@5011000/ili9341@1/reset-gpios 2>/dev/null || echo "(not found)"

echo ""
echo "--- pinctrl phandle raw ---"
od -An -tx1 /proc/device-tree/soc/pinctrl@300b000/phandle 2>/dev/null || echo "(not found)"

echo ""
echo "--- working cd-gpios raw (for comparison) ---"
find /proc/device-tree -name "cd-gpios" 2>/dev/null | while read f; do
    echo "$f:"
    od -An -tx1 "$f" 2>/dev/null
done | head -10
'
