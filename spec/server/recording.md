# Recording

[← Back to Index](../index.md) · [Server Overview](overview.md)

---

## Overview

The server records audio streams as directed by session template mappings (any mapping targeting `record` as the sink).

## Behavior

- Recording starts when the source client's media track becomes active
- Recording stops when the session ends or the source disconnects
- Each recorded channel produces a separate file
- Files are named with session ID, role, channel name, and timestamp

## File Format

**OGG/Opus** — WebRTC audio is typically Opus-encoded, so the server can write the Opus frames directly into an OGG container with minimal transcoding overhead.

If the source uses a different codec, the server should either:
1. Transcode to Opus (preferred for consistency), or
2. Write raw RTP packets and note the codec in metadata

## Storage

```
data/
└── recordings/
    └── <session-id>/
        ├── translator-mic-2026-04-21T10-30-00.ogg
        ├── studio-input-2026-04-21T10-30-00.ogg
        └── session-meta.json   # session info, start/end times, participants
```

- Storage path configurable in server config
- `session-meta.json` includes: template used, roles filled, start/end time, file manifest
- No automatic cleanup — admin manages storage externally (future: configurable retention)

## Monitoring

- Recording status is visible in session detail via REST API
- Active recordings report bytes written, duration, and any errors
- Recording errors (disk full, write failure) are surfaced as session events on the control data channel
