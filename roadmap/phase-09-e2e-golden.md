# Phase 9: E2E Golden Audio Tests

[← Roadmap](index.md)

**Status**: `PARTIAL — 2/5 tasks done, 3 have significant gaps`  
**Depends on**: Phase 8 (integration tests) + K2B board on network

Real audio through real hardware. This is the final acceptance gate.

## Tasks

### 9.1 Audio Comparison Tool
- [x] `dev/scripts/compare-audio.sh` — takes two audio files, outputs cross-correlation score (0.0–1.0) `0ec566b`
- [x] Normalize both files to same sample rate (16kHz mono) via ffmpeg `0ec566b`
- [x] Cross-correlation via ~~sox or~~ custom ~~Go~~ Python tool (uses Pearson correlation, stdlib only — no sox/numpy) `0ec566b`
- [x] Exit 0 if score > 0.9, exit 1 otherwise (threshold is configurable, defaults to 0.9) `0ec566b`

**Test**: Compare a file with itself → score ~1.0. Compare with silence → score ~0.0. Compare with a slightly compressed version → score > 0.9.

> **Review**: The self-test (file vs itself) is embedded in `run-e2e-tests.sh` (test 9.5e) and works. However, there is **no standalone test script** that validates the three acceptance criteria independently (self-correlation, silence comparison, compressed-version comparison). The silence and compressed-version tests are missing entirely. The script itself is well-implemented and correct.

### 9.2 Test Fixtures
- [x] `test/fixtures/test-tone-1khz-5s.wav` — 5-second 1kHz sine wave, 48kHz mono PCM s16le `0ec566b`
- [ ] `test/fixtures/test-speech-5s.mp3` — 5-second speech sample

> **Gap**: The speech fixture exists as `test-speech-5s.wav` (not `.mp3` as specified). The files are distinct (different MD5 hashes) and both are valid WAV audio (48kHz, mono, 16-bit PCM, 5.00s duration). However, the spec and the `spec/testing/e2e-golden.md` fixture list both call for `.mp3` format — the current file is `.wav`. This is a minor format mismatch.

- [x] Generate test tone: `ffmpeg -f lavfi -i "sine=frequency=1000:duration=5" -ar 48000 test-tone-1khz-5s.wav` — tone file is present and valid `0ec566b`

**Test**: Fixtures exist and are valid audio files (ffprobe succeeds).

> **Review**: Both fixtures exist and pass `ffprobe` validation. No automated test exists that checks this — the closest is the prerequisite check in `run-e2e-tests.sh` (`[[ -f "$REF_TONE" ]]`), but that only checks the tone file exists, not that `ffprobe` succeeds, and doesn't check the speech file at all.

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

> **GAP — Major divergence from spec**: The `run-e2e-tests.sh` script (`b24b487`) does NOT implement the specified Browser→K2B golden test. Instead of using **Playwright to inject audio through a browser WebRTC connection**, it tests **ALSA loopback wiring on the K2B board** by injecting a tone directly into `plughw:Loopback,0,0` and capturing from `plughw:Loopback,1,0`. This proves the ALSA loopback kernel module works, but does **not** prove audio flows through the full path: Browser → WebRTC → Server → WebRTC → CLI client → PipeWire.
>
> Specific gaps:
> - **No Playwright involvement** — no browser connects to the session at all.
> - **No WebRTC audio path tested** — audio never traverses the server.
> - **ct-client joins but is not tested as a conduit** — the script starts ct-client on K2B and verifies it connects, but the audio test bypasses it entirely.
> - **Threshold is 0.60, not 0.90** — the script uses `E2E_THRESHOLD=0.60` by default, below the 0.9 specified in the roadmap.
> - Steps 4 and 5 (Playwright opens session, injects mic audio) are completely missing.

### 9.4 Golden Test: K2B → Browser
- [ ] Same script orchestrates:
  1. Create session, connect both clients
  2. Play test tone into K2B loopback source via ffmpeg
  3. Playwright captures browser audio output (Web Audio API recorder)
  4. Wait 6 seconds
  5. Retrieve captured audio from Playwright
  6. Run `compare-audio.sh` against reference → score > 0.9

**Test**: `task test:e2e` includes this test. Passes if cross-correlation > 0.9.

> **GAP — Same divergence as 9.3**: The script tests the **reverse ALSA loopback path** on K2B (inject into `plughw:Loopback,1,0`, capture from `plughw:Loopback,0,0`) rather than the full K2B→WebRTC→Server→WebRTC→Browser path.
>
> Specific gaps:
> - **No Playwright involvement** — no browser captures audio.
> - **No WebRTC audio path tested** — audio stays entirely on the K2B board.
> - **No Web Audio API recorder** — steps 3 and 5 are completely missing.
> - **Threshold is 0.60, not 0.90**.
> - The test essentially verifies the same ALSA loopback hardware as 9.3, just in the opposite direction.

### 9.5 test:e2e Task
- [x] `task test:e2e` runs `dev/scripts/run-e2e-tests.sh` `b24b487`
- [x] Requires: dev environment running + K2B board on network (script checks SSH, PipeWire, etc.) `b24b487`
- [ ] Reports pass/fail with correlation scores for each direction

> **Review**: The task definition exists in `Taskfile.yml:163` and invokes the script correctly. The script does report correlation scores and pass/fail per test. However, since the underlying golden tests (9.3, 9.4) test ALSA loopback rather than the full WebRTC audio path, the "pass" result does not validate the actual exit criteria. Infrastructure verification (9.5a–9.5f) is solid: checks server API, binary validity, PipeWire loopback nodes, ct-client connectivity, compare-audio self-test, and WebRTC signaling evidence in logs.

**Test**: `task test:e2e` exits 0 when both golden tests pass.

> **Review**: The script exits 0 when all sub-tests pass and exits 1 on any failure. The mechanics are correct, but the golden tests don't test what the exit criteria require.

## Exit Criteria

`task test:e2e` passes both golden tests:

1. **Browser → K2B**: 1kHz tone sent from browser arrives at K2B PipeWire loopback sink. Cross-correlation with reference > 0.9.
2. **K2B → Browser**: 1kHz tone played into K2B PipeWire loopback source arrives at browser. Cross-correlation with reference > 0.9.

This proves real audio flows through the full system: browser → WebRTC → server (pure forwarding) → WebRTC → CLI client → PipeWire, and vice versa.

> **EXIT CRITERIA NOT MET**: The current implementation tests ALSA loopback hardware integrity on the K2B board, not the full WebRTC audio pipeline. To meet exit criteria, the tests need:
> 1. Playwright-based browser client that connects to a session and injects/captures audio via WebRTC.
> 2. Audio routed through the actual server (WebRTC SFU/forwarding).
> 3. ct-client acting as the real conduit between WebRTC and PipeWire/ALSA.
> 4. Cross-correlation threshold raised to 0.9.
>
> **What works today**: The ALSA loopback tests prove the K2B board's audio plumbing is correctly wired (loopback kernel module, subdevice pairing). The infrastructure verification proves the build/deploy pipeline, server API, and ct-client deployment all function. These are valuable building blocks but not the final acceptance gate.
>
> **Pre-existing test failure**: `cli/pion.TestConnection_ConnectAndSendHello` fails with a timeout waiting for the data channel to open, suggesting the WebRTC control channel handshake has an issue that would need to be resolved before full E2E audio can work.

## Spec Updates

- 7.3 E2E / Golden Tests → 7
- 2.2 WebRTC Signaling → 7
- 2.3 Session Orchestration → 8
- 3.2 PipeWire Integration → 7
- 3.3 Channel Lifecycle → 7

> **Review**: These score bumps are not yet justified. The E2E golden tests don't exercise WebRTC signaling, session orchestration, or PipeWire integration through the full path. Recommended scores based on current evidence:
> - 7.3 E2E / Golden Tests → **2** (tooling and fixtures exist, but golden tests don't validate the described behavior)
> - 2.2, 2.3, 3.2, 3.3 — scores should not increase from this phase since 9.3/9.4 don't exercise them

## What's Next

After this phase, the core audio pipeline is proven. Remaining work:
- Broadcast client implementation
- Session recording polish (playback, management UI)
- Error recovery hardening
- Production deployment
- Monitoring / observability
