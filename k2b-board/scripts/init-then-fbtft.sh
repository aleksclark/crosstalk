#!/usr/bin/env bash
# Init the display manually via spidev, then reload fbtft on top.
# This proves fbtft works if the display is already initialized.
set -euo pipefail
SSH="ssh -o ConnectTimeout=5 root@${1:?Usage: $0 <board-ip>}"

$SSH '
systemctl stop app 2>/dev/null || true
rmmod fb_ili9341 2>/dev/null || true
rmmod fbtft 2>/dev/null || true
rmmod panel_ilitek_ili9341 2>/dev/null || true
rmmod ili9341 2>/dev/null || true
sleep 0.3

# Bind spidev
modprobe spidev
sleep 0.2
if [ -e /sys/bus/spi/devices/spi0.1/driver ]; then
    echo spi0.1 > /sys/bus/spi/devices/spi0.1/driver/unbind 2>/dev/null || true
fi
echo spidev > /sys/bus/spi/devices/spi0.1/driver_override 2>/dev/null || true
echo spi0.1 > /sys/bus/spi/drivers/spidev/bind 2>/dev/null || true
sleep 0.3

echo "=== Phase 1: Init display via spidev ==="
python3 <<PYEOF
import spidev, time

DC = 71
RST = 76

def gpio_w(n, v):
    open(f"/sys/class/gpio/gpio{n}/value","w").write(str(v))

for g in [DC, RST]:
    try: open("/sys/class/gpio/unexport","w").write(str(g))
    except: pass
    try: open("/sys/class/gpio/export","w").write(str(g))
    except: pass
    open(f"/sys/class/gpio/gpio{g}/direction","w").write("out")

spi = spidev.SpiDev()
spi.open(0, 1)
spi.max_speed_hz = 32000000
spi.mode = 0

def cmd(c, *a):
    gpio_w(DC, 0); spi.xfer2([c])
    if a: gpio_w(DC, 1); spi.xfer2(list(a))

gpio_w(RST, 1); time.sleep(0.05)
gpio_w(RST, 0); time.sleep(0.05)
gpio_w(RST, 1); time.sleep(0.15)

cmd(0x01); time.sleep(0.15)
cmd(0xCB,0x39,0x2C,0x00,0x34,0x02)
cmd(0xCF,0x00,0xC1,0x30)
cmd(0xE8,0x85,0x00,0x78)
cmd(0xEA,0x00,0x00)
cmd(0xED,0x64,0x03,0x12,0x81)
cmd(0xF7,0x20)
cmd(0xC0,0x23)
cmd(0xC1,0x10)
cmd(0xC5,0x3E,0x28)
cmd(0xC7,0x86)
cmd(0x36,0x28)
cmd(0x3A,0x55)
cmd(0xB1,0x00,0x18)
cmd(0xB6,0x08,0x82,0x27)
cmd(0xF2,0x00)
cmd(0x26,0x01)
cmd(0x11); time.sleep(0.15)
cmd(0x29); time.sleep(0.05)
spi.close()
print("Display initialized via spidev")
PYEOF

# Unexport GPIOs so fbtft can claim them
echo 71 > /sys/class/gpio/unexport 2>/dev/null || true
echo 76 > /sys/class/gpio/unexport 2>/dev/null || true

# Unbind spidev so fbtft can claim the SPI device
echo spi0.1 > /sys/bus/spi/drivers/spidev/unbind 2>/dev/null || true
echo "" > /sys/bus/spi/devices/spi0.1/driver_override 2>/dev/null || true

echo "=== Phase 2: Load fbtft (display already initialized) ==="
modprobe fbtft
modprobe fb_ili9341
sleep 1

echo "=== Phase 3: Write test pattern ==="
python3 -c "
import sys
sys.stdout.buffer.write(b\"\\x00\\xf8\" * (320*240))
" > /dev/fb0

echo "RED pattern written to fb0."
dmesg | grep -i fbtft | tail -5
echo ""
echo "=== Phase 4: Start ct-client ==="
systemctl start app
sleep 3
journalctl -u app --no-pager -l -n 5 | grep -iE "display|fb|error"
echo "Check the display now!"
'
