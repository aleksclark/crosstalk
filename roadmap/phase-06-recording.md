# Phase 6: Server-Side Recording

[← Roadmap](index.md)

**Status**: `NOT STARTED`  
**Depends on**: Phase 5 (audio forwarding must work)

Capture audio from `→ record` mappings to OGG/Opus files on disk.

## Tasks

### 6.1 OGG/Opus Writer
- [ ] Accept Opus RTP packets, write them into an OGG container
- [ ] File naming: `<session-id>/<role>-<channel>-<timestamp>.ogg`
- [ ] Handle start/stop lifecycle: create file when binding activates, finalize on deactivate or session end

**Test**: Feed known Opus packets into the writer, verify output file is valid OGG/Opus via `ffprobe` (exit code 0, codec = opus).

### 6.2 Recording Integration
- [ ] When a `→ record` binding activates, start writing received RTP to a new OGG file
- [ ] When binding deactivates or session ends, finalize the file
- [ ] Write `session-meta.json` with template, roles, timing, file manifest
- [ ] Surface recording status in `GET /api/sessions/:id` response
- [ ] Emit `SESSION_RECORDING_STARTED` / `SESSION_RECORDING_STOPPED` events on control channel

**Test**: Create session with `→ record` mapping. Client sends 5 seconds of audio. End session. Verify: file exists, `ffprobe` shows ~5s duration (±1s), `session-meta.json` is valid JSON with correct fields.

### 6.3 Error Handling
- [ ] Disk full → log error, emit session event, stop recording (don't crash server)
- [ ] Recording directory not writable → log error at session start, skip recording

**Test**: Set `recording_path` to a read-only directory. Create session with `→ record`. Verify server logs error but continues running, session works without recording.

## Exit Criteria

1. Session with `→ record` mapping produces valid OGG/Opus files
2. `ffprobe` confirms codec and approximate duration
3. `session-meta.json` written with correct metadata
4. Recording errors don't crash the server

## Spec Updates

- 2.4 Recording → 4
