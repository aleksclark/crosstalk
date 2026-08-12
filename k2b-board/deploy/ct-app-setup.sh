#!/usr/bin/env bash
# Checked setup helper for app.service (display + audio).
# Distinguishes required vs optional failures:
#   - Required steps print ERROR and exit non-zero.
#   - Optional steps print WARNING and continue.
#
# Required: none by default (board may boot headless / without USB capture).
# Optional: SPI/display GPIO export, PipeWire card profiles, USB capture gain.
#
# When /var/lib/crosstalk/audio-managed exists, skip legacy USB AGC/Mic defaults
# so remote-managed gain is not overwritten during the reconnect window.
#
# Override with CT_APP_REQUIRE_SPI=1 to fail if /dev/spidev0.1 is missing.
set -uo pipefail

log()  { echo "ct-app-setup: $*"; }
warn() { echo "ct-app-setup: WARNING: $*" >&2; }
err()  { echo "ct-app-setup: ERROR: $*" >&2; }

REQUIRE_SPI="${CT_APP_REQUIRE_SPI:-0}"
STATE_DIR="${CT_ABC_STATE_DIR:-/var/lib/crosstalk}"
MANAGED_MARKER="${STATE_DIR}/audio-managed"
# Prefer streamlate (uid 999) when present; fall back to app.
AUDIO_USER="${CT_APP_AUDIO_USER:-}"
if [[ -z "$AUDIO_USER" ]]; then
  if id streamlate >/dev/null 2>&1; then
    AUDIO_USER=streamlate
  elif id app >/dev/null 2>&1; then
    AUDIO_USER=app
  else
    AUDIO_USER=root
  fi
fi
AUDIO_UID="$(id -u "$AUDIO_USER" 2>/dev/null || echo 999)"
XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/${AUDIO_UID}}"

rc=0

log "=== Display setup (user=${AUDIO_USER}) ==="

# Unbind fbtft/ili9341 from SPI device if they grabbed it (optional).
for drv in fb_ili9341 panel-ilitek-ili9341 ili9341; do
  if [[ -e "/sys/bus/spi/drivers/${drv}/spi0.1" ]]; then
    if echo spi0.1 > "/sys/bus/spi/drivers/${drv}/unbind" 2>/dev/null; then
      log "Unbound ${drv} from spi0.1"
    else
      warn "could not unbind ${drv} from spi0.1"
    fi
  fi
done

# Force unload fbtft modules (optional — may be in use).
for mod in ili9341 panel_ilitek_ili9341 fb_ili9341 fbtft drm_mipi_dbi; do
  rmmod "$mod" 2>/dev/null || true
done

# Ensure spidev is bound (optional unless REQUIRE_SPI=1).
modprobe spidev 2>/dev/null || true
if [[ ! -e /sys/bus/spi/drivers/spidev/spi0.1 ]]; then
  echo spidev > /sys/bus/spi/devices/spi0.1/driver_override 2>/dev/null || true
  echo spi0.1 > /sys/bus/spi/drivers/spidev/bind 2>/dev/null || true
fi

if [[ -e /dev/spidev0.1 ]]; then
  chmod 0660 /dev/spidev0.1 2>/dev/null || true
  groupadd -f spi 2>/dev/null || true
  chgrp spi /dev/spidev0.1 2>/dev/null || true
  usermod -aG spi "$AUDIO_USER" 2>/dev/null || true
  log "spidev ready: /dev/spidev0.1"
else
  if [[ "$REQUIRE_SPI" == "1" ]]; then
    err "/dev/spidev0.1 missing (required)"
    rc=1
  else
    warn "/dev/spidev0.1 missing (display optional)"
  fi
fi

# Export and configure GPIOs for DC (71/PC7) and RESET (76/PC12) — optional.
for g in 71 76; do
  if [[ ! -d "/sys/class/gpio/gpio${g}" ]]; then
    echo "$g" > /sys/class/gpio/export 2>/dev/null || true
  fi
  if [[ -d "/sys/class/gpio/gpio${g}" ]]; then
    echo out > "/sys/class/gpio/gpio${g}/direction" 2>/dev/null || true
    chown "$AUDIO_USER" "/sys/class/gpio/gpio${g}/value" "/sys/class/gpio/gpio${g}/direction" 2>/dev/null || true
  else
    warn "gpio${g} not available"
  fi
done

log "=== Audio setup ==="

# Activate PipeWire audio card profiles (optional — cards may be absent).
if [[ -d "$XDG_RUNTIME_DIR" ]]; then
  run_pactl() {
    sudo -u "$AUDIO_USER" XDG_RUNTIME_DIR="$XDG_RUNTIME_DIR" \
      PIPEWIRE_RUNTIME_DIR="$XDG_RUNTIME_DIR" \
      PULSE_RUNTIME_PATH="${XDG_RUNTIME_DIR}/pulse" \
      pactl "$@" 2>/dev/null
  }
  for card in alsa_card.platform-5096000.codec alsa_card.platform-soc_ahub1_mach; do
    if run_pactl set-card-profile "$card" pro-audio; then
      log "card profile set: $card -> pro-audio"
    else
      warn "could not set card profile $card (optional)"
    fi
  done
else
  warn "XDG_RUNTIME_DIR=$XDG_RUNTIME_DIR missing; skip PipeWire profiles"
fi

# Enable ALSA audiocodec DAC -> LINEOUT mixer path only.
# NEVER walk numeric card indices with blind numids: on this board USB
# "Device" shares low numids (Speaker/Mic) and a blind cset zeroes remote
# managed volume/gain during every app.service start.
AUDIOCODEC_CARD=""
while IFS= read -r line; do
  # " 1 [audiocodec     ]: audiocodec - ..."
  if [[ "$line" == *"[audiocodec"* ]] || [[ "$line" == *"]: audiocodec"* ]]; then
    AUDIOCODEC_CARD="${line%% *}"
    AUDIOCODEC_CARD="${AUDIOCODEC_CARD#"${AUDIOCODEC_CARD%%[![:space:]]*}"}"
    break
  fi
done < /proc/asound/cards 2>/dev/null || true
if [[ -n "${AUDIOCODEC_CARD}" ]] && amixer -c "$AUDIOCODEC_CARD" scontrols >/dev/null 2>&1; then
  amixer -c "$AUDIOCODEC_CARD" cset numid=4 on 2>/dev/null || true
  amixer -c "$AUDIOCODEC_CARD" cset numid=5 on 2>/dev/null || true
  amixer -c "$AUDIOCODEC_CARD" cset numid=6 on 2>/dev/null || true
  amixer -c "$AUDIOCODEC_CARD" cset numid=7 on 2>/dev/null || true
  amixer -c "$AUDIOCODEC_CARD" cset numid=2 31 2>/dev/null || true
  amixer -c "$AUDIOCODEC_CARD" cset numid=3 on 2>/dev/null || true
  log "audiocodec LINEOUT path enabled (card ${AUDIOCODEC_CARD})"
else
  warn "audiocodec card not found; skip onboard LINEOUT path"
fi

# USB capture defaults: only when not managed by ct-abc remote audio control.
if [[ -f "$MANAGED_MARKER" ]]; then
  log "managed marker present ($MANAGED_MARKER); skip legacy USB AGC/Mic defaults"
else
  # Resolve stable ALSA card ID (not numeric index) for USB-Audio.
  USB_CARD_ID=""
  while IFS= read -r line; do
    # " 1 [Device         ]: USB-Audio - ..."
    if [[ "$line" == *"]: USB-Audio"* ]] || [[ "$line" == *"]:USB-Audio"* ]]; then
      if [[ "$line" =~ \[([A-Za-z0-9_-]+) ]]; then
        USB_CARD_ID="${BASH_REMATCH[1]}"
        break
      fi
    fi
  done < /proc/asound/cards 2>/dev/null || true

  if [[ -n "${USB_CARD_ID:-}" ]]; then
    if amixer -D "hw:CARD=${USB_CARD_ID}" sset "Auto Gain Control" on 2>/dev/null \
       && amixer -D "hw:CARD=${USB_CARD_ID}" sset Mic 34 cap 2>/dev/null; then
      log "USB capture card id=${USB_CARD_ID}: AGC on, mic gain fixed (legacy unmanaged)"
    else
      warn "USB capture card id=${USB_CARD_ID} present but mixer setup failed (optional)"
    fi
  else
    warn "USB audio card not found for capture setup (optional)"
  fi
fi

if [[ $rc -ne 0 ]]; then
  err "required setup steps failed (rc=$rc)"
  exit "$rc"
fi
log "=== Setup complete ==="
exit 0
