#!/usr/bin/env bash
set -euo pipefail
SSH="ssh -o ConnectTimeout=5 root@${1:?Usage: $0 <board-ip>}"

echo "--- spidev ---"
$SSH "ls /dev/spidev* 2>/dev/null || echo MISSING"

echo "--- fbtft loaded ---"
$SSH "lsmod | grep -E 'fbtft|ili' || echo 'none loaded'"

echo "--- blacklist ---"
$SSH "cat /etc/modprobe.d/blacklist-fbtft.conf 2>/dev/null || echo 'no blacklist'"

echo "--- gpio 71/76 ---"
$SSH "ls /sys/class/gpio/gpio71 /sys/class/gpio/gpio76 2>/dev/null || echo 'not exported'"

echo "--- spidev perms ---"
$SSH "ls -la /dev/spidev0.1 2>/dev/null || echo 'no spidev'"

echo "--- app service status ---"
$SSH "systemctl status app --no-pager -l 2>/dev/null | head -20"

echo "--- app logs (last 10) ---"
$SSH "journalctl -u app --no-pager -l -n 10 2>/dev/null"
