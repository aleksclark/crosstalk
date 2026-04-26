#!/usr/bin/env bash
# Find GPIO base for H618 port H.
# Usage: ./find-gpio.sh <board-ip>
set -euo pipefail
BOARD_IP="${1:?Usage: $0 <board-ip>}"
SSH="ssh -o ConnectTimeout=5 root@${BOARD_IP}"

echo "--- GPIO chips ---"
$SSH "
for chip in /sys/class/gpio/gpiochip*; do
    label=\$(cat \$chip/label 2>/dev/null)
    base=\$(cat \$chip/base 2>/dev/null)
    ngpio=\$(cat \$chip/ngpio 2>/dev/null)
    echo \"\$chip: label=\$label base=\$base ngpio=\$ngpio\"
done
"

echo ""
echo "--- /sys/kernel/debug/gpio (first 40 lines) ---"
$SSH "cat /sys/kernel/debug/gpio 2>/dev/null | head -40 || echo '(not available)'"

echo ""
echo "--- Try gpiodetect/gpioinfo ---"
$SSH "gpiodetect 2>/dev/null || echo '(gpiodetect not installed)'"
$SSH "gpioinfo 2>/dev/null | head -50 || echo '(gpioinfo not installed)'"
