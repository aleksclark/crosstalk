#!/usr/bin/env bash
set -euo pipefail
SSH="ssh -o ConnectTimeout=5 root@${1:?Usage: $0 <board-ip>}"

echo "--- Set ahub1 to pro-audio (duplex) ---"
$SSH 'sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 pactl set-card-profile alsa_card.platform-soc_ahub1_mach pro-audio'
sleep 1

echo "--- Sources ---"
$SSH 'sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 pactl list sources short'

echo ""
echo "--- Sinks ---"
$SSH 'sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 pactl list sinks short'

echo ""
echo "--- ALSA capture test on ahub1 (card 2, mono) ---"
$SSH "timeout 1 arecord -D plughw:2,0 -f S16_LE -r 48000 -c 1 /tmp/ahub-cap.wav 2>&1; ls -la /tmp/ahub-cap.wav 2>/dev/null; rm -f /tmp/ahub-cap.wav"
