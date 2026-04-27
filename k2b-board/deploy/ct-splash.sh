#!/bin/bash
# Write splash image to the ILI9341 framebuffer at boot.
# Called by ct-splash.service very early in the boot process.
#
# The tinydrm fbdev emulation exposes a 32bpp XRGB8888 buffer.
# After writing pixels, we issue FBIOPAN_DISPLAY to trigger the
# DRM dirty-rect flush that sends data over SPI to the panel.
set -eu

FB=""
for dev in /dev/fb1 /dev/fb0; do
    [ -e "$dev" ] && FB="$dev" && break
done
[ -z "$FB" ] && exit 0

W=320
H=240

python3 -c "
import struct, sys, fcntl, os, mmap

W, H = $W, $H
BPP = 4  # XRGB8888

# Colors in XRGB8888 (little-endian uint32: 0x00RRGGBB)
bg_val = 0x000A0A14  # dark blue-grey
fg_val = 0x00A0B4DC  # light blue

bg = struct.pack('<I', bg_val)
fg = struct.pack('<I', fg_val)

font = {
 'C': ['01110','10000','10000','10000','10000','10000','01110'],
 'R': ['11110','10001','10001','11110','10100','10010','10001'],
 'O': ['01110','10001','10001','10001','10001','10001','01110'],
 'S': ['01110','10000','10000','01110','00001','00001','11110'],
 'T': ['11111','00100','00100','00100','00100','00100','00100'],
 'A': ['01110','10001','10001','11111','10001','10001','10001'],
 'L': ['10000','10000','10000','10000','10000','10000','11111'],
 'K': ['10001','10010','10100','11000','10100','10010','10001'],
}

text = 'CROSSTALK'
scale = 3
gw, gh = 5 * scale, 7 * scale
gap = scale
tw = len(text) * (gw + gap) - gap
sx = (W - tw) // 2
sy = (H - gh) // 2

buf = bytearray(bg * W * H)

for ci, ch in enumerate(text):
    glyph = font.get(ch)
    if not glyph: continue
    ox = sx + ci * (gw + gap)
    for row, bits in enumerate(glyph):
        for col, b in enumerate(bits):
            if b == '1':
                for dy in range(scale):
                    for dx in range(scale):
                        px = ox + col * scale + dx
                        py = sy + row * scale + dy
                        if 0 <= px < W and 0 <= py < H:
                            off = (py * W + px) * BPP
                            buf[off:off+BPP] = fg

# Open fb, mmap, write pixels
fd = os.open('$FB', os.O_RDWR)
mm = mmap.mmap(fd, len(buf))
mm.write(buf)

# Unblank
FBIOBLANK = 0x4611
try: fcntl.ioctl(fd, FBIOBLANK, 0)
except: pass

# Trigger DRM dirty flush via PAN_DISPLAY
FBIOGET_VSCREENINFO = 0x4600
FBIOPAN_DISPLAY = 0x4606
vinfo = bytearray(160)
try:
    fcntl.ioctl(fd, FBIOGET_VSCREENINFO, vinfo)
    fcntl.ioctl(fd, FBIOPAN_DISPLAY, vinfo)
except: pass

mm.close()
os.close(fd)
"

# Turn on backlight
for bl in /sys/class/backlight/*/brightness; do
    [ -f "$bl" ] || continue
    max=$(cat "$(dirname "$bl")/max_brightness")
    echo "$max" > "$bl" 2>/dev/null || true
done
