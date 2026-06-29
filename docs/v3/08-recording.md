# Step 08: Per-Source + Per-Channel Recording

## Goal

Record all audio with timestamps. Every source gets its own recording file, and every channel output gets recorded too. This enables post-hoc remixing and archival.

## Recording Strategy

1. **Per-source recording**: Every source's raw audio (pre-mix) is recorded individually
2. **Per-channel recording**: Each channel's mixed output is recorded
3. **Timestamps**: Recordings include start time relative to session; RTP timestamps provide sample-accurate timing
4. **Format**: OGG/Opus (same as v2) — efficient, good quality, widely supported

## File Layout

```
recordings/
  {session_id}/
    sources/
      {source_id}_{source_name}_{timestamp}.ogg    # Individual source
    channels/
      {channel_id}_{channel_name}_{timestamp}.ogg  # Mixed channel output
    metadata.json                                    # Recording manifest
```

### metadata.json
```json
{
  "session_id": "01ABC...",
  "session_name": "10am Sunday Service",
  "started_at": "2025-01-05T10:00:00Z",
  "recordings": [
    {
      "id": "01DEF...",
      "type": "source",
      "source_id": "01GHI...",
      "source_name": "Booth A",
      "file": "sources/01GHI_booth-a_1704452400.ogg",
      "started_at": "2025-01-05T10:00:05Z",
      "ended_at": "2025-01-05T11:15:30Z",
      "size_bytes": 15234567
    }
  ]
}
```

## Tasks

### 8.1 Recording service
- [ ] `recording/` package — manages recording lifecycle
- [ ] Start recording when source connects (auto-record all sources)
- [ ] Start channel recording when channel has at least one active source
- [ ] Stop recording on source disconnect or session end
- [ ] Handle reconnection: new file with new timestamp (don't append to old file)

### 8.2 Source recording
- [ ] Tap into source's decoded RTP stream (before mixer input)
- [ ] Write to OGG/Opus file using v2's Recorder pattern
- [ ] File naming: `{source_id}_{sanitized_name}_{unix_timestamp}.ogg`
- [ ] Track in `recordings` table

### 8.3 Channel recording
- [ ] Tap into mixer output (after mix, before encoding for WebRTC)
- [ ] Write mixed PCM to OGG/Opus file
- [ ] File naming: `{channel_id}_{sanitized_name}_{unix_timestamp}.ogg`
- [ ] Track in `recordings` table

### 8.4 Recording API
- [ ] `GET /api/sessions/{id}/recordings` — list all recordings for a session
- [ ] `GET /api/recordings/{id}/download` — download a specific recording file
- [ ] Recording status in session detail response
- [ ] Auto-cleanup: configurable retention period

### 8.5 Recording manifest
- [ ] Generate `metadata.json` when session ends or on demand
- [ ] Include timing offsets so recordings can be aligned for remixing
- [ ] Update manifest as new recordings are added/completed

## Acceptance Criteria

- [ ] When an ABC connects and sends audio, a source recording file is created
- [ ] When a translator connects and sends audio, a source recording file is created
- [ ] Channel recording captures the mixed output
- [ ] Recordings are valid OGG/Opus (ffprobe confirms codec + duration)
- [ ] Source disconnection creates a clean file boundary (no corruption)
- [ ] `GET /api/sessions/{id}/recordings` lists all recordings with metadata
- [ ] Files can be downloaded via API

## Reusable from v2

- `server/pion/recording.go` — Recorder struct (OGG writer, thread-safe WriteRTP, Close)
- Recording path configuration from `server/config.go`
- File organization pattern from orchestrator's recording setup
