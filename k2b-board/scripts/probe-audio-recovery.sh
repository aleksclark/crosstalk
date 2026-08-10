#!/usr/bin/env bash
# PipeWire / audio discovery probe for K2B.
# Prefer streamlate (uid 999) when present; otherwise live-discover the audio user.
#
# Usage: ./probe-audio-recovery.sh <board-ip>
set -euo pipefail

BOARD_IP="${1:?Usage: $0 <board-ip>}"
SSH=(ssh -o ConnectTimeout=8 -o BatchMode=yes "root@${BOARD_IP}")
ts() { date -u +%Y-%m-%dT%H:%M:%SZ; }
log() { echo "[$(ts)] $*"; }

log "=== Audio discovery ${BOARD_IP} ==="
"${SSH[@]}" 'set -e
  echo "--- passwd ---"
  getent passwd streamlate app 2>/dev/null || true
  echo "--- runtime dirs ---"
  ls -la /run/user 2>/dev/null || true
  # Discover uid 999 owner
  U999=$(getent passwd 999 | cut -d: -f1 || true)
  echo "uid999_user=${U999:-none}"
  AUDIO_USER=""
  if id streamlate >/dev/null 2>&1; then AUDIO_USER=streamlate
  elif [[ -n "${U999:-}" ]]; then AUDIO_USER="$U999"
  elif id app >/dev/null 2>&1; then AUDIO_USER=app
  fi
  echo "audio_user=${AUDIO_USER:-none}"
  if [[ -z "${AUDIO_USER:-}" ]]; then
    echo "NO_AUDIO_USER"; exit 0
  fi
  UIDN=$(id -u "$AUDIO_USER")
  export XDG_RUNTIME_DIR=/run/user/$UIDN
  echo "XDG_RUNTIME_DIR=$XDG_RUNTIME_DIR"
  ls -la "$XDG_RUNTIME_DIR" 2>/dev/null | head -20 || true
  echo "--- pactl ---"
  sudo -u "$AUDIO_USER" XDG_RUNTIME_DIR="$XDG_RUNTIME_DIR" pactl info 2>/dev/null | head -20 || echo "pactl failed"
  echo "--- sinks/sources ---"
  sudo -u "$AUDIO_USER" XDG_RUNTIME_DIR="$XDG_RUNTIME_DIR" pactl list short sinks 2>/dev/null || true
  sudo -u "$AUDIO_USER" XDG_RUNTIME_DIR="$XDG_RUNTIME_DIR" pactl list short sources 2>/dev/null || true
  echo "--- pw nodes ---"
  sudo -u "$AUDIO_USER" XDG_RUNTIME_DIR="$XDG_RUNTIME_DIR" pw-cli ls Node 2>/dev/null | head -50 || true
  echo "--- alsa cards ---"
  cat /proc/asound/cards 2>/dev/null || true
  echo "--- app unit ---"
  systemctl cat app.service 2>/dev/null | head -40 || true
  systemctl is-active app.service 2>/dev/null || true
'
log "=== Audio discovery complete ==="
