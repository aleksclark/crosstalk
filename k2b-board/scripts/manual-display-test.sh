#!/usr/bin/env bash
set -euo pipefail
SSH="ssh -o ConnectTimeout=5 root@${1:?Usage: $0 <board-ip>}"

echo "=== Manual ILI9341 display test ==="
echo ""

$SSH '
systemctl stop app 2>/dev/null || true
rmmod fb_ili9341 2>/dev/null || true
rmmod fbtft 2>/dev/null || true
rmmod panel_ilitek_ili9341 2>/dev/null || true
rmmod ili9341 2>/dev/null || true
sleep 0.3

# Rebind spidev
modprobe spidev
sleep 0.2
if [ -e /sys/bus/spi/devices/spi0.1/driver ]; then
    echo spi0.1 > /sys/bus/spi/devices/spi0.1/driver/unbind 2>/dev/null || true
fi
echo spidev > /sys/bus/spi/devices/spi0.1/driver_override 2>/dev/null || true
echo spi0.1 > /sys/bus/spi/drivers/spidev/bind 2>/dev/null || true
sleep 0.3

SPIDEV=$(ls /dev/spidev* 2>/dev/null | head -1)
if [ -z "$SPIDEV" ]; then
    echo "ERROR: no spidev device"
    exit 1
fi
echo "Using $SPIDEV"

pip3 install spidev 2>/dev/null || apt-get install -y -qq python3-spidev 2>/dev/null || true

python3 <<PYEOF
import time, struct

# GPIO via sysfs
def gpio_export(n):
    try:
        open("/sys/class/gpio/export","w").write(str(n))
    except:
        pass
    
def gpio_dir(n, d):
    open(f"/sys/class/gpio/gpio{n}/direction","w").write(d)

def gpio_set(n, v):
    open(f"/sys/class/gpio/gpio{n}/value","w").write(str(v))

DC = 71    # PC7
RST = 76   # PC12

for g in [DC, RST]:
    try:
        open("/sys/class/gpio/unexport","w").write(str(g))
    except:
        pass
    gpio_export(g)
    gpio_dir(g, "out")

def reset():
    gpio_set(RST, 1)
    time.sleep(0.05)
    gpio_set(RST, 0)
    time.sleep(0.05)
    gpio_set(RST, 1)
    time.sleep(0.15)

# SPI setup
try:
    import spidev
    spi = spidev.SpiDev()
    spi.open(0, 1)
    spi.max_speed_hz = 32000000
    spi.mode = 0
    USE_SPIDEV = True
    print("Using spidev module")
except:
    USE_SPIDEV = False
    spifd = open("$SPIDEV", "wb", buffering=0)
    print("Using raw spidev fd (fallback)")

def spi_write(data):
    if USE_SPIDEV:
        # spidev.xfer2 limited to 4096, chunk it
        d = list(data) if isinstance(data, (bytes, bytearray)) else data
        for i in range(0, len(d), 4096):
            spi.xfer2(d[i:i+4096])
    else:
        spifd.write(bytes(data))

def cmd(c, *args):
    gpio_set(DC, 0)
    spi_write([c])
    if args:
        gpio_set(DC, 1)
        spi_write(list(args))

print("Resetting display...")
reset()

print("Sending init sequence...")
cmd(0x01)          # Software reset
time.sleep(0.15)

cmd(0xCB, 0x39, 0x2C, 0x00, 0x34, 0x02)  # Power Control A
cmd(0xCF, 0x00, 0xC1, 0x30)                # Power Control B
cmd(0xE8, 0x85, 0x00, 0x78)                # Timing A
cmd(0xEA, 0x00, 0x00)                       # Timing B
cmd(0xED, 0x64, 0x03, 0x12, 0x81)          # Power On Seq
cmd(0xF7, 0x20)                             # Pump ratio
cmd(0xC0, 0x23)                             # Power Control 1
cmd(0xC1, 0x10)                             # Power Control 2
cmd(0xC5, 0x3E, 0x28)                       # VCOM 1
cmd(0xC7, 0x86)                             # VCOM 2
cmd(0x36, 0x28)                             # MADCTL: landscape + BGR
cmd(0x3A, 0x55)                             # 16-bit color
cmd(0xB1, 0x00, 0x18)                       # Frame rate
cmd(0xB6, 0x08, 0x82, 0x27)                # Display Function
cmd(0xF2, 0x00)                             # Gamma disable
cmd(0x26, 0x01)                             # Gamma set
cmd(0x11)          # Sleep out
time.sleep(0.15)
cmd(0x29)          # Display ON
time.sleep(0.05)

print("Filling RED...")
cmd(0x2A, 0x00, 0x00, 0x01, 0x3F)  # Column 0-319
cmd(0x2B, 0x00, 0x00, 0x00, 0xEF)  # Row 0-239
gpio_set(DC, 0)
spi_write([0x2C])  # Memory write command
gpio_set(DC, 1)

# 320*240 pixels, RGB565 red = 0xF800 (big endian on wire)
red = bytes([0xF8, 0x00]) * (320 * 240)
spi_write(red)

print("RED fill sent!")
print()
print("If RED shows: fbtft driver init is the problem.")
print("If blank: hardware issue (bad display, wrong voltage, MOSI/CLK swapped).")
print()
print("Now trying GREEN...")
time.sleep(2)

cmd(0x2C)
gpio_set(DC, 1)
green = bytes([0x07, 0xE0]) * (320 * 240)
spi_write(green)
print("GREEN fill sent!")

PYEOF
'
