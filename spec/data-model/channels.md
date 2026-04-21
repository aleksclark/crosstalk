# Channels

[← Back to Index](../index.md) · [Data Model Overview](overview.md)

---

## Channel Properties

Every channel has three defining attributes:

| Property | Values | Description |
|----------|--------|-------------|
| **Type** | `audio`, `video`, `logstream` | What kind of data flows through it |
| **Direction** | `source`, `sink` | From the client's perspective — source sends, sink receives |
| **Name** | string | Human-readable identifier (e.g., `mic`, `speakers`, `output`) |

## Channel Types

### Audio

Primary focus of the initial implementation. Carries real-time audio over WebRTC media tracks. Codec is negotiated via standard WebRTC SDP (typically Opus).

### Video

Future extension. Same architecture as audio but with video media tracks. Not in scope for the initial build.

### Logstream

Structured log messages carried over the WebRTC data channel (not media tracks). Defined in Protobuf with fields:

| Field | Type | Description |
|-------|------|-------------|
| timestamp | int64 | Unix timestamp (milliseconds) |
| severity | enum | DEBUG, INFO, WARN, ERROR |
| message | string | Log message text |
| source | string | Originating client or "server" |

Used for streaming session-level logs to the admin web UI's session connect view.

## The `control` Channel

A special data channel established on every WebRTC connection:

- **Name**: `control`
- **Type**: data channel (reliable, ordered)
- **Purpose**: server ↔ client command/status protocol
- **Not used in sessions** — it exists for the management plane, independent of session channel mappings
- **Always present** — created immediately after WebRTC connection

The control channel carries Protobuf-encoded messages for:
- Capability reporting (client → server)
- Channel bind/unbind commands (server → client)
- Status updates (client → server)
- Session log streaming (server → client, for admin UI)

> See [Protobuf Schema](protobuf.md) for message definitions.

## Channel Naming Convention

In template mappings, channels are referenced as `role:channel_name`:
- `translator:mic` — the "mic" channel on the client filling the "translator" role
- `studio:output` — the "output" channel on the client filling the "studio" role

The `channel_name` in a mapping must match what the client reports as its source/sink name (from PipeWire, or overridden via env var).

## Broadcast

`broadcast` is a special sink in template mappings. It is not a channel on any specific client — instead, the server forwards the source track to **all connected broadcast clients**.

Broadcast clients:
- Connect via `/ws/broadcast` WebSocket endpoint with **no authentication**
- Receive listen-only audio tracks — they cannot send
- Are not assigned to any role and do not appear in session client lists
- Will be implemented as a separate lightweight client component (future)

> See [Session Templates](session-templates.md) for mapping syntax and validation rules.
