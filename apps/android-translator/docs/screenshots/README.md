# Screenshot capture matrix (Phase 5)

Compose screenshot tooling is not wired in this module. Capture these manually
(or via future instrumentation) on an API 35 emulator / device and store PNGs
beside this README using the exact filenames below.

## Required captures

| File | Surface | Theme | Viewport | Notes |
|---|---|---|---|---|
| `login-dark-phone.png` | Login (Configure) | dark | 390×844 | Welcome (Newsreader), deployment identity mono, fields, Sign in |
| `assignments-dark-phone.png` | Session list | dark | 390×844 | Name primary, status secondary, ULID collapsed |
| `live-connected-dark-phone.png` | Live session | dark | 390×844 | Connected sentence, meters, Stop dominant, Mute |
| `live-reconnecting-dark-phone.png` | Live session | dark | 390×844 | Reconnect banner + polite status |
| `mic-denied-dark-phone.png` | Live session | dark | 390×844 | Mic denied / Open Settings |
| `live-connected-light-phone.png` | Live session | light | 390×844 | Geometry parity with dark |
| `live-connected-dark-tablet.png` | Live session | dark | 834×1112 | Page title 30sp path |
| `live-connected-dark-font200.png` | Live session | dark | 390×844 @ fontScale 2.0 | No clipping; reflow |

## Capture checklist

1. Install debug APK: `./gradlew :app:installDebug`
2. Force dark: `adb shell cmd uimode night yes`
3. Force light: `adb shell cmd uimode night no`
4. Font scale 200%: Settings → Display → Font size, or:
   `adb shell settings put system font_scale 2.0`
5. TalkBack pass: session/channel names before IDs; Mute announces muted/unmuted;
   status sentence is a live region; diagnostics disclosure is not a focus trap.
6. Anti-slop: no system font, generic blue, card grid, emoji, gradient, shadows,
   rounded-everything, ID-led titles, or color-only state. Accent is cyan `#3DE0F0` only.

## Accent

Selected product accent: **cyan `#3DE0F0`**.
