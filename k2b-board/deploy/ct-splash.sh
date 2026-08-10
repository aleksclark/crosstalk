#!/bin/bash
set -eu

FB=""
for dev in /dev/fb1 /dev/fb0; do
    [ -e "$dev" ] && FB="$dev" && break
done
[ -z "$FB" ] && exit 0

SPLASH=/usr/local/share/crosstalk/ct-splash.png
[ -r "$SPLASH" ] || exit 0

python3 - "$FB" "$SPLASH" <<'PY'
import fcntl
import mmap
import os
import struct
import sys
import zlib

fb_path, png_path = sys.argv[1:]
width, height, bytes_per_pixel = 320, 240, 4

def read_png(path):
    data = open(path, "rb").read()
    if data[:8] != b"\x89PNG\r\n\x1a\n":
        raise ValueError("invalid PNG")
    offset = 8
    compressed = bytearray()
    png_width = png_height = color_type = None
    while offset < len(data):
        length = struct.unpack(">I", data[offset:offset + 4])[0]
        kind = data[offset + 4:offset + 8]
        payload = data[offset + 8:offset + 8 + length]
        offset += 12 + length
        if kind == b"IHDR":
            png_width, png_height, bit_depth, color_type, compression, filtering, interlace = struct.unpack(">IIBBBBB", payload)
            if bit_depth != 8 or color_type not in (2, 6) or compression or filtering or interlace:
                raise ValueError("unsupported PNG format")
        elif kind == b"IDAT":
            compressed.extend(payload)
        elif kind == b"IEND":
            break
    channels = 3 if color_type == 2 else 4
    stride = png_width * channels
    raw = zlib.decompress(compressed)
    rows = []
    previous = bytearray(stride)
    position = 0
    for _ in range(png_height):
        filter_type = raw[position]
        position += 1
        row = bytearray(raw[position:position + stride])
        position += stride
        for index in range(stride):
            left = row[index - channels] if index >= channels else 0
            up = previous[index]
            upper_left = previous[index - channels] if index >= channels else 0
            if filter_type == 1:
                row[index] = (row[index] + left) & 255
            elif filter_type == 2:
                row[index] = (row[index] + up) & 255
            elif filter_type == 3:
                row[index] = (row[index] + ((left + up) // 2)) & 255
            elif filter_type == 4:
                predictor = left + up - upper_left
                distances = (abs(predictor - left), abs(predictor - up), abs(predictor - upper_left))
                row[index] = (row[index] + (left, up, upper_left)[distances.index(min(distances))]) & 255
            elif filter_type != 0:
                raise ValueError("unsupported PNG filter")
        rows.append(row)
        previous = row
    return png_width, png_height, channels, rows

png_width, png_height, channels, rows = read_png(png_path)
if (png_width, png_height) != (width, height):
    raise ValueError("splash PNG must be 320x240")

buffer = bytearray(width * height * bytes_per_pixel)
for y, row in enumerate(rows):
    for x in range(width):
        source = x * channels
        red, green, blue = row[source:source + 3]
        destination = (y * width + x) * bytes_per_pixel
        buffer[destination:destination + bytes_per_pixel] = struct.pack("<I", (red << 16) | (green << 8) | blue)

fd = os.open(fb_path, os.O_RDWR)
framebuffer = mmap.mmap(fd, len(buffer))
framebuffer.write(buffer)
try:
    fcntl.ioctl(fd, 0x4611, 0)
except OSError:
    pass
try:
    screen_info = bytearray(160)
    fcntl.ioctl(fd, 0x4600, screen_info)
    fcntl.ioctl(fd, 0x4606, screen_info)
except OSError:
    pass
framebuffer.close()
os.close(fd)
PY

for bl in /sys/class/backlight/*/brightness; do
    [ -f "$bl" ] || continue
    max=$(cat "$(dirname "$bl")/max_brightness")
    echo "$max" > "$bl" 2>/dev/null || true
done
