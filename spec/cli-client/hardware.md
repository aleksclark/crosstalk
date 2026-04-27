# Hardware: KickPi K2B

[← Back to Index](../index.md) · [CLI Client Overview](overview.md)

---

## Board Overview

The primary deployment target for the CLI client is the [KickPi K2B](https://www.kickpi.net/) board.

- ARM-based SBC
- TRRS 3.5mm audio jack (combined mic input + headphone output)
- MSP2401 ILI9341 SPI LCD (320×240) for status display
- Ethernet connectivity
- Runs Armbian Linux with PipeWire for audio

## Display

MSP2401 ILI9341 LCD connected to SPI1, driven by the kernel tinydrm `ili9341` driver as a framebuffer (`/dev/fb1`). PWM backlight on PH3.

### Boot Sequence

```
Kernel → loads ili9341 tinydrm → /dev/fb1 appears
       → ct-splash.service writes splash image to /dev/fb1
       → app.service starts ct-client
       → ct-client mmaps /dev/fb1 for live status display
```

### Display Provisioning

```bash
k2b-board/scripts/provision-display-fb.sh <board-ip>
```

This installs the DT overlay, configures the ili9341 kernel module, sets up fbcon, and installs the boot splash service.

## Audio Path

```
TRRS Jack (mic in) → ALSA → PipeWire source → CLI client → WebRTC
WebRTC → CLI client → PipeWire sink → ALSA → TRRS Jack (headphone out)
```

The TRRS jack provides:
- **Input**: Mono microphone from the ring contact
- **Output**: Stereo headphone on tip + ring contacts

## Deployment

Existing infrastructure in `k2b-board/`:

- `scripts/build-image.sh` — build Armbian image with CrossTalk overlay
- `scripts/deploy.sh` — deploy CLI binary to device
- `scripts/provision-k2b.sh` — first-time device setup
- `scripts/provision-display-fb.sh` — set up ILI9341 as kernel framebuffer
- `scripts/setup-loopback.sh` — create PipeWire loopback for testing
- `deploy/app.service` — systemd service unit for the CLI client
- `deploy/ct-splash.service` — early boot splash on the LCD
- `deploy/ct-splash.sh` — splash image writer

### Deployment Flow

```
Build CLI binary (cross-compile for ARM)
        ↓
deploy.sh: scp binary to device, restart service
        ↓
systemd manages CLI client lifecycle
```

### Configuration on Device

Environment variables set in the systemd service unit or `/etc/environment`:
- `CROSSTALK_SERVER`
- `CROSSTALK_TOKEN`
- `CROSSTALK_SOURCE_NAME` (typically: role-specific name like `translator-mic`)
- `CROSSTALK_SINK_NAME` (typically: `translator-speakers`)

## Testing with Loopback

For automated testing without physical audio hardware:

```
setup-loopback.sh creates:
  PipeWire loopback source → captures audio played into it
  PipeWire loopback sink → outputs audio that can be recorded

Test flow:
  1. Play MP3 into loopback source (simulates mic input)
  2. CLI client picks it up as a source, sends via WebRTC
  3. CLI client receives audio on sink, writes to loopback sink
  4. Record from loopback sink to MP3 (captured output)
  5. Compare input/output for golden test validation
```

> See [Dev Environment > K2B Deploy Loop](../dev-environment/k2b-deploy.md) for the watcher script and test harness.
