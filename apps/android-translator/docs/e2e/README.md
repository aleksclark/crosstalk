# Android translator E2E evidence (Phase 6)

This directory holds **human-reviewed evidence docs and templates**. Binary
artifacts (WAV, spectra PNG, Logcat) are produced by
`test/android/run-device-golden.sh` into a temp evidence dir and must pass a
secret canary scan before upload.

## Capture labels (anti-cheat)

| Label | Meaning | Acceptable as merge proof? |
|---|---|---|
| `physical-mic` | Real phone mic + external 880 Hz injector; external capture of 440 Hz feed; real listener decode | **Yes** (required) |
| `synthetic-capture-debug-only` | Emulator / fake mic / injected tone in debug builds | **No** for physical gate |

Never relabel synthetic emulator evidence as physical.

## Required physical golden flow

Script: `test/android/run-device-golden.sh`

```text
preflight device/API/battery/audio route
→ install debug APK
→ seed via public APIs (session, Floor Feed, English Broadcast, translator assign, ABC)
→ login/join (UI + instrumented real-server tests)
→ establish 440/880 tones (operator external gear on physical)
→ record pre-sleep stats/spectra
→ adb shell input keyevent KEYCODE_HOME
→ adb shell input keyevent KEYCODE_SLEEP
→ ≥10 minute bounded polling of service/server/listener non-silent evidence
→ adb shell input keyevent KEYCODE_WAKEUP
→ unlock → assert same named session/state
→ toggle network → fresh ticket/new peer + tones recover
→ notification Stop → service/peer terminate, no resurrection
→ redact and archive evidence
```

Process death uses **`am kill`**, not `force-stop`. `force-stop` is terminal
admin only and is documented separately.

## Env (never echo secrets)

| Variable | Role |
|---|---|
| `CROSSTALK_BASE_URL` | Live server base |
| `CROSSTALK_ADMIN_PASSWORD` | Seed only |
| `CROSSTALK_SERIAL` | Optional adb serial |
| `CROSSTALK_SLEEP_SECONDS` | Default 600 |
| `CROSSTALK_ALLOW_EMULATOR=1` | Debug synthetic path only |

## Instrumentation matrix

Under `app/src/androidTest/`:

- `AuthKeystoreInstrumentedTest` — real Android Keystore
- `ForegroundServiceLifecycleInstrumentedTest` — Home / rotation best-effort
- `ScreenOffContinuityInstrumentedTest` — service remains after sleep sim
- `ProcessDeathRejoinInstrumentedTest` — `am kill`, no auto mic restart
- `PermissionRevocationInstrumentedTest` — as feasible on emulator
- `RealServerAssignmentInstrumentedTest` — requires `CROSSTALK_BASE_URL`
- `RealPionWebRtcInstrumentedTest` — requires `CROSSTALK_BASE_URL` + credentials

Real-server tests use `assumeTrue` when env is absent so static CI stays honest;
they **run automatically** when the golden harness injects runner args.

## Managed devices

Gradle names: `pixel2Api33`, `pixel2Api34`, `pixel2Api35` (`aosp-atd`).

Host status at implementation time:

- API 34 `google_apis/x86_64` image present (~4.2G)
- API 35 system image **missing** (do not download >2GB in-lane)
- Prefer `connectedDebugAndroidTest` against attached emulator/device

## CI jobs

See `.github/workflows/ci.yml`:

- `android-static` — JDK17, SDK35, wrapper, OpenAPI, lint, unit, assemble
- `android-instrumented-api35` — KVM emulator when available; clean skip notice otherwise
- `android-physical-audio` — self-hosted `[self-hosted, linux, android-audio]` **or**
  `workflow_dispatch`; fails closed with clear message when runner absent
  (never silently green without evidence)

## Evidence checklist (physical)

- [ ] 440±25 Hz feed at phone speaker with non-zero energy while screen off ≥10 min
- [ ] 880±25 Hz at broadcast listener with non-zero energy
- [ ] Native inbound `totalAudioEnergy` / bytes received increase
- [ ] Mute removes 880 Hz; 440 Hz continues
- [ ] Stop ends service; no resurrection
- [ ] Network recovery mints fresh ticket
- [ ] Secret canary scan clean on archive
- [ ] Capture label = `physical-mic`
