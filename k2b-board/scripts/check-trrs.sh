#!/usr/bin/env bash
set -euo pipefail
SSH="ssh -o ConnectTimeout=5 root@${1:?Usage: $0 <board-ip>}"

echo "--- PipeWire card profiles ---"
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 PULSE_RUNTIME_PATH=/run/user/999/pulse pactl list cards 2>/dev/null"

echo ""
echo "--- ALSA capture test on audiocodec (card 1) ---"
$SSH "timeout 1 arecord -D hw:1,0 -f S16_LE -r 48000 -c 1 /tmp/alsa-test.wav 2>&1 || echo 'arecord hw:1: exit \$?'"
$SSH "ls -la /tmp/alsa-test.wav 2>/dev/null; rm -f /tmp/alsa-test.wav"

echo ""
echo "--- ALSA capture test on ahub1 (card 2) ---"
$SSH "timeout 1 arecord -D hw:2,0 -f S16_LE -r 48000 -c 1 /tmp/alsa-test2.wav 2>&1 || echo 'arecord hw:2: exit \$?'"
$SSH "ls -la /tmp/alsa-test2.wav 2>/dev/null; rm -f /tmp/alsa-test2.wav"

echo ""
echo "--- Set ahub1 profile to duplex ---"
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 PULSE_RUNTIME_PATH=/run/user/999/pulse pactl list cards short 2>/dev/null"
