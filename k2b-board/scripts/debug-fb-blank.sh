#!/usr/bin/env bash
set -euo pipefail
BOARD_IP="${1:?Usage: $0 <board-ip>}"
SSH="ssh -o ConnectTimeout=5 root@${BOARD_IP}"

$SSH '
echo "=== fb0 info ==="
cat /sys/class/graphics/fb0/name 2>/dev/null
cat /sys/class/graphics/fb0/virtual_size 2>/dev/null
cat /sys/class/graphics/fb0/bits_per_pixel 2>/dev/null
cat /sys/class/graphics/fb0/stride 2>/dev/null

echo ""
echo "=== dmesg ili9341 ==="
dmesg | grep -iE "ili|mipi|spi0|drm" | head -20

echo ""
echo "=== GPIO bank calculation ==="
echo "PC7: bank C=2, pin 7. Linux GPIO = (2*32)+7 = 71"
echo "PC12: bank C=2, pin 12. Linux GPIO = (2*32)+12 = 76"
echo ""
echo "BUT Allwinner pio banks in DT use different numbering."
echo "Check what the DT overlay from provision-display.sh used:"
echo "Old working userspace: DC=gpio71 RST=gpio76"
echo "DT patch: dc-gpios = <pio 0x02 0x07 0x00> reset-gpios = <pio 0x02 0x0c 0x00>"

echo ""
echo "=== Verify: gpio chip base ==="
for chip in /sys/class/gpio/gpiochip*/; do
    label=$(cat ${chip}label 2>/dev/null)
    base=$(cat ${chip}base 2>/dev/null)
    ngpio=$(cat ${chip}ngpio 2>/dev/null)
    echo "  $label: base=$base ngpio=$ngpio"
done

echo ""
echo "=== Try writing random data to fb0 ==="
dd if=/dev/urandom bs=153600 count=1 2>/dev/null > /dev/fb0
echo "Random data written to fb0"

echo ""
echo "=== Try writing solid red ==="
python3 -c "
import sys
sys.stdout.buffer.write(b\"\\x00\\xf8\" * (320 * 240))
" > /dev/fb0
echo "Red pattern written to fb0"

echo ""
echo "=== Check if tinydrm is doing dirty updates ==="
echo "The tinydrm driver only flushes to SPI on dirty-rect calls."
echo "Writing to mmap fb0 alone may not trigger a flush."
echo ""
echo "=== Try fbset ==="
fbset -fb /dev/fb0 2>/dev/null || echo "fbset not available"

echo ""
echo "=== Check DRM card1 (ili9341) ==="
ls -la /dev/dri/card1
cat /sys/class/drm/card1-*/status 2>/dev/null || echo "(no connector status)"
cat /sys/class/drm/card1-*/enabled 2>/dev/null || echo "(no enabled)"
cat /sys/class/drm/card1-*/modes 2>/dev/null || echo "(no modes)"
'
