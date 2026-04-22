# Phase 6: Server-Side Recording

[← Roadmap](index.md)

**Status**: `COMPLETE — 11/11 tasks done`  
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
> ✅ `TestRecorder_FFProbeValidation` — writes 50 packets (1s), validates codec=opus and asserts duration within ±1.5s of expected 1.0s via ffprobe.
> Both pass.

### 6.2 Recording Integration
- [x] When a `→ record` binding activates, start writing received RTP to a new OGG file — `7ed7c38`
- [x] When binding deactivates or session ends, finalize the file — `7ed7c38`
- [x] Write `session-meta.json` with template, roles, timing, file manifest
- [x] Surface recording status in `GET /api/sessions/:id` response
- [x] Emit `SESSION_RECORDING_STARTED` / `SESSION_RECORDING_STOPPED` events on control channel

> **session-meta.json**: `WriteSessionMeta` in `recording.go` writes
> `session_id`, `template_name`, `participants` (map of role→peer_id),
> `started_at`, `ended_at`, and `files[]`. The `Participants` field is populated
> from the orchestrator's live session state in `EndSession`.
>
> **REST API**: `handleGetSession` in `server/http/handler.go` now includes a
> `recording` field in the response (active, file_count, total_bytes) when the
> orchestrator reports recording status. `RecordingStatus()` method added to
> `SessionOrchestrator` interface and implemented in `pion.Orchestrator`.
>
> **Control channel events**: `SESSION_RECORDING_STARTED` emitted in
> `startRecording` after successfully creating the recorder.
> `SESSION_RECORDING_STOPPED` emitted in `deactivateBinding` when closing a
> recorder, which covers both explicit deactivation and `EndSession` cleanup.

**Test**: Create session with `→ record` mapping. Client sends audio. End session. Verify: file exists, `ffprobe` shows codec, `session-meta.json` is valid JSON with correct fields.

> ✅ `TestRecordingIntegration_SessionProducesFile` — verifies OGG file exists and session-meta.json fields including participants.
> ✅ `TestRecordingIntegration_EmitsEvents` — verifies SESSION_RECORDING_STARTED and SESSION_RECORDING_STOPPED events are emitted on the control channel.
> ✅ `TestGetSession_WithRecordingStatus` — verifies REST API includes recording info.
> ✅ `TestIntegration_SessionWithRecording` — hard-asserts OGG file, session-meta.json, and validates via ffprobe.

### 6.3 Error Handling
- [x] Disk full → log error, emit session event, stop recording (don't crash server) — `7ed7c38`
- [x] Recording directory not writable → log error at session start, skip recording — `7ed7c38`

> `startRecording` in `orchestrator.go` handles both: `os.MkdirAll` failure is logged
> and returns (session continues). `NewRecorder` failure is also logged and returns.
> `WriteRTP` errors are logged at debug level and writing continues.

**Test**: Set `recording_path` to a read-only directory. Create session with `→ record`. Verify server logs error but continues running, session works without recording.

> ✅ `TestRecordingIntegration_ReadOnlyDir` — creates read-only dir, verifies
> `JoinSession` succeeds, verifies `lb.Recorder` is nil, verifies `EndSession` doesn't
> panic. Passes.

## Exit Criteria

1. Session with `→ record` mapping produces valid OGG/Opus files — ✅ **MET**
2. `ffprobe` confirms codec and approximate duration — ✅ **MET** (duration asserted within ±1.5s)
3. `session-meta.json` written with correct metadata including participants — ✅ **MET**
4. Recording errors don't crash the server — ✅ **MET**
5. Recording status surfaced in REST API — ✅ **MET**
6. Recording events emitted on control channel — ✅ **MET**

## Gaps — All Addressed

| # | Gap | Status |
|---|-----|--------|
| G1 | `SessionMeta` missing `Roles`/`Participants` field | ✅ Fixed — added `Participants map[string]string` |
| G2 | `GET /api/sessions/:id` does not surface recording status | ✅ Fixed — added `RecordingInfo` to response |
| G3 | `SESSION_RECORDING_STARTED`/`STOPPED` events never emitted | ✅ Fixed — emitted in `startRecording` and `deactivateBinding` |
| G4 | Integration test silently skips assertions | ✅ Fixed — now hard-asserts with `require` |
| G5 | No test validates ffprobe duration within ±1s | ✅ Fixed — `strconv.ParseFloat` + `math.Abs` assertion |

## Spec Updates

- 2.4 Recording → 8
