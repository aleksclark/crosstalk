#!/usr/bin/env bash
set -euo pipefail
SSH="ssh -o ConnectTimeout=5 root@${1:?Usage: $0 <board-ip>}"
echo "--- dmesg USB/audio (last 20) ---"
$SSH "dmesg | grep -iE 'usb|audio|sound|snd' | tail -20"
echo ""
echo "--- lsusb ---"
$SSH "lsusb"
