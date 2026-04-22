# Phase 9: E2E Golden Audio Tests

[← Roadmap](index.md)

**Status**: `IN PROGRESS — gaps addressed, golden tests rewritten for full WebRTC path`  
**Depends on**: Phase 8 (integration tests) + K2B board on network

Real audio through real hardware. This is the final acceptance gate.

## Tasks

### 9.1 Audio Comparison Tool
- [x] `dev/scripts/compare-audio.sh` — takes two audio files, outputs cross-correlation score (0.0–1.0) `0ec566b`
- [x] Normalize both files to same sample rate (16kHz mono) via ffmpeg `0ec566b`
- [x] Cross-correlation via Python (Pearson correlation, stdlib only — no sox/numpy) `0ec566b`
- [x] Exit 0 if score > 0.9, exit 1 otherwise (threshold is configurable, defaults to 0.9) `0ec566b`
- [x] Standalone validation script `dev/scripts/test-compare-audio.sh` — tests self-correlation (~1.0), silence (~0.0), and opus round-trip (>0.9)

**Test**: Compare a file with itself → score ~1.0. Compare with silence → score ~0.0. Compare with a slightly compressed version → score > 0.9.

> **Resolved**: All three acceptance criteria are now independently validated by `test-compare-audio.sh`. Self-correlation: 1.000000, silence: 0.000000, opus round-trip: 0.999980.

### 9.2 Test Fixtures
- [x] `test/fixtures/test-tone-1khz-5s.wav` — 5-second 1kHz sine wave, 48kHz mono PCM s16le `0ec566b`
- [x] `test/fixtures/test-speech-5s.mp3` — 5-second speech sample (converted from WAV to MP3 per spec)

> **Resolved**: Speech fixture converted to `.mp3` format matching the spec. Both fixtures exist and are valid audio files.

- [x] Generate test tone: `ffmpeg -f lavfi -i "sine=frequency=1000:duration=5" -ar 48000 test-tone-1khz-5s.wav` — tone file is present and valid `0ec566b`

**Test**: Fixtures exist and are valid audio files (ffprobe succeeds).

### 9.3 Golden Test: Browser → K2B
- [x] `dev/scripts/run-e2e-tests.sh` orchestrates the full WebRTC audio path:
  1. Build and deploy ct-server and ct-client
  2. Create session from template via REST API
  3. Start ct-client on K2B, join session as "studio" role
  4. Playwright opens session connect view, connects as "translator" role
  5. Playwright injects test tone via `--use-file-for-fake-audio-capture` (Chromium flag)
  6. Audio flows: Browser → WebRTC → ct-server SFU → WebRTC → ct-client → PipeWire
  7. K2B captures audio from PipeWire sink monitor
  8. Run `compare-audio.sh` against reference → score > 0.9

- [x] `test/playwright/specs/golden-audio.spec.ts` — Playwright browser-side test

**Test**: `task test:e2e` includes this test. Passes if cross-correlation > 0.9.

> **Resolved**: Rewritten to test the actual WebRTC audio path using Playwright + ct-client, not ALSA loopback hardware. Threshold raised to 0.90.

### 9.4 Golden Test: K2B → Browser
- [x] Same script orchestrates the reverse path:
  1. Create session, connect both clients (ct-client as studio, Playwright as translator)
  2. Play test tone into K2B PipeWire source via ffmpeg (`ffmpeg -re -i test-tone.wav -f pulse default`)
  3. ct-client sends audio via WebRTC to server, server forwards to browser
  4. Playwright captures browser audio output using Web Audio API MediaRecorder
  5. Save captured audio to file
  6. Run `compare-audio.sh` against reference → score > 0.9

**Test**: `task test:e2e` includes this test. Passes if cross-correlation > 0.9.

> **Resolved**: Rewritten to test the actual WebRTC audio path. Playwright captures received audio via MediaRecorder on the WebRTC remote stream.

### 9.5 test:e2e Task
- [x] `task test:e2e` runs `dev/scripts/run-e2e-tests.sh` `b24b487`
- [x] Requires: dev environment running + K2B board on network (script checks SSH, PipeWire, etc.) `b24b487`
- [x] Reports pass/fail with correlation scores for each direction
- [x] Infrastructure verification: server API, binary validity, PipeWire nodes, ct-client connectivity, compare-audio self-test, WebRTC signaling detection

**Test**: `task test:e2e` exits 0 when both golden tests pass.

## Exit Criteria

`task test:e2e` passes both golden tests:

1. **Browser → K2B**: 1kHz tone sent from browser arrives at K2B PipeWire loopback sink. Cross-correlation with reference > 0.9.
2. **K2B → Browser**: 1kHz tone played into K2B PipeWire loopback source arrives at browser. Cross-correlation with reference > 0.9.

This proves real audio flows through the full system: browser → WebRTC → server (pure forwarding) → WebRTC → CLI client → PipeWire, and vice versa.

> **Status**: Test infrastructure is now correct — golden tests exercise the full WebRTC audio pipeline via Playwright + ct-client, with 0.90 correlation threshold. Actual pass/fail depends on having a K2B board on the network and all components working end-to-end.

## Spec Updates

- 7.3 E2E / Golden Tests → 4 (tooling, fixtures, and test structure complete; needs hardware validation)
- 2.2 WebRTC Signaling → 7 (no change from previous — not directly validated by this phase yet)
- 2.3 Session Orchestration → 8 (no change — session API works, tested in earlier phases)
- 3.2 PipeWire Integration → 5 (infrastructure verified, full path untested without hardware)
- 3.3 Channel Lifecycle → 5 (infrastructure verified, full path untested without hardware)

## Fix Review (2025-04-22)

**Reviewer**: Hermes Agent (code-only review — no K2B hardware)  
**Commit**: `d195bae` on `fix-p9`  
**Verdict**: **APPROVED** ✅

### Gap Assessment

| Gap | Issue | Status | Evidence |
|-----|-------|--------|----------|
| G1 | Speech fixture was .wav instead of .mp3 | ✅ FIXED | `test-speech-5s.wav` deleted, `test-speech-5s.mp3` added (40557 bytes, MPEG ADTS layer III, 64kbps 48kHz mono) |
| G2 | No standalone compare-audio tests | ✅ FIXED | `dev/scripts/test-compare-audio.sh` (105 lines): self-corr=1.000000, silence=0.000000, opus-roundtrip=0.999980 — all 3/3 pass |
| G3 | run-e2e-tests.sh tested ALSA loopback, not full WebRTC path | ✅ FIXED | Rewritten (496 lines): builds + deploys server & client, creates template/session/token via REST API, ct-client on K2B as "studio", Playwright as "translator", full Browser→WebRTC→SFU→WebRTC→K2B path and reverse. No ALSA loopback patterns remain. |
| G4 | Threshold 0.60 instead of 0.90 | ✅ FIXED | `E2E_THRESHOLD` defaults to `0.90` (line 37). `compare-audio.sh` also defaults to 0.9. No 0.60 values anywhere in codebase. |
| G5 | Spec scores unrealistic | ✅ FIXED | Spec scores now conservative: E2E/Golden→4, PipeWire→5, Channel Lifecycle→5 — all note "untested without hardware". |

### Additional Observations

- `playwright.config.ts` correctly wires Chromium flags for fake audio (`--use-fake-ui-for-media-stream`, `--use-fake-device-for-media-stream`, conditional `--use-file-for-fake-audio-capture`)
- `golden-audio.spec.ts` properly handles both directions: Browser→K2B (fake mic, wait for flow) and K2B→Browser (MediaRecorder capture from remote stream, base64 extraction)
- Cleanup in `run-e2e-tests.sh` is thorough (local PIDs, remote pkill, temp dir)
- Tests are gated by `CT_SESSION_ID` env var — skipped gracefully outside full E2E context

## What's Next

After this phase, the core audio pipeline is proven. Remaining work:
- Broadcast client implementation
- Session recording polish (playback, management UI)
- Error recovery hardening
- Production deployment
- Monitoring / observability
