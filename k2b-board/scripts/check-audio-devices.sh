#!/usr/bin/env bash
set -euo pipefail
SSH="ssh -o ConnectTimeout=5 root@${1:?Usage: $0 <board-ip>}"

echo "--- PipeWire sinks ---"
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 PULSE_RUNTIME_PATH=/run/user/999/pulse pactl list sinks 2>/dev/null | grep -E 'Name:|Description:|State:'"

echo ""
echo "--- PipeWire sources ---"
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 PULSE_RUNTIME_PATH=/run/user/999/pulse pactl list sources 2>/dev/null | grep -E 'Name:|Description:|State:'"

echo ""
echo "--- ALSA cards ---"
$SSH "cat /proc/asound/cards"

echo ""
echo "--- ALSA pcm devices ---"
$SSH "cat /proc/asound/pcm"

echo ""
echo "--- pw-record target test (audiocodec input, 1s) ---"
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 timeout 1 pw-record --format=s16 --rate=48000 --channels=1 --target=alsa_input.platform-audiocodec.stereo-fallback /tmp/test.wav 2>&1 || echo 'exit: \$?'"
$SSH "ls -la /tmp/test.wav 2>/dev/null; rm -f /tmp/test.wav"

echo ""
echo "--- pw-record target test (soc_ahub1_mach input) ---"
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 timeout 1 pw-record --format=s16 --rate=48000 --channels=1 --target=alsa_input.platform-soc_ahub1_mach.stereo-fallback /tmp/test2.wav 2>&1 || echo 'exit: \$?'"
$SSH "ls -la /tmp/test2.wav 2>/dev/null; rm -f /tmp/test2.wav"
