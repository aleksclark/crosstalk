# Physical device golden — operator runbook

## Hardware

- Android 15 phone (API 35) preferred; Android 13/14 smoke optional
- Calibrated external speaker aimed at phone mic (880 Hz translation stimulus)
- Calibrated external capture mic at phone earpiece/speaker (440 Hz feed return)
- Host running CrossTalk (`ct-server` + PostgreSQL) reachable from the phone
- Optional: second host/browser as broadcast listener for 880 Hz decode

## Separation

Keep injector and capture paths physically separated to reject acoustic
crosstalk. Document geometry in the evidence `summary.json` notes field.

## Commands

```bash
export JAVA_HOME=/usr/lib/jvm/java-17-openjdk
export ANDROID_HOME=/opt/android-sdk
export CROSSTALK_BASE_URL=http://<lan-ip>:8080
export CROSSTALK_ADMIN_PASSWORD='***'   # never commit / never echo
export CROSSTALK_SERIAL=<adb-serial>    # optional
export CROSSTALK_SLEEP_SECONDS=600

# Physical only — omit CROSSTALK_ALLOW_EMULATOR
bash test/android/run-device-golden.sh
```

## Emulator (debug only)

```bash
export CROSSTALK_ALLOW_EMULATOR=1
export CROSSTALK_SLEEP_SECONDS=60   # short smoke
bash test/android/run-device-golden.sh
```

Evidence will be labelled `synthetic-capture-debug-only` and is **not** merge
proof for bidirectional physical audio.

## am kill vs force-stop

| Action | Use |
|---|---|
| `adb shell am kill <pkg>` | Process-death / LMK simulation; app must not auto-restart mic |
| `adb shell am force-stop <pkg>` | Terminal admin; clears stopped state; **not** continuity expectation |

## Residual / blockers to record

- TURN-only networks
- OEM battery savers killing FGS
- Bluetooth route flips
- Missing physical Android 13/14 hardware
