#!/usr/bin/env bash
# Configure K2B audio for TRRS jack (audiocodec card).
# Usage: ./setup-trrs-audio.sh <board-ip>
set -euo pipefail
BOARD_IP="${1:?Usage: $0 <board-ip>}"
SSH="ssh -o ConnectTimeout=5 root@${BOARD_IP}"

echo "=== Setting up TRRS audio on K2B ==="

$SSH '
export XDG_RUNTIME_DIR=/run/user/999
export PULSE_RUNTIME_PATH=/run/user/999/pulse

# Activate audiocodec card with pro-audio profile (TRRS jack)
sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 pactl set-card-profile alsa_card.platform-5096000.codec pro-audio
echo "Activated audiocodec card (pro-audio)"

# Check what appeared
echo ""
echo "--- Sinks ---"
sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 pactl list sinks short
echo ""
echo "--- Sources ---"
sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 pactl list sources short
echo ""
echo "--- Quick capture test from audiocodec ---"
sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 timeout 1 pw-record --target=alsa_output.platform-5096000.codec.pro-output-0 /dev/null 2>&1 || true
sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 timeout 1 pw-record /dev/null 2>&1 | head -3 || true

echo ""
echo "--- Full source/sink names ---"
sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 pactl list sinks 2>/dev/null | grep -E "Name:|Description:"
echo "---"
sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 pactl list sources 2>/dev/null | grep -E "Name:|Description:"
'
