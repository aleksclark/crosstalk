#!/usr/bin/env bash
set -euo pipefail
SSH="ssh -o ConnectTimeout=5 root@${1:?Usage: $0 <board-ip>}"

echo "--- Test at different LINEOUT volumes ---"
for vol in 31 20 15 10 5; do
    echo ""
    echo "=== LINEOUT volume: $vol ==="
    $SSH "amixer -c 1 cset numid=2 $vol >/dev/null"
    $SSH 'sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 bash -c '\''
python3 -c "
import struct, math, sys
rate=48000; freq=440; dur=2
for i in range(rate*dur):
    v=int(16000*math.sin(2*math.pi*freq*i/rate))
    sys.stdout.buffer.write(struct.pack(\"<h\",v))
" | timeout 3 pw-cat -p --format=s16 --rate=48000 --channels=1 --target=alsa_output.platform-5096000.codec.pro-output-0 - 2>/dev/null
'\'''
    echo "  (did that sound cleaner?)"
    sleep 1
done

echo ""
echo "--- Restoring volume to 20 ---"
$SSH "amixer -c 1 cset numid=2 20 >/dev/null"
