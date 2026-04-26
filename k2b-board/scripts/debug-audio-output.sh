#!/usr/bin/env bash
set -euo pipefail
SSH="ssh -o ConnectTimeout=5 root@${1:?Usage: $0 <board-ip>}"

echo "--- App logs (last 30) ---"
$SSH "journalctl -u app --no-pager -l -n 30 2>/dev/null"

echo ""
echo "--- PipeWire node status ---"
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 pw-cli list-objects Node 2>/dev/null | grep -E 'node.name|media.class|state' | head -30"

echo ""
echo "--- PipeWire links ---"
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 pw-link -l 2>/dev/null | head -30 || echo 'pw-link not available'"

echo ""
echo "--- Active sinks and their state ---"
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 PULSE_RUNTIME_PATH=/run/user/999/pulse pactl list sinks 2>/dev/null | grep -E 'Name:|State:|Volume:|Mute:'"

echo ""
echo "--- Active sources and their state ---"
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 PULSE_RUNTIME_PATH=/run/user/999/pulse pactl list sources 2>/dev/null | grep -E 'Name:|State:|Volume:|Mute:'"

echo ""
echo "--- Default sink ---"
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 PULSE_RUNTIME_PATH=/run/user/999/pulse pactl get-default-sink 2>/dev/null"

echo ""
echo "--- ALSA audiocodec mixer (card 1) ---"
$SSH "amixer -c 1 contents 2>/dev/null | head -30 || echo 'no card 1 mixer'"

echo ""
echo "--- pw-top ---"
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 timeout 2 pw-top -b 2>/dev/null | head -20"

echo ""
echo "--- Running processes ---"
$SSH "ps aux | grep -E 'ffmpeg|pw-cat|pw-record|ct-client' | grep -v grep"
