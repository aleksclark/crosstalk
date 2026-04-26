#!/usr/bin/env bash
set -euo pipefail
SSH="ssh -o ConnectTimeout=5 root@${1:?Usage: $0 <board-ip>}"

echo "--- Audiocodec sink format ---"
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 PULSE_RUNTIME_PATH=/run/user/999/pulse pactl list sinks 2>/dev/null | grep -A10 'platform-5096000' | grep -E 'Name:|Format:|Sample|Channel'"

echo ""
echo "--- pw-dump audiocodec node ---"
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 pw-dump 2>/dev/null | python3 -c \"
import json,sys
for obj in json.load(sys.stdin):
    props = obj.get('info',{}).get('props',{})
    if '5096000' in props.get('node.name','') or '5096000' in props.get('object.path',''):
        print(json.dumps(obj, indent=2))
        break
\" 2>/dev/null | head -40 || echo '(pw-dump parse failed)'"

echo ""
echo "--- Test: play s32le tone to audiocodec ---"
$SSH 'sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 bash -c '\''
python3 -c "
import struct, math, sys
rate=48000; freq=440; dur=2
for i in range(rate*dur):
    v=int(500000000*math.sin(2*math.pi*freq*i/rate))
    sys.stdout.buffer.write(struct.pack(\"<i\",v))
" | timeout 3 pw-cat -p --format=s32 --rate=48000 --channels=1 --target=alsa_output.platform-5096000.codec.pro-output-0 - 2>&1
echo "s32 exit: $?"
'\'''

echo ""
echo "--- Test: play s16le tone (for comparison) ---"
$SSH 'sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 bash -c '\''
python3 -c "
import struct, math, sys
rate=48000; freq=880; dur=2
for i in range(rate*dur):
    v=int(16000*math.sin(2*math.pi*freq*i/rate))
    sys.stdout.buffer.write(struct.pack(\"<h\",v))
" | timeout 3 pw-cat -p --format=s16 --rate=48000 --channels=1 --target=alsa_output.platform-5096000.codec.pro-output-0 - 2>&1
echo "s16 exit: $?"
'\'''
