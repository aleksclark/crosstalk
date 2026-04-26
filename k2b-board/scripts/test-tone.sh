#!/usr/bin/env bash
set -euo pipefail
SSH="ssh -o ConnectTimeout=5 root@${1:?Usage: $0 <board-ip>}"

echo "--- Generate and play a test tone via PipeWire ---"
$SSH 'sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 bash -c '\''
# Generate 2s of 440Hz sine wave as raw S16LE
python3 -c "
import struct, math, sys
rate = 48000
freq = 440
duration = 2
samples = rate * duration
for i in range(samples):
    v = int(16000 * math.sin(2 * math.pi * freq * i / rate))
    sys.stdout.buffer.write(struct.pack(\"<h\", v))
" | pw-cat -p --format=s16 --rate=48000 --channels=1 --target=alsa_output.platform-5096000.codec.pro-output-0 -
echo "Tone playback complete"
'\'''
