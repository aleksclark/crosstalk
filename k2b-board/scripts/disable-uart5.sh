#!/usr/bin/env bash
# Disable uart5 in the DTB to free PH2/PH3 for GPIO use.
# Usage: ./disable-uart5.sh <board-ip>
set -euo pipefail
BOARD_IP="${1:?Usage: $0 <board-ip>}"
SSH="ssh -o ConnectTimeout=5 root@${BOARD_IP}"

echo "Disabling uart5 in DTB to free PH3 for backlight..."
$SSH '
set -e
DTB="/boot/dtb/allwinner/sun50i-h618-kickpi-k2b-v2.dtb"
BACKUP="${DTB}.bak"

# Always work from the original backup
[ -f "${BACKUP}" ] || cp "${DTB}" "${BACKUP}"

TMP=$(mktemp /tmp/dtb.XXXXXX.dts)
dtc -I dtb -O dts -o "$TMP" "${BACKUP}" 2>/dev/null

# Disable uart5 (serial@5001400) — use sed to flip status
sed -i "/serial@5001400/,/};/{s/status = \"okay\"/status = \"disabled\"/}" "$TMP"
echo "uart5 status set to disabled"

dtc -W no-unit_address_vs_reg -I dts -O dtb -o "${DTB}" "$TMP" 2>/dev/null || true
rm -f "$TMP"

# Verify
if dtc -I dtb -O dts "${DTB}" 2>/dev/null | grep -A5 "serial@5001400" | grep -q "disabled"; then
    echo "OK: uart5 disabled in DTB"
else
    echo "WARN: uart5 may not be disabled, restoring backup"
    cp "${BACKUP}" "${DTB}"
fi
'

echo "Rebooting..."
$SSH "reboot" 2>/dev/null || true
sleep 5

echo "Waiting for board..."
TRIES=0
while [ $TRIES -lt 18 ]; do
    if $SSH -o ConnectTimeout=3 "true" 2>/dev/null; then break; fi
    TRIES=$((TRIES + 1)); sleep 5
done

echo "--- PH3 (227) status ---"
$SSH "
cat /sys/kernel/debug/pinctrl/300b000.pinctrl/pinmux-pins | grep 'pin 227 '
echo 227 > /sys/class/gpio/export 2>/dev/null || true
echo out > /sys/class/gpio/gpio227/direction 2>/dev/null || true
echo 1 > /sys/class/gpio/gpio227/value 2>/dev/null && echo 'PH3 GPIO: OK - set HIGH' || echo 'PH3 GPIO: FAILED'
echo 227 > /sys/class/gpio/unexport 2>/dev/null || true
"
