#!/usr/bin/env bash
set -euo pipefail
BOARD_IP="${1:?Usage: $0 <board-ip>}"
SSH="ssh -o ConnectTimeout=5 root@${BOARD_IP}"

$SSH '
echo "=== Force DRM modeset via fbcon ==="

# Check if fbcon is bound
cat /sys/class/vtconsole/vtcon*/name 2>/dev/null
echo ""

# Try binding fbcon to the ili9341 fb
for vt in /sys/class/vtconsole/vtcon*/; do
    name=$(cat ${vt}name 2>/dev/null)
    bind=$(cat ${vt}bind 2>/dev/null)
    echo "  $vt: name=$name bind=$bind"
done

echo ""
echo "=== Force enable via sysfs ==="
# Try enabling the connector
echo "enabled" > /sys/class/drm/card1-SPI-1/enabled 2>/dev/null || echo "cannot write enabled"

echo ""
echo "=== Try IOCTL dirty-fb via python ==="
python3 -c "
import fcntl, struct, os

fb = os.open(\"/dev/fb0\", os.O_RDWR)

# Write solid red (XRGB8888 = 0x00FF0000)
import mmap
size = 320 * 240 * 4
mm = mmap.mmap(fb, size)
pixel = struct.pack(\"<I\", 0x00FF0000)  # XRGB8888 red
mm.write(pixel * (320 * 240))

# Issue FBIOBLANK(0) = unblank
FBIOBLANK = 0x4611
try:
    fcntl.ioctl(fb, FBIOBLANK, 0)
    print(\"FBIOBLANK(0) = unblank sent\")
except Exception as e:
    print(f\"FBIOBLANK failed: {e}\")

# Issue FBIO_WAITFORVSYNC (may trigger flush)
FBIO_WAITFORVSYNC = 0x40044620
try:
    fcntl.ioctl(fb, FBIO_WAITFORVSYNC, struct.pack(\"I\", 0))
    print(\"WAITFORVSYNC sent\")
except Exception as e:
    print(f\"WAITFORVSYNC failed: {e}\")

# Issue FBIOPAN_DISPLAY to trigger dirty
FBIOPAN_DISPLAY = 0x4606
vinfo = bytearray(160)
try:
    fcntl.ioctl(fb, 0x4600, vinfo)  # FBIOGET_VSCREENINFO
    # Set yoffset=0 and pan
    fcntl.ioctl(fb, FBIOPAN_DISPLAY, vinfo)
    print(\"PAN_DISPLAY sent (triggers DRM flush)\")
except Exception as e:
    print(f\"PAN_DISPLAY failed: {e}\")

mm.close()
os.close(fb)
print(\"Done. Check display.\")
"

echo ""
echo "=== DRM connector status after ==="
cat /sys/class/drm/card1-SPI-1/enabled 2>/dev/null
cat /sys/class/drm/card1-SPI-1/status 2>/dev/null
'
