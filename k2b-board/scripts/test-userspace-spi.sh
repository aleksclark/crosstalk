#!/usr/bin/env bash
# Revert to old userspace SPI and show red to confirm display hardware works.
set -euo pipefail
BOARD_IP="${1:?Usage: $0 <board-ip>}"
SSH="ssh -o ConnectTimeout=5 root@${BOARD_IP}"

echo "=== Reverting to userspace SPI to test display hardware ==="
$SSH '
systemctl stop app 2>/dev/null || true

# Unbind tinydrm
rmmod panel_ilitek_ili9341 2>/dev/null || true
rmmod ili9341 2>/dev/null || true
rmmod drm_mipi_dbi 2>/dev/null || true
sleep 0.5

# Rebind spidev
modprobe spidev
echo spidev > /sys/bus/spi/devices/spi0.1/driver_override 2>/dev/null || true
echo spi0.1 > /sys/bus/spi/drivers/spidev/bind 2>/dev/null || true
sleep 0.3

ls -la /dev/spidev0.1 2>/dev/null || { echo "FAIL: no spidev"; exit 1; }

for g in 71 76; do
    echo $g > /sys/class/gpio/export 2>/dev/null || true
    echo out > /sys/class/gpio/gpio$g/direction 2>/dev/null || true
done

python3 <<\PYEOF
import spidev, time

DC, RST = 71, 76
def gw(n, v):
    open(f"/sys/class/gpio/gpio{n}/value","w").write(str(v))

spi = spidev.SpiDev()
spi.open(0, 1)
spi.max_speed_hz = 32000000
spi.mode = 0

def cmd(c, *a):
    gw(DC, 0); spi.xfer2([c])
    if a: gw(DC, 1); spi.xfer2(list(a))

gw(RST, 1); time.sleep(0.05)
gw(RST, 0); time.sleep(0.05)
gw(RST, 1); time.sleep(0.15)

cmd(0x01); time.sleep(0.15)
cmd(0xCB,0x39,0x2C,0x00,0x34,0x02)
cmd(0xCF,0x00,0xC1,0x30)
cmd(0xE8,0x85,0x00,0x78)
cmd(0xEA,0x00,0x00)
cmd(0xED,0x64,0x03,0x12,0x81)
cmd(0xF7,0x20)
cmd(0xC0,0x23); cmd(0xC1,0x10)
cmd(0xC5,0x3E,0x28); cmd(0xC7,0x86)
cmd(0x36,0x28)
cmd(0x3A,0x55)
cmd(0xB1,0x00,0x18)
cmd(0xB6,0x08,0x82,0x27)
cmd(0xF2,0x00); cmd(0x26,0x01)
cmd(0x11); time.sleep(0.15)
cmd(0x29); time.sleep(0.05)

cmd(0x2A, 0x00, 0x00, 0x01, 0x3F)
cmd(0x2B, 0x00, 0x00, 0x00, 0xEF)
gw(DC, 0); spi.xfer2([0x2C])
gw(DC, 1)
red = [0xF8, 0x00] * 320
for y in range(240):
    spi.xfer2(red)

spi.close()
print("RED sent via userspace SPI. Display should show solid red.")
PYEOF
'

echo ""
echo "Check the display — it should show solid RED."
echo "If it does, the hardware is fine and the issue is with the tinydrm GPIO/init."
