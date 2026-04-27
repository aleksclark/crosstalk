#!/usr/bin/env bash
set -euo pipefail
SSH="ssh -o ConnectTimeout=5 root@${1:?Usage: $0 <board-ip>}"

echo "--- USB devices ---"
$SSH "lsusb"

echo ""
echo "--- ALSA cards ---"
$SSH "cat /proc/asound/cards"

echo ""
echo "--- ALSA PCM devices ---"
$SSH "cat /proc/asound/pcm"

echo ""
echo "--- PipeWire sinks ---"
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 PULSE_RUNTIME_PATH=/run/user/999/pulse pactl list sinks short"

echo ""
echo "--- PipeWire sources ---"
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 PULSE_RUNTIME_PATH=/run/user/999/pulse pactl list sources short"

echo ""
echo "--- PipeWire cards ---"
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 PULSE_RUNTIME_PATH=/run/user/999/pulse pactl list cards short"

echo ""
echo "--- PipeWire card details (USB) ---"
$SSH "sudo -u streamlate XDG_RUNTIME_DIR=/run/user/999 PULSE_RUNTIME_PATH=/run/user/999/pulse pactl list cards 2>/dev/null | grep -A30 -i usb || echo '(no USB card found in pactl)'"

echo ""
echo "--- ALSA USB mixer ---"
$SSH "for c in 0 1 2 3 4; do amixer -c \$c 2>/dev/null | head -1 && echo \"  card \$c\" || true; done"

echo ""
echo "--- Test capture from USB ---"
$SSH "for dev in \$(cat /proc/asound/pcm | grep capture | cut -d: -f1); do
    card=\${dev%%-*}; sub=\${dev##*-}
    echo \"Testing hw:\$card,\$sub...\"
    timeout 1 arecord -D hw:\$card,\$sub -f S16_LE -r 48000 -c 1 /dev/null 2>&1 | tail -2
done"
