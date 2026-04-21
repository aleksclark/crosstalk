#!/usr/bin/env bash
# Provision a fresh K2B board.
# Run from the host machine (not the board).
# Usage: ./provision-k2b.sh <board-ip> [binary-path]
set -euo pipefail

BOARD_IP="${1:?Usage: $0 <board-ip> [binary-path]}"
BINARY="${2:-}"
SSH="ssh -i ~/.ssh/id_rsa root@${BOARD_IP}"
SCP="scp -i ~/.ssh/id_rsa"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "=== Provisioning K2B at ${BOARD_IP} ==="

# 1. Create app user (uid 999, audio group)
echo "Creating app user..."
$SSH "
id app &>/dev/null || {
    useradd -r -u 999 -g systemd-journal -G audio -m -s /bin/bash app
    loginctl enable-linger app
}
"

# 2. Install snd-aloop module config
echo "Configuring ALSA loopback..."
$SCP "${SCRIPT_DIR}/config/modprobe.d/snd-aloop.conf" "root@${BOARD_IP}:/etc/modprobe.d/snd-aloop.conf"
$SCP "${SCRIPT_DIR}/config/modules-load.d/snd-aloop.conf" "root@${BOARD_IP}:/etc/modules-load.d/snd-aloop.conf"
$SSH "modprobe snd-aloop pcm_notify=1 pcm_substreams=2 2>/dev/null || true"

# 3. Install PipeWire stack (if not present)
echo "Ensuring PipeWire is installed..."
$SSH "
dpkg -s pipewire &>/dev/null || {
    apt-get update -qq
    apt-get install -y -qq pipewire pipewire-pulse wireplumber pipewire-alsa pulseaudio-utils
}
"

# 4. Enable PipeWire user services
echo "Enabling PipeWire for app user..."
$SSH "
sudo -u app XDG_RUNTIME_DIR=/run/user/999 systemctl --user enable pipewire pipewire-pulse wireplumber
sudo -u app XDG_RUNTIME_DIR=/run/user/999 systemctl --user start pipewire pipewire-pulse wireplumber
"

# 5. Deploy config
echo "Deploying config..."
$SSH "mkdir -p /etc/app"
$SCP "${SCRIPT_DIR}/config/config.toml" "root@${BOARD_IP}:/etc/app/config.toml"

# 6. Deploy binary (if provided)
if [ -n "$BINARY" ]; then
    echo "Deploying binary from ${BINARY}..."
    $SCP "$BINARY" "root@${BOARD_IP}:/usr/local/bin/app"
    $SSH "chmod +x /usr/local/bin/app"
fi

# 7. Install systemd service
echo "Installing systemd service..."
$SCP "${SCRIPT_DIR}/deploy/app.service" "root@${BOARD_IP}:/etc/systemd/system/app.service"
$SSH "systemctl daemon-reload && systemctl enable app"

# 8. Start service (only if binary exists)
$SSH "
if [ -x /usr/local/bin/app ]; then
    systemctl restart app
    echo 'Service started.'
else
    echo 'No binary found at /usr/local/bin/app — deploy one and restart.'
fi
"

echo ""
echo "=== Provisioning complete ==="
echo "Board: ${BOARD_IP}"
echo "SSH:   ssh -i ~/.ssh/id_rsa root@${BOARD_IP}"
echo "Logs:  ssh root@${BOARD_IP} journalctl -u app -f"
