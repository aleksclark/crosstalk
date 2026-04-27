#!/usr/bin/env bash
# Patch the K2B DTB to replace spidev@1 with ili9341@1 under spi@5011000.
set -euo pipefail
BOARD_IP="${1:?Usage: $0 <board-ip>}"
SSH="ssh -o ConnectTimeout=5 root@${BOARD_IP}"
SCP="scp -o ConnectTimeout=5"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "=== Patching DTB for ILI9341 ==="

# Upload the Python patcher
$SCP "${SCRIPT_DIR}/patch-dts.py" "root@${BOARD_IP}:/tmp/patch-dts.py"

$SSH 'set -e
DTB="/boot/dtb/allwinner/sun50i-h618-kickpi-k2b-v2.dtb"
ORIG="${DTB}.orig"

# Restore original before patching
if [ -f "${ORIG}" ]; then cp "${ORIG}" "${DTB}"; fi
if [ ! -f "${ORIG}" ]; then cp "${DTB}" "${ORIG}"; fi

# Decompile
dtc -I dtb -O dts -o /tmp/k2b.dts "${ORIG}" 2>/dev/null

echo "--- Before ---"
grep -n -A 5 "spidev@1" /tmp/k2b.dts

# Patch
python3 /tmp/patch-dts.py

echo "--- After ---"
grep -n -A 10 "ili9341@1" /tmp/k2b-patched.dts

# Recompile
dtc -I dts -O dtb -o "${DTB}" /tmp/k2b-patched.dts 2>/dev/null
echo "DTB written"

# Verify
echo "--- Verify ---"
dtc -I dtb -O dts "${DTB}" 2>/dev/null | grep -A 10 "ili9341"

# Ensure module loads
echo ili9341 > /etc/modules-load.d/ili9341.conf
rm -f /etc/modprobe.d/blacklist-fbtft.conf
rm -f /etc/modules-load.d/spidev.conf

echo "Rebooting..."
reboot
'

echo "Waiting for board to come back..."
sleep 8
TRIES=0
while [ $TRIES -lt 18 ]; do
    if $SSH -o ConnectTimeout=3 "true" 2>/dev/null; then break; fi
    TRIES=$((TRIES + 1))
    sleep 5
done

echo ""
echo "=== Post-reboot ==="
bash "${SCRIPT_DIR}/check-display.sh" "${BOARD_IP}"
