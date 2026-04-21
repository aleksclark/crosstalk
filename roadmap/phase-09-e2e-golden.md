# Phase 9: E2E Golden Audio Tests

[← Roadmap](index.md)

**Status**: `NOT STARTED`  
**Depends on**: Phase 8 (integration tests) + K2B board on network

Real audio through real hardware. This is the final acceptance gate.

## Tasks

### 9.1 Audio Comparison Tool
- [ ] `dev/scripts/compare-audio.sh` — takes two audio files, outputs cross-correlation score (0.0–1.0)
- [ ] Normalize both files to same sample rate (16kHz mono) via ffmpeg
- [ ] Cross-correlation via sox or custom Go tool
- [ ] Exit 0 if score > 0.9, exit 1 otherwise

**Test**: Compare a file with itself → score ~1.0. Compare with silence → score ~0.0. Compare with a slightly compressed version → score > 0.9.

### 9.2 Test Fixtures
- [ ] `test/fixtures/test-tone-1khz-5s.wav` — 5-second 1kHz sine wave
- [ ] `test/fixtures/test-speech-5s.mp3` — 5-second speech sample
- [ ] Generate test tone: `ffmpeg -f lavfi -i "sine=frequency=1000:duration=5" -ar 48000 test-tone-1khz-5s.wav`

**Test**: Fixtures exist and are valid audio files (ffprobe succeeds).

### 9.3 Golden Test: Browser → K2B
- [ ] `dev/scripts/run-e2e-tests.sh` orchestrates:
  1. Ensure K2B PipeWire loopback is set up
  2. Create session from Translation template via REST
  3. Start `ct-client` on K2B, join as `studio` role
  4. Playwright opens session connect view, connects as `translator`
  5. Playwright injects test tone as mic input (via media stream injection)
  6. Wait 6 seconds
  7. Retrieve recorded audio from K2B loopback sink
  8. Run `compare-audio.sh` against reference → score > 0.9

**Test**: `task test:e2e` includes this test. Passes if cross-correlation > 0.9.

### 9.4 Golden Test: K2B → Browser
- [ ] Same script orchestrates:
  1. Create session, connect both clients
  2. Play test tone into K2B loopback source via ffmpeg
  3. Playwright captures browser audio output (Web Audio API recorder)
  4. Wait 6 seconds
  5. Retrieve captured audio from Playwright
  6. Run `compare-audio.sh` against reference → score > 0.9

**Test**: `task test:e2e` includes this test. Passes if cross-correlation > 0.9.

### 9.5 test:e2e Task
- [ ] `task test:e2e` runs `dev/scripts/run-e2e-tests.sh`
- [ ] Requires: dev environment running + K2B board on network
- [ ] Reports pass/fail with correlation scores for each direction

**Test**: `task test:e2e` exits 0 when both golden tests pass.

## Exit Criteria

`task test:e2e` passes both golden tests:

1. **Browser → K2B**: 1kHz tone sent from browser arrives at K2B PipeWire loopback sink. Cross-correlation with reference > 0.9.
2. **K2B → Browser**: 1kHz tone played into K2B PipeWire loopback source arrives at browser. Cross-correlation with reference > 0.9.

This proves real audio flows through the full system: browser → WebRTC → server (pure forwarding) → WebRTC → CLI client → PipeWire, and vice versa.

## Spec Updates

- 7.3 E2E / Golden Tests → 7
- 2.2 WebRTC Signaling → 7
- 2.3 Session Orchestration → 8
- 3.2 PipeWire Integration → 7
- 3.3 Channel Lifecycle → 7

## What's Next

After this phase, the core audio pipeline is proven. Remaining work:
- Broadcast client implementation
- Session recording polish (playback, management UI)
- Error recovery hardening
- Production deployment
- Monitoring / observability
