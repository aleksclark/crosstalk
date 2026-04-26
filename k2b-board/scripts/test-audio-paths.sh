#!/usr/bin/env bash
set -euo pipefail
SSH="ssh -o ConnectTimeout=5 root@${1:?Usage: $0 <board-ip>}"

TONE='python3 -c "
import struct,math,sys
r=48000;f=440;d=2
for i in range(r*d):
    v=int(16000*math.sin(2*math.pi*f*i/r))
    sys.stdout.buffer.write(struct.pack(\"<h\",v))
"'

echo "=== Test 1: pro-audio profile (current) ==="
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 PULSE_RUNTIME_PATH=/run/user/999/pulse pactl list cards short"
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 PULSE_RUNTIME_PATH=/run/user/999/pulse pactl list sinks short"
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 bash -c '$TONE | timeout 3 pw-cat -p --format=s16 --rate=48000 --channels=1 - 2>/dev/null'" || true
echo "(listen...)"
sleep 2

echo ""
echo "=== Test 2: switch audiocodec to off, use ahub1 for output ==="
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 PULSE_RUNTIME_PATH=/run/user/999/pulse pactl set-card-profile alsa_card.platform-5096000.codec off"
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 PULSE_RUNTIME_PATH=/run/user/999/pulse pactl set-card-profile alsa_card.platform-soc_ahub1_mach output:stereo-fallback 2>/dev/null || true"
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 PULSE_RUNTIME_PATH=/run/user/999/pulse pactl set-default-sink alsa_output.platform-soc_ahub1_mach.stereo-fallback 2>/dev/null || true"
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 PULSE_RUNTIME_PATH=/run/user/999/pulse pactl list sinks short"
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 bash -c '$TONE | timeout 3 pw-cat -p --format=s16 --rate=48000 --channels=1 --target=alsa_output.platform-soc_ahub1_mach.stereo-fallback - 2>/dev/null'" || true
echo "(listen - ahub1 output)"
sleep 2

echo ""
echo "=== Test 3: use ALSA directly (bypass PipeWire) ==="
$SSH "amixer -c 1 cset numid=4 on; amixer -c 1 cset numid=6 on; amixer -c 1 cset numid=2 20" >/dev/null 2>&1
$SSH "$TONE | timeout 3 aplay -D plughw:1,0 -f S16_LE -r 48000 -c 1 - 2>/dev/null" || true
echo "(listen - direct ALSA hw:1)"
sleep 2

echo ""
echo "=== Test 4: ALSA with 2 channels ==="
$SSH 'python3 -c "
import struct,math,sys
r=48000;f=440;d=2
for i in range(r*d):
    v=int(16000*math.sin(2*math.pi*f*i/r))
    sys.stdout.buffer.write(struct.pack(\"<hh\",v,v))
" | timeout 3 aplay -D plughw:1,0 -f S16_LE -r 48000 -c 2 - 2>/dev/null' || true
echo "(listen - ALSA stereo)"

echo ""
echo "=== Restoring pro-audio ==="
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 PULSE_RUNTIME_PATH=/run/user/999/pulse pactl set-card-profile alsa_card.platform-5096000.codec pro-audio 2>/dev/null || true"
