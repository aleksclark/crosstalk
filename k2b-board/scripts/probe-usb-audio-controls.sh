#!/usr/bin/env bash
# Read-only probe of K2B USB audio identity, ALSA mixer controls, PipeWire names,
# service identity, and config source/sink (token redacted).
#
# MUST NOT call: sset, cset, wpctl set-*, systemctl restart/start/stop.
#
# Usage:
#   ./probe-usb-audio-controls.sh [board-ip]
#   BOARD_IP=192.168.0.109 ./probe-usb-audio-controls.sh
set -euo pipefail

BOARD_IP="${1:-${BOARD_IP:-}}"
if [[ -z "$BOARD_IP" ]]; then
  echo "Usage: $0 <board-ip>" >&2
  exit 1
fi

SSH=(ssh -o ConnectTimeout=8 -o BatchMode=yes "root@${BOARD_IP}")

remote() {
  "${SSH[@]}" "$@"
}

echo "=== CrossTalk K2B USB audio control probe (read-only) ==="
echo "board: ${BOARD_IP}"
echo "when:  $(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo ""

echo "--- Host / service identity ---"
remote 'set -euo pipefail
  hostname
  uname -a
  systemctl is-active app.service 2>/dev/null || true
  systemctl show app.service -p User -p Group -p FragmentPath -p StateDirectory -p ExecStart --no-pager 2>/dev/null || true
  if [ -x /usr/local/bin/ct-abc ]; then
    ls -la /usr/local/bin/ct-abc
    sha256sum /usr/local/bin/ct-abc
  fi
'

echo ""
echo "--- Config (token redacted) ---"
remote 'set -euo pipefail
  CFG=/etc/app/crosstalk.json
  if [ -f "$CFG" ]; then
    if command -v python3 >/dev/null 2>&1; then
      python3 - <<'\''PY'\''
import json,sys
p="/etc/app/crosstalk.json"
try:
    d=json.load(open(p))
except Exception as e:
    print("config_error",e); sys.exit(0)
if "token" in d:
    t=str(d["token"])
    d["token"]="***REDACTED***" if not t else (t[:4]+"…"+"(len=%d)"%len(t))
for k in ("server_url","source_name","sink_name","log_level","token"):
    if k in d: print(f"{k}={d[k]}")
PY
    else
      # Fallback: sed-redact token line
      sed -E "s/(\"token\"[[:space:]]*:[[:space:]]*\")[^\"]+/\1***REDACTED***/" "$CFG"
    fi
  else
    echo "config_missing"
  fi
  MARK=/var/lib/crosstalk/audio-managed
  if [ -f "$MARK" ]; then
    echo "managed_marker=present"
    cat "$MARK" 2>/dev/null || true
  else
    echo "managed_marker=absent"
  fi
'

echo ""
echo "--- USB devices (lsusb) ---"
remote 'lsusb 2>/dev/null || true'

echo ""
echo "--- ALSA cards ---"
remote 'cat /proc/asound/cards 2>/dev/null || true'

echo ""
echo "--- ALSA card IDs (sysfs) ---"
remote 'set +e
  for d in /sys/class/sound/card*; do
    [ -d "$d" ] || continue
    id=$(cat "$d/id" 2>/dev/null || echo "?")
    echo "$(basename "$d") id=${id}"
    # USB parent identity (redact serial middle)
    dev="$d/device"
    if [ -e "$dev/idVendor" ]; then
      vid=$(cat "$dev/idVendor" 2>/dev/null)
      pid=$(cat "$dev/idProduct" 2>/dev/null)
      ser=$(cat "$dev/serial" 2>/dev/null || true)
      if [ -n "$ser" ] && [ "${#ser}" -gt 4 ]; then
        ser="${ser:0:2}***${#ser}"
      fi
      echo "  usb vid=${vid} pid=${pid} serial=${ser:-none}"
      readlink -f "$dev" 2>/dev/null | sed "s/^/  sysfs=/"
    else
      # walk parents
      cur=$(readlink -f "$dev" 2>/dev/null || true)
      for i in 1 2 3 4 5 6; do
        [ -n "$cur" ] || break
        if [ -f "$cur/idVendor" ]; then
          vid=$(cat "$cur/idVendor")
          pid=$(cat "$cur/idProduct")
          ser=$(cat "$cur/serial" 2>/dev/null || true)
          if [ -n "$ser" ] && [ "${#ser}" -gt 4 ]; then
            ser="${ser:0:2}***${#ser}"
          fi
          echo "  usb vid=${vid} pid=${pid} serial=${ser:-none}"
          echo "  sysfs=$cur"
          break
        fi
        cur=$(dirname "$cur")
      done
    fi
  done
'

echo ""
echo "--- ALSA simple controls (read-only sget/scontrols) ---"
remote 'set +e
  for d in /sys/class/sound/card*; do
    [ -d "$d" ] || continue
    id=$(cat "$d/id" 2>/dev/null || continue)
    echo "## card id=${id}"
    amixer -D "hw:CARD=${id}" scontrols 2>/dev/null || continue
    # sget for common semantic names only
    for ctrl in Speaker Headphone PCM Mic Capture "Auto Gain Control"; do
      out=$(amixer -M -D "hw:CARD=${id}" sget "$ctrl" 2>/dev/null) || continue
      echo "--- sget $ctrl ---"
      echo "$out" | head -n 20
    done
  done
'

echo ""
echo "--- PipeWire / Pulse names (read-only) ---"
remote 'set +e
  UID_NUM=$(id -u app 2>/dev/null || echo 999)
  export XDG_RUNTIME_DIR=/run/user/${UID_NUM}
  export PIPEWIRE_RUNTIME_DIR=$XDG_RUNTIME_DIR
  export PULSE_RUNTIME_PATH=$XDG_RUNTIME_DIR/pulse
  if command -v pactl >/dev/null 2>&1; then
    sudo -u app XDG_RUNTIME_DIR=$XDG_RUNTIME_DIR PIPEWIRE_RUNTIME_DIR=$PIPEWIRE_RUNTIME_DIR PULSE_RUNTIME_PATH=$PULSE_RUNTIME_PATH \
      pactl list cards short 2>/dev/null || true
    sudo -u app XDG_RUNTIME_DIR=$XDG_RUNTIME_DIR PIPEWIRE_RUNTIME_DIR=$PIPEWIRE_RUNTIME_DIR PULSE_RUNTIME_PATH=$PULSE_RUNTIME_PATH \
      pactl list sinks short 2>/dev/null || true
    sudo -u app XDG_RUNTIME_DIR=$XDG_RUNTIME_DIR PIPEWIRE_RUNTIME_DIR=$PIPEWIRE_RUNTIME_DIR PULSE_RUNTIME_PATH=$PULSE_RUNTIME_PATH \
      pactl list sources short 2>/dev/null || true
  else
    echo "pactl missing"
  fi
'

echo ""
echo "=== Probe complete (no mixer writes, no service restarts) ==="
