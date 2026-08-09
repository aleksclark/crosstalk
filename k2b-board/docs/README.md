# KickPi K2B V2 — Board Reference

## Hardware
- **Board**: KickPi K2B V2 (Allwinner H618, sun50iw9)
- **SoC**: Allwinner H618 (quad Cortex-A53, 1GB RAM)
- **Storage**: 14.5GB eMMC (mmcblk2)
- **Network**: WiFi (brcm) + built-in ethernet
- **Audio**: onboard audiocodec (card 0), HDMI (card 1), snd-aloop virtual (card 2)
- **Kernel**: 6.12.47-current-sunxi64 (aarch64)
- **DTB**: sun50i-h618-kickpi-k2b-v2.dtb

## OS
- Armbian-unofficial 25.11.0-trunk (Ubuntu Noble 24.04 base)
- Board family: sun50iw9-bpi
- Image type: user-built via Armbian build framework

## Current Network
- Hostname: `k2b-1`
- WiFi managed by NetworkManager
- IP: assigned via DHCP on wlan0
- MAC: `60:48:9c:41:b3:e4`

## SSH Access
- Root: `ssh -i ~/.ssh/id_rsa root@<board-ip>`
- Application user `app` (uid 999, groups: systemd-journal, audio, video, spi)
- Lingering enabled for app user

## Audio Stack
- PipeWire 1.0.5 + WirePlumber 0.4.17 + pipewire-pulse (running as user `app`)
- ALSA loopback (snd-aloop) loaded at boot with 2 subdevices
- PipeWire runs as systemd user service (lingering)
- XDG_RUNTIME_DIR=/run/user/999

### Audio Devices
| Card | Name | Use |
|------|------|-----|
| 0 | audiocodec | Onboard DAC (playback) |
| 1 | HDMI | HDMI audio output |
| 2 | Loopback | Virtual loopback (2 subdevices) for routing |

### Loopback Subdevice Mapping
```
plughw:Loopback,0,0  <-->  plughw:Loopback,1,0   (audio path 1)
plughw:Loopback,0,1  <-->  plughw:Loopback,1,1   (audio path 2)
```

## Display

MSP2401 ILI9341 SPI LCD (320×240, RGB565) connected to SPI1 on the 20-pin header.

### Wiring
| MSP2401 | K2B Pin | Signal       | GPIO   |
|---------|---------|--------------|--------|
| VCC     | 2       | VCC_3V3      |        |
| GND     | 6       | GND          |        |
| SCK     | 13      | SPI1_CLK     | PH6    |
| SDI     | 9       | SPI1_MOSI    | PH7    |
| CS      | 7       | SPI1_CS1     | PH9    |
| DC/RS   | 14      | GPIO_PC7     | gpio71 |
| RESET   | 12      | GPIO_PC12    | gpio76 |
| LED     | PWM1    | GPIO_PH3     |        |

### Driver Stack
- **Kernel driver**: tinydrm `ili9341` module (`compatible = "adafruit,yx240qv29"`)
- **Device tree overlay**: `config/overlays/ili9341-spi1.dts` → `/boot/overlay-user/ili9341-spi1.dtbo`
- **Framebuffer**: `/dev/fb1` (fb0 = HDMI), RGB565 16bpp, 320×240
- **Backlight**: PWM via `/sys/class/backlight/` (pwm-backlight node in DT)
- **Console**: fbcon maps to fb1 (`fbcon=map:1 fbcon=font:VGA8x8`)

### Boot Sequence
1. Kernel loads ili9341 tinydrm module → creates `/dev/fb1`
2. `ct-splash.service` (sysinit.target) → writes "CROSSTALK" splash to fb1
3. `app.service` (multi-user.target) → `ct-abc` takes over fb1 for status display

### Provisioning
```bash
k2b-board/scripts/provision-display-fb.sh <board-ip>
```

## Application Service
- Binary: `/usr/local/bin/ct-abc` (aarch64 static; Audio Booth Connector)
  - Historical name `ct-client` still appears in some scripts/docs; the unit and
    on-device path are **`ct-abc`**.
- Config: `/etc/app/crosstalk.json`
- Systemd: `/etc/systemd/system/app.service`
- Setup helper: `/usr/local/lib/crosstalk/ct-app-setup.sh` (checked required vs optional steps)
- Runs as user `app`, groups `audio`, `video`, `spi`
- PipeWire runtime: uid 999 (`streamlate` or `app` depending on image)
- Restart: `sudo systemctl restart app`

## WiFi Provisioning (Captive Portal)

When the box can't reach its configured WiFi network, `ct-netcfg` raises a WiFi
hotspot and a captive-portal web UI so WiFi credentials can be entered from a
phone. No SSH or serial console required.

- Binary: `/usr/local/bin/ct-netcfg` (aarch64 static, no CGO)
- Systemd: `/etc/systemd/system/ct-netcfg.service` (runs as `root`)
- Code: `cli/cmd/ct-netcfg`, `cli/netcfg` (nmcli wrapper), `cli/portal` (web UI)
- Requires `dnsmasq-base` (hotspot DHCP) and `iptables` (hotspot NAT) — without
  them NetworkManager's shared/AP mode fails with "IP configuration could not
  be reserved". Installed by `provision-k2b.sh`.

### Flow (single-radio)
1. On boot `ct-netcfg` waits up to `CT_NETCFG_ONLINE_TIMEOUT` (default 45s) for
   NetworkManager to report full connectivity.
2. If still offline it:
   - brings up the wired ethernet interface in DHCP mode as a fallback
     management path (critical alternate path before WiFi disruption),
   - **scans for networks before starting the hotspot** (the radio cannot scan
     while hosting the AP) and injects the cached list into the portal, then
   - starts the AP (default SSID `CrossTalk-Setup`, password `crosstalk`) with
     captive DNS under `/etc/NetworkManager/dnsmasq-shared.d/ct-captive.conf`.
3. The portal (port 80) shows the cached list + always allows manual/hidden
   SSID entry. Submitting POSTs `/connect` (idempotent). Main then **tears the
   hotspot down first**, joins as station (with hidden-SSID profile fallback),
   and waits for full connectivity. Passphrases are never logged.
4. Wrong password / join failure: process exits non-zero; systemd restarts and
   the portal reappears for retry (sticky bad profiles are deleted).
5. On success the process exits cleanly; `app.service` runs over the new WiFi.

The unit `Restart=always`, so it keeps offering the portal until the box is
provisioned and re-checks connectivity if WiFi drops later.

### Deploy / rollback
```bash
task build:netcfg:arm64
k2b-board/scripts/deploy-netcfg.sh 192.168.0.109 bin/ct-netcfg-arm64
# Atomic: backup under /root/crosstalk-rollback/$STAMP, temp+checksum+rename.
k2b-board/scripts/deploy-netcfg.sh --rollback 192.168.0.109          # newest
k2b-board/scripts/deploy-netcfg.sh --rollback 192.168.0.109 $STAMP
```

### Live probes
```bash
k2b-board/scripts/probe-wifi-recovery.sh 192.168.0.109
K2B_WIFI_SSID=... K2B_WIFI_PASS=... k2b-board/scripts/probe-wifi-recovery.sh 192.168.0.109 --matrix
k2b-board/scripts/probe-audio-recovery.sh 192.168.0.109
```

### Provisioning from a phone
1. Join the `CrossTalk-Setup` WiFi (password `crosstalk`).
2. The captive-portal sign-in sheet opens automatically (or browse to
   `http://10.42.0.1/`).
3. Pick your network, enter the password, tap **Connect**.

### Config (env in the unit)
| Variable | Default | Purpose |
|---|---|---|
| `CT_NETCFG_HOTSPOT_SSID` | `CrossTalk-Setup` | Hotspot network name |
| `CT_NETCFG_HOTSPOT_PASS` | `crosstalk` | Hotspot password (8+ chars, blank = open) |
| `CT_NETCFG_PORTAL_ADDR` | `:80` | Portal listen address |
| `CT_NETCFG_ONLINE_TIMEOUT` | `45s` | Wait for existing WiFi before starting the hotspot |
| `CT_NETCFG_IFACE` | (auto) | Wireless interface, blank lets NM choose |

### Deploy
```bash
task build:netcfg:arm64
k2b-board/scripts/deploy-netcfg.sh <board-ip> bin/ct-netcfg-arm64
```
`provision-k2b.sh` also installs the unit; set `NETCFG_BINARY=bin/ct-netcfg-arm64`
to deploy the binary during provisioning.

## Image Build
- Built with Armbian build framework
- Custom overlay places the application binary at `/usr/local/bin/`
- Board config: `kickpi-k2b-v2`, branch `current`, family `sunxi64`

## Boot Config (/boot/armbianEnv.txt)
```
verbosity=1
bootlogo=false
console=both
disp_mode=1920x1080p60
overlay_prefix=sun50i-h616
fdtfile=sun50i-h618-kickpi-k2b-v2.dtb
rootdev=UUID=d32b5ff9-b9c1-4a4d-9221-8d332a5d2d02
rootfstype=ext4
```

## Cross-Compilation (Rust example)
```bash
cross build --release -p my-app --target aarch64-unknown-linux-gnu
```

## Deploying Updates
```bash
scp target/aarch64-unknown-linux-gnu/release/my-app root@<board-ip>:/usr/local/bin/app
ssh root@<board-ip> "systemctl restart app"
```
