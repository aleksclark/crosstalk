# Logging

[← Back to Index](../index.md) · [Server Overview](overview.md)

---

## Overview

All components use **structured JSON logging**. Every log line is a single JSON object with a consistent schema. Clients log to their local output **and** stream logs back to the server over the WebRTC control data channel.

## Log Format

Every log entry is a JSON object:

```json
{
  "ts": "2026-04-21T10:30:01.123Z",
  "level": "info",
  "msg": "channel bound",
  "component": "webrtc",
  "channel_id": "ch_abc123",
  "track_id": "trk_xyz",
  "role": "translator"
}
```

### Standard Fields

| Field | Type | Description |
|-------|------|-------------|
| `ts` | string (ISO 8601) | Timestamp with millisecond precision |
| `level` | string | `debug`, `info`, `warn`, `error` |
| `msg` | string | Human-readable message |
| `component` | string | Subsystem that emitted the log (e.g., `webrtc`, `session`, `auth`, `pipewire`) |

Additional fields are added as structured context — never concatenated into the message string.

## Output Destinations

### Server

| Destination | Format | When |
|---|---|---|
| **stdout** | JSON, one object per line | Always |

The server is the log aggregation point. It receives logs from clients over the control data channel and can fan them out to the admin web UI's session connect view.

### CLI Client

| Destination | Format | When |
|---|---|---|
| **stdout** | JSON, one object per line | Always |
| **Server (control channel)** | Protobuf `LogEntry` | When WebRTC connected |

The CLI client always logs to stdout (for systemd journal / local debugging). When a WebRTC connection is active, it also streams logs to the server as Protobuf `LogEntry` messages on the control data channel. If the connection drops, logs continue to stdout only — no buffering or retry for log delivery.

### Admin Web UI

| Destination | Format | When |
|---|---|---|
| **Browser console** | `console.log/warn/error` with structured data | Always |
| **Server (control channel)** | Protobuf `LogEntry` | When WebRTC connected |

Same dual-output as the CLI client: browser console for local debugging, control channel for server aggregation.

## Log Levels

| Level | Usage |
|-------|-------|
| `debug` | Detailed internal state, WebRTC stats, PipeWire node enumeration |
| `info` | Normal operations: connections, bindings, session lifecycle |
| `warn` | Recoverable issues: config validation warnings, reconnection attempts, codec fallback |
| `error` | Failures: auth rejected, channel bind failed, recording write error |

Default level: `info`. Configurable via `log_level` in the JSON config file.

## Session Log Streaming

The server aggregates logs from all clients in a session and streams them to the admin web UI's session connect view:

```
CLI Client A ──► control channel ──► Server ──► control channel ──► Admin Web UI
CLI Client B ──► control channel ──┘                                    │
Server internal logs ──────────────────────────────────────────────────┘
```

- Server attaches source metadata (client ID, role) to forwarded logs
- Admin UI can filter by source, severity, and component
- Only session-relevant logs are forwarded (not all server logs)

## Library Choices

- **Server + CLI**: `log/slog` (Go stdlib, structured, JSON handler) — no external dependency
- **Web UI**: A thin wrapper around `console.*` that formats structured JSON and sends to the control channel
