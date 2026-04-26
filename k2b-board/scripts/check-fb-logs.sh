#!/usr/bin/env bash
set -euo pipefail
SSH="ssh -o ConnectTimeout=5 root@${1:?Usage: $0 <board-ip>}"

echo "--- App logs (display related) ---"
$SSH "journalctl -u app --no-pager -l 2>/dev/null | grep -iE 'display|fb|backlight|framebuffer|error|WARN' | tail -20"

echo ""
echo "--- Full recent app logs ---"
$SSH "journalctl -u app --no-pager -l -n 30 2>/dev/null"

echo ""
echo "--- dmesg fbtft (recent) ---"
$SSH "dmesg | grep -i fbtft | tail -10"

echo ""
echo "--- fb0 permissions ---"
$SSH "ls -la /dev/fb0; id streamlate; groups streamlate"

echo ""
echo "--- Quick write test: fill fb0 with red ---"
$SSH "dd if=/dev/urandom bs=153600 count=1 2>/dev/null | head -c 153600 > /dev/fb0 && echo 'write OK' || echo 'write FAILED'"
