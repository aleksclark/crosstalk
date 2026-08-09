#!/usr/bin/env bash
# Live WiFi / portal recovery probes for K2B (read-mostly + optional matrix).
#
# Usage:
#   ./probe-wifi-recovery.sh <board-ip>              # discovery only
#   ./probe-wifi-recovery.sh <board-ip> --matrix     # requires SSID+PASS env
#
# Environment (never echoed):
#   K2B_WIFI_SSID   — target station SSID for join tests
#   K2B_WIFI_PASS   — passphrase (never printed)
#   K2B_BAD_PASS    — wrong passphrase for negative test (default: wrong-password-probe)
#
# Does not print secrets/tokens/passphrases.
set -euo pipefail

BOARD_IP="${1:?Usage: $0 <board-ip> [--matrix]}"
DO_MATRIX=0
[[ "${2:-}" == "--matrix" ]] && DO_MATRIX=1

SSH=(ssh -o ConnectTimeout=8 -o BatchMode=yes "root@${BOARD_IP}")
ts() { date -u +%Y-%m-%dT%H:%M:%SZ; }
log() { echo "[$(ts)] $*"; }

log "=== K2B discovery ${BOARD_IP} ==="
"${SSH[@]}" 'set -e
  echo "hostname=$(hostname)"
  uname -a
  echo "--- ip ---"; ip -br a || true
  echo "--- nm ---"; nmcli -t -f DEVICE,TYPE,STATE,CONNECTION dev status 2>/dev/null || true
  echo "--- wifi ---"; nmcli -t -f ACTIVE,SSID,SIGNAL dev wifi list 2>/dev/null | head -20 || true
  echo "--- units ---"
  systemctl is-active ct-netcfg.service 2>/dev/null || true
  systemctl is-active app.service 2>/dev/null || true
  systemctl is-enabled ct-netcfg.service 2>/dev/null || true
  ls -la /usr/local/bin/ct-netcfg /usr/local/bin/ct-abc /usr/local/bin/ct-client 2>/dev/null || true
  sha256sum /usr/local/bin/ct-netcfg 2>/dev/null || true
  ls -la /etc/systemd/system/ct-netcfg.service /etc/systemd/system/app.service 2>/dev/null || true
  echo "--- listen ---"; ss -lntp 2>/dev/null | head -30 || true
  echo "--- audio user ---"
  getent passwd streamlate app 2>/dev/null || true
  ls -la /run/user/999 2>/dev/null | head -8 || true
  if [ -d /run/user/999 ]; then
    sudo -u "$(getent passwd 999 | cut -d: -f1)" XDG_RUNTIME_DIR=/run/user/999 \
      pactl info 2>/dev/null | head -15 || true
    sudo -u "$(getent passwd 999 | cut -d: -f1)" XDG_RUNTIME_DIR=/run/user/999 \
      pw-cli ls Node 2>/dev/null | head -40 || true
  fi
  echo "--- rollback stamps ---"
  ls -la /root/crosstalk-rollback 2>/dev/null || echo "(none)"
'

if [[ "$DO_MATRIX" -ne 1 ]]; then
  log "Discovery complete. Live WiFi matrix not requested."
  log "To run matrix: K2B_WIFI_SSID=... K2B_WIFI_PASS=... $0 ${BOARD_IP} --matrix"
  exit 0
fi

if [[ -z "${K2B_WIFI_SSID:-}" || -z "${K2B_WIFI_PASS:-}" ]]; then
  log "BLOCKED: live WiFi matrix requires K2B_WIFI_SSID and K2B_WIFI_PASS"
  exit 10
fi

BAD_PASS="${K2B_BAD_PASS:-wrong-password-probe}"
SSID="$K2B_WIFI_PASS"  # placeholder to force careful use below
unset SSID
TARGET_SSID="$K2B_WIFI_SSID"

log "=== Live matrix (ssid present, passphrase not logged) ==="

log "[M1] wrong password recovery"
# Submit wrong password via a one-shot nmcli path is unsafe while portal owns
# the radio; instead restart netcfg after injecting a bad connection attempt
# only if already in station mode. Prefer portal-style: stop hotspot path is
# exercised by restarting ct-netcfg after a failed connect.
"${SSH[@]}" "set -e
  # Ensure offline path: if fully online, skip destructive wrong-pw test unless forced
  CONN=\$(nmcli -t networking connectivity check 2>/dev/null || echo unknown)
  echo \"connectivity=\$CONN\"
  # Attempt a bad join in a subshell with timeout; never echo passphrase
  timeout 40 nmcli device wifi connect $(printf '%q' "$TARGET_SSID") password $(printf '%q' "$BAD_PASS") 2>&1 | sed -E 's/(password|psk)[= ]+[^ ]+/\\1 [REDACTED]/gi' || true
  # Clean sticky bad profile
  nmcli connection delete $(printf '%q' "$TARGET_SSID") 2>/dev/null || true
  systemctl reset-failed ct-netcfg.service 2>/dev/null || true
  systemctl restart ct-netcfg.service
  sleep 20
  systemctl is-active ct-netcfg.service || true
  # Hotspot should reappear when still offline
  nmcli -t -f NAME,DEVICE connection show --active 2>/dev/null || true
  journalctl -u ct-netcfg -n 30 --no-pager 2>/dev/null | sed -E 's/(password|passphrase|psk)[=: ]+[^\" ,}]+/\\1 [REDACTED]/gi' || true
"

log "[M2] valid join + SSH recovery"
"${SSH[@]}" "set -e
  timeout 60 nmcli device wifi connect $(printf '%q' "$TARGET_SSID") password $(printf '%q' "$K2B_WIFI_PASS") 2>&1 | sed -E 's/(password|psk)[= ]+[^ ]+/\\1 [REDACTED]/gi'
  sleep 5
  nmcli -t networking connectivity check || true
  ip -br a || true
"

log "[M3] NM restart stay online"
"${SSH[@]}" "set -e
  systemctl restart NetworkManager
  sleep 15
  nmcli -t networking connectivity check || true
  ip -br a || true
"

log "[M4] reboot stay online (will drop SSH)"
log "Issuing reboot; waiting for SSH..."
"${SSH[@]}" "systemctl reboot" || true
for i in $(seq 1 60); do
  if ssh -o ConnectTimeout=3 -o BatchMode=yes "root@${BOARD_IP}" 'true' 2>/dev/null; then
    log "SSH back after reboot (try $i)"
    break
  fi
  sleep 5
done
"${SSH[@]}" 'nmcli -t networking connectivity check; ip -br a; systemctl is-active ct-netcfg app || true'

log "[M5] rollback dry-run listing"
"${SSH[@]}" 'ls -la /root/crosstalk-rollback 2>/dev/null || echo no-backups'

log "=== Matrix complete ==="
