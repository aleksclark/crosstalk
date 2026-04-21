# KickPi K2B V2 — Board Reference

## Hardware
- **Board**: KickPi K2B V2 (Allwinner H618, sun50iw9)
- **SoC**: Allwinner H618 (quad Cortex-A53, 1GB RAM)
- **Storage**: 14.5GB eMMC (mmcblk2)
- **Network**: WiFi (brcm), no ethernet
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
- Application user `app` (uid 999, groups: systemd-journal, audio)
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

## Application Service
- Binary: `/usr/local/bin/app` (aarch64 static)
- Config: `/etc/app/config.toml`
- Systemd: `/etc/systemd/system/app.service`
- Runs as user `app`, group `audio`
- Restart: `sudo systemctl restart app`

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
