# Screenshot capture matrix (Phase 5)

Compose screenshots are produced by
`app/src/androidTest/.../ui/ScreenshotInstrumentedTest` using
`captureToImage` with demo/fake UI state (no live server required).

## Required captures

| File | Surface | Theme | Viewport | Notes |
|---|---|---|---|---|
| `login-dark-phone.png` | Login (Configure) | dark | ~390dp | Welcome (Newsreader), deployment identity mono, fields, Sign in |
| `assignments-dark-phone.png` | Session list | dark | ~390dp | Name primary, status secondary, ULID collapsed |
| `live-connected-dark-phone.png` | Live session | dark | ~390dp | Connected sentence, meters, Stop dominant, Mute |
| `live-reconnecting-dark-phone.png` | Live session | dark | ~390dp | Reconnect banner + polite status |
| `mic-denied-dark-phone.png` | Live session | dark | ~390dp | Mic denied / Open Settings |
| `live-connected-light-phone.png` | Live session | light | ~390dp | Geometry parity with dark |
| `live-connected-dark-tablet.png` | Live session | dark | device | Same surface; tablet width when available |
| `live-connected-dark-font200.png` | Live session | dark | ~390dp @ fontScale 2.0 | Set `font_scale` before class for true 200% |

Checksums: `CHECKSUMS.sha256` (sha256 of each PNG).

## Capture via instrumentation

```bash
cd apps/android-translator
./gradlew :app:connectedDebugAndroidTest \
  -Pandroid.testInstrumentationRunnerArguments.class=com.crosstalk.translator.ui.ScreenshotInstrumentedTest

# Pull PNGs + checksums (public path survives app-data wipe):
adb pull /sdcard/Download/crosstalk-screenshots/. docs/screenshots/
(cd docs/screenshots && sha256sum *.png > CHECKSUMS.sha256)
```

`test/android/run-device-golden.sh` also pulls screenshots after the connected
suite and mirrors them into this directory when present.

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
