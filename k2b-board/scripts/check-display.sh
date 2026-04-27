#!/usr/bin/env bash
# Quick post-reboot check for framebuffer display.
# Usage: ./check-display.sh <board-ip>
set -euo pipefail
BOARD_IP="${1:?Usage: $0 <board-ip>}"
SSH="ssh -o ConnectTimeout=10 root@${BOARD_IP}"

echo "--- Framebuffer devices ---"
$SSH "ls -la /dev/fb* 2>/dev/null || echo 'no /dev/fb*'"

echo "--- DRM devices ---"
$SSH "ls -la /dev/dri/* 2>/dev/null || echo '(no DRM devices)'"

echo "--- Backlight ---"
$SSH "ls /sys/class/backlight/ 2>/dev/null && cat /sys/class/backlight/*/brightness 2>/dev/null || echo '(no backlight)'"

echo "--- Splash service ---"
$SSH "systemctl status ct-splash --no-pager -l 2>/dev/null | head -10 || echo '(not found)'"

echo "--- App service ---"
$SSH "systemctl status app --no-pager -l 2>/dev/null | head -10 || true"

echo "--- dmesg: ili9341/drm/fb ---"
$SSH "dmesg | grep -iE 'ili|tinydrm|drm.*fb|fb[0-9]|panel' | tail -15"

echo "--- Loaded modules ---"
$SSH "lsmod | grep -iE 'ili|drm|fb' || echo '(none)'"

echo "--- Kernel config (display) ---"
$SSH "zcat /proc/config.gz 2>/dev/null | grep -iE 'CONFIG_TINYDRM_ILI|CONFIG_DRM_PANEL_ILITEK|CONFIG_DRM_FBDEV|CONFIG_FB_TFT' || echo '(config not available)'"
