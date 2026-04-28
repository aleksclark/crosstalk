#!/usr/bin/env bash
# Investigate why the patched DTB isn't sticking on Armbian.
set -euo pipefail
BOARD_IP="${1:?Usage: $0 <board-ip>}"
SSH="ssh -o ConnectTimeout=5 root@${BOARD_IP}"

$SSH '
echo "=== DTB file on disk ==="
ls -la /boot/dtb/allwinner/sun50i-h618-kickpi-k2b-v2.dtb*

echo ""
echo "=== Decompile current DTB - grep ili/spidev ==="
dtc -I dtb -O dts /boot/dtb/allwinner/sun50i-h618-kickpi-k2b-v2.dtb 2>/dev/null | grep -E "spidev|ili9341"

echo ""
echo "=== armbian-release ==="
cat /etc/armbian-release 2>/dev/null | grep -i force || echo "(no FORCE settings)"

echo ""
echo "=== Boot script ==="
ls -la /boot/boot.cmd /boot/boot.scr 2>/dev/null || echo "(no boot.cmd/scr)"

echo ""
echo "=== Does boot.cmd reference DTB copy/restore? ==="
grep -i "dtb\|overlay\|fdt" /boot/boot.cmd 2>/dev/null | head -20 || echo "(no boot.cmd)"

echo ""
echo "=== Check for dpkg trigger ==="
ls /var/lib/dpkg/info/*dtb* 2>/dev/null | head -5 || echo "(no dtb packages)"

echo ""
echo "=== /boot directory ==="
ls /boot/*.dtb 2>/dev/null || echo "(no dtbs in /boot root)"

echo ""
echo "=== What U-Boot loads ==="
# Check environment for DTB path
grep -i fdtfile /boot/armbianEnv.txt
'
