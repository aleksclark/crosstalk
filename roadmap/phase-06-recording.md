# Phase 6: Server-Side Recording

[← Roadmap](index.md)

**Status**: `PARTIAL — 7/11 tasks done`  
**Depends on**: Phase 5 (audio forwarding must work)

Capture audio from `→ record` mappings to OGG/Opus files on disk.

## Tasks

### 6.1 OGG/Opus Writer
- [x] Accept Opus RTP packets, write them into an OGG container — `7ed7c38`
- [x] File naming: `<session-id>/<role>-<channel>-<timestamp>.ogg` — `7ed7c38`
- [x] Handle start/stop lifecycle: create file when binding activates, finalize on deactivate or session end — `7ed7c38`

> `Recorder` type in `server/pion/recording.go` wraps `pion/webrtc/oggwriter`.
> `RecordingFileName()` generates `<role>-<channel>-<timestamp>.ogg`; session-id is
> the parent directory, created by `startRecording` in `orchestrator.go:399`.
> Close is called in `deactivateBinding` and `EndSession`.

**Test**: Feed known Opus packets into the writer, verify output file is valid OGG/Opus via `ffprobe` (exit code 0, codec = opus).

> ✅ `TestRecorder_WriteAndClose` — writes 10 Opus RTP packets, verifies file non-empty.
> ✅ `TestRecorder_FFProbeValidation` — writes 50 packets (1s), validates codec=opus and duration via ffprobe.
> Both pass.

### 6.2 Recording Integration
- [x] When a `→ record` binding activates, start writing received RTP to a new OGG file — `7ed7c38`
- [x] When binding deactivates or session ends, finalize the file — `7ed7c38`
- [ ] Write `session-meta.json` with template, roles, timing, file manifest
- [ ] Surface recording status in `GET /api/sessions/:id` response
- [ ] Emit `SESSION_RECORDING_STARTED` / `SESSION_RECORDING_STOPPED` events on control channel

> **session-meta.json (PARTIAL)**: `WriteSessionMeta` in `recording.go:90` writes
> `session_id`, `template_name`, `started_at`, `ended_at`, and `files[]`. However the
> spec (`spec/server/recording.md`) and roadmap require "roles filled" / participant
> info — the `SessionMeta` struct has no `Roles` or `Participants` field. Marking
> incomplete.
>
> **REST API (GAP)**: `handleGetSession` in `server/http/handler.go:663` returns only
> basic session fields (`id`, `template_id`, `name`, `status`, `created_at`,
> `ended_at`). No recording status, bytes written, or duration is surfaced. The spec
> (`spec/server/recording.md:43-44`) explicitly requires "Recording status is visible
> in session detail via REST API" and "Active recordings report bytes written, duration,
> and any errors."
>
> **Control channel events (GAP)**: `SessionEventType_SESSION_RECORDING_STARTED` (=5)
> and `SESSION_RECORDING_STOPPED` (=6) are defined in `proto/crosstalk/v1/control.proto`
> and generated in `control.pb.go`, but `sendSessionEvent` is never called with these
> types anywhere in the codebase. The `startRecording` function in `orchestrator.go:391`
> logs the start but does not emit the protobuf event. Similarly, `deactivateBinding`
> closes the recorder but emits no event.

**Test**: Create session with `→ record` mapping. Client sends 5 seconds of audio. End session. Verify: file exists, `ffprobe` shows ~5s duration (±1s), `session-meta.json` is valid JSON with correct fields.

> ⚠️ `TestRecordingIntegration_SessionProducesFile` — **passes** but only sends ~400ms
> of audio (20 packets × 20ms), not 5s as specified. Validates OGG file exists and
> `session-meta.json` fields, but does not validate duration via ffprobe.
>
> ⚠️ `TestIntegration_SessionWithRecording` (integration_test.go:672) — sends ~2s of
> audio, but the test **returns early with a log message** (no failure) if the recording
> directory doesn't exist (line 787-789). This means the core acceptance criteria (file
> exists, ffprobe validates codec/duration, session-meta.json has correct fields) can be
> silently skipped in test environments where OnTrack doesn't fire. The test does not
> hard-assert on recording output.
>
> Neither test validates that `session-meta.json` contains "roles filled" data (because
> the struct doesn't have that field).

### 6.3 Error Handling
- [x] Disk full → log error, emit session event, stop recording (don't crash server) — `7ed7c38`
- [x] Recording directory not writable → log error at session start, skip recording — `7ed7c38`

> `startRecording` in `orchestrator.go:391` handles both: `os.MkdirAll` failure is logged
> and returns (session continues). `NewRecorder` failure is also logged and returns.
> `WriteRTP` errors are logged at debug level and writing continues.
>
> Note: the roadmap says "emit session event" for disk-full, but no
> `SESSION_RECORDING_STOPPED` event is emitted — only a log entry. This is consistent
> with the control-channel events gap in 6.2.

**Test**: Set `recording_path` to a read-only directory. Create session with `→ record`. Verify server logs error but continues running, session works without recording.

> ✅ `TestRecordingIntegration_ReadOnlyDir` — creates read-only dir, verifies
> `JoinSession` succeeds, verifies `lb.Recorder` is nil, verifies `EndSession` doesn't
> panic. Passes.

## Exit Criteria

1. Session with `→ record` mapping produces valid OGG/Opus files — ✅ **MET** (unit test proves it; integration test is soft)
2. `ffprobe` confirms codec and approximate duration — ⚠️ **PARTIAL** (`TestRecorder_FFProbeValidation` confirms codec; duration is checked as non-empty but not asserted to ±1s of expected value)
3. `session-meta.json` written with correct metadata — ⚠️ **PARTIAL** (written and tested, but missing "roles filled" field per spec)
4. Recording errors don't crash the server — ✅ **MET** (read-only dir test proves it)

## Remaining Gaps

| # | Gap | Effort |
|---|-----|--------|
| G1 | `SessionMeta` missing `Roles`/`Participants` field (spec says "roles filled") | S |
| G2 | `GET /api/sessions/:id` does not surface recording status, bytes, duration | M |
| G3 | `SESSION_RECORDING_STARTED`/`STOPPED` events never emitted on control channel | S |
| G4 | Integration test `TestIntegration_SessionWithRecording` silently skips assertions when recording dir missing — should hard-fail or be restructured | S |
| G5 | No test validates ffprobe duration is within ±1s of expected value | S |

## Spec Updates

- 2.4 Recording → 4
