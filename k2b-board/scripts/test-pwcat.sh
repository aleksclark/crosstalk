#!/usr/bin/env bash
set -euo pipefail
SSH="ssh -o ConnectTimeout=5 root@${1:?Usage: $0 <board-ip>}"

echo "--- What killed pw-cat? ---"
$SSH "journalctl -u app --no-pager -l 2>/dev/null | grep -iE 'pw-cat|playback.*error|playback.*fail|sink.*error' | tail -10"

echo ""
echo "--- Test pw-cat to audiocodec directly ---"
$SSH 'sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 bash -c '\''
echo "Test 1: pw-cat -p with --target (pro-audio name)"
python3 -c "
import struct, math, sys
rate=48000; freq=440; dur=2
for i in range(rate*dur):
    v=int(16000*math.sin(2*math.pi*freq*i/rate))
    sys.stdout.buffer.write(struct.pack(\"<h\",v))
" | timeout 3 pw-cat -p --format=s16 --rate=48000 --channels=1 --target=alsa_output.platform-5096000.codec.pro-output-0 - 2>&1
echo "exit: $?"

echo ""
echo "Test 2: pw-cat -p without --target (default sink)"
python3 -c "
import struct, math, sys
rate=48000; freq=880; dur=2
for i in range(rate*dur):
    v=int(16000*math.sin(2*math.pi*freq*i/rate))
    sys.stdout.buffer.write(struct.pack(\"<h\",v))
" | timeout 3 pw-cat -p --format=s16 --rate=48000 --channels=1 - 2>&1
echo "exit: $?"

echo ""
echo "Test 3: pw-play with target"
python3 -c "
import struct, math, sys
rate=48000; freq=660; dur=2
for i in range(rate*dur):
    v=int(16000*math.sin(2*math.pi*freq*i/rate))
    sys.stdout.buffer.write(struct.pack(\"<h\",v))
" | timeout 3 pw-cat -p --format=s16 --rate=48000 --channels=1 --target=75 - 2>&1
echo "exit: $?"
'\'''

echo ""
echo "--- Default sink ---"
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 PULSE_RUNTIME_PATH=/run/user/999/pulse pactl get-default-sink"

echo ""
echo "--- Sink node IDs ---"
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 pw-cli list-objects Node 2>/dev/null | grep -B1 -A2 'Audio/Sink' | head -20"
