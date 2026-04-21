# WebRTC Signaling

[← Back to Index](../index.md) · [Server Overview](overview.md)

---

## Overview

All WebRTC operations use [Pion](https://github.com/pion/webrtc). The server acts as a pure forwarder (SFU) — every client connects to the server, never directly to another client. The server forwards media tracks without mixing or processing.

## Connection Lifecycle

```
Client                              Server
  │                                    │
  ├── POST /api/webrtc/token ─────────►│  (authenticated via REST)
  │◄── { token: "abc123" } ───────────┤
  │                                    │
  ├── WS /ws/signaling ──────────────►│  (WebSocket, auth token in query)
  │◄── connected ─────────────────────┤
  │                                    │
  ├── { type: "offer", sdp: "..." } ──►│  (SDP offer over WebSocket)
  │    validate token                  │
  │    create PeerConnection           │
  │◄── { type: "answer", sdp: "..." } ┤  (SDP answer over WebSocket)
  │                                    │
  ├── { type: "ice", candidate: ... } ►│  (ICE trickle, bidirectional)
  │◄── { type: "ice", candidate: ... } ┤
  │                                    │
  │◄═══════ WebRTC Connected ═════════►│
  │                                    │
  │  ┌─ control data channel ─────────►│  (client status, capabilities)
  │  │◄── server commands ────────────┤  (bind channels, etc.)
  │  │                                │
  │  ├─ audio track(s) ──────────────►│  (source streams)
  │  │◄── audio track(s) ────────────┤  (sink streams)
  │  └────────────────────────────────┘
```

## Broadcast Client Connection

Broadcast clients use a separate, **unauthenticated** WebSocket:

```
Broadcast Client                     Server
  │                                    │
  ├── WS /ws/broadcast?session=ID ────►│  (no auth required)
  │◄── connected ─────────────────────┤
  │                                    │
  │◄── { type: "offer", sdp: "..." } ─┤  (server sends offer, listen-only)
  ├── { type: "answer", sdp: "..." } ─►│
  │                                    │
  │◄── { type: "ice", candidate: ... } ┤  (ICE trickle)
  ├── { type: "ice", candidate: ... } ─►│
  │                                    │
  │◄═══════ WebRTC Connected ═════════►│
  │                                    │
  │  ◄── audio track(s) ─────────────┤  (receive only, no send)
  │                                    │
```

Broadcast clients:
- Receive only tracks designated by `broadcast` sink mappings in the session template
- Cannot send audio or data
- Have no control data channel
- Are not tracked as session clients (no role assignment)

## ICE / STUN / TURN

- **STUN**: Configure at least one STUN server (e.g., `stun:stun.l.google.com:19302`)
- **TURN**: Required if clients are behind symmetric NATs — server config should support specifying TURN credentials
- ICE candidates are trickled over the WebSocket connection (both signaling and broadcast)

Configuration in server config file (`ct-server.json`):

```json
{
  "webrtc": {
    "stun_servers": ["stun:stun.l.google.com:19302"],
    "turn": {
      "enabled": false,
      "server": "",
      "username": "",
      "credential": ""
    }
  }
}
```

> See [Configuration](configuration.md) for full config schema details.

## Peer Connection Management

The server maintains a registry of active peer connections:

- **Connection state tracking** — monitors ICE connection state, fires events on connect/disconnect/fail
- **Graceful cleanup** — on disconnect, notifies session orchestrator, releases media tracks
- **Track management** — add/remove tracks dynamically as the session orchestrator commands channel bindings
- **Pure forwarding** — the server never decodes, mixes, or processes media; it forwards RTP packets as-is between peer connections

## Control Data Channel

Immediately after WebRTC connection, the server and client establish a reliable, ordered data channel named `control`. All messages are Protobuf-encoded.

This channel carries:
- Client → Server: capability reports, status updates, source/sink changes
- Server → Client: bind/unbind commands, session events
- Bidirectional: log messages streamed to session participants

Note: Broadcast clients do **not** have a control data channel.

> See [Data Model > Protobuf Schema](../data-model/protobuf.md) for message format.
