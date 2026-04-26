#!/usr/bin/env bash
set -euo pipefail
SSH="ssh -o ConnectTimeout=5 root@${1:?Usage: $0 <board-ip>}"

echo "--- PipeWire nodes ---"
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 pw-cli list-objects Node 2>/dev/null | head -30 || echo 'pw-cli failed'"

echo ""
echo "--- pactl list sinks (with volume) ---"
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 PULSE_RUNTIME_PATH=/run/user/999/pulse pactl list sinks short 2>/dev/null || echo 'pactl failed'"

echo ""
echo "--- pw-top snapshot ---"
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 timeout 2 pw-top -b 2>/dev/null | head -20 || echo 'pw-top failed'"

echo ""
echo "--- pw-metadata ---"
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 pw-metadata 2>/dev/null | head -20 || echo 'pw-metadata failed'"

echo ""
echo "--- Check for pw-mon / pw-dump ---"
$SSH "which pw-dump pw-mon pw-record pw-cat pactl 2>/dev/null || echo 'tools check done'"

echo ""
echo "--- ALSA mixer controls for audiocodec ---"
$SSH "amixer -c 0 contents 2>/dev/null | head -40 || echo 'amixer failed'"

echo ""
echo "--- pactl subscribe test (2s) ---"
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 PULSE_RUNTIME_PATH=/run/user/999/pulse timeout 2 pactl subscribe 2>/dev/null || echo '(timeout - normal)'"
