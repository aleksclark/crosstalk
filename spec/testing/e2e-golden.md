# E2E / Golden Tests

[← Back to Index](../index.md) · [Testing Overview](overview.md)

---

## Scope

End-to-end tests that prove **actual audio flows through the entire system** — from one endpoint to another, through real WebRTC connections, with recorded output compared to expected results.

These tests run in the dev environment with a real K2B board connected.

## Golden Test: Admin Web → K2B Device

Verify that audio sent from the browser reaches the K2B device's output.

```
Admin Web UI (Playwright)
    │
    ├── Browser mic replaced with test audio file
    │   (Playwright can inject media streams)
    │
    ├── Connects to session as "translator" role
    │
    ├── Audio flows: browser → server (WebRTC) → K2B CLI client (WebRTC)
    │
    └── K2B CLI client outputs to PipeWire loopback sink

K2B Device
    │
    ├── PipeWire loopback sink records to file
    │
    └── Retrieved via scp, compared to input
```

### Steps

1. Create session from Translation template
2. CLI client on K2B connects as "studio" role
3. Playwright opens session connect view, connects as "translator"
4. Playwright injects test audio stream as mic input
5. Audio routes: translator:mic → studio:output (per template)
6. K2B records from loopback sink
7. Retrieve recorded file, compare to reference

### Comparison

- Not bit-exact (WebRTC encoding introduces artifacts)
- Use audio fingerprinting or cross-correlation to verify the right audio arrived
- Threshold-based: correlation > 0.9 = pass

## Golden Test: K2B Device → Admin Web

Verify that audio from the K2B device reaches the browser.

```
K2B Device
    │
    ├── ffmpeg plays test audio into PipeWire loopback source
    │
    ├── CLI client captures from loopback source
    │
    ├── Audio flows: K2B CLI (WebRTC) → server → browser (WebRTC)
    │
    └── Playwright captures browser audio output

Admin Web UI (Playwright)
    │
    ├── Connected to session as "translator" role
    │
    ├── Receives audio on translator:speakers
    │
    └── Playwright records audio from the page
        (Web Audio API capture or similar)
```

### Steps

1. Create session, connect both clients
2. Play test audio into K2B loopback source
3. Audio routes: studio:input → translator:speakers (per template)
4. Playwright captures audio output from the browser page
5. Compare captured audio to reference

## Test Infrastructure

### Fixtures

- `test/fixtures/test-speech-5s.mp3` — 5-second speech sample
- `test/fixtures/test-tone-1khz.wav` — 1kHz sine wave (easier to verify programmatically)
- `test/fixtures/reference/` — expected output files for comparison

### Audio Comparison Tool

```bash
# dev/scripts/compare-audio.sh
# Cross-correlate two audio files, output similarity score
ffmpeg -i "$1" -f f32le -ar 16000 -ac 1 /tmp/a.raw
ffmpeg -i "$2" -f f32le -ar 16000 -ac 1 /tmp/b.raw
# Use sox or custom tool for correlation
sox /tmp/a.raw -n stat 2>&1  # basic stats
# ... cross-correlation logic
```

### Run

```bash
# Requires: dev environment running + K2B board connected
dev/scripts/run-e2e-tests.sh
```

## Limitations

- Requires physical K2B board on the network (not CI-friendly)
- Audio comparison has inherent tolerance (codec artifacts, latency)
- Test duration: ~30-60 seconds per golden test (real-time audio playback)
- Network conditions affect results — run on a stable local network
