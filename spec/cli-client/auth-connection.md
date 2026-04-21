# Authentication & Connection

[← Back to Index](../index.md) · [CLI Client Overview](overview.md)

---

## Authentication Flow

```
CLI Client                           Server
    │                                   │
    ├── GET /api/clients/self ─────────►│  (Bearer token from CROSSTALK_TOKEN)
    │◄── 200 OK (validates token) ─────┤
    │                                   │
    ├── POST /api/webrtc/token ────────►│
    │◄── { token: "wrt_xyz" } ─────────┤  (24h lifetime)
    │                                   │
    ├── WS /ws/signaling ─────────────►│  (WebSocket, auth token in query)
    │◄── connected ────────────────────┤
    │                                   │
    ├── { type: "offer", sdp: "..." } ─►│  (SDP offer over WebSocket)
    │◄── { type: "answer", sdp: "..." } ┤  (SDP answer over WebSocket)
    │                                   │
    ├── ICE candidate exchange ────────►│  (trickled over WebSocket)
    │◄── ICE candidate exchange ───────┤
    │                                   │
    │◄═══════ WebRTC Connected ════════►│
    │                                   │
    │── control channel: Hello ────────►│  (capabilities, sources/sinks)
    │◄── control channel: Welcome ─────┤  (server config, assigned roles)
```

## Token Handling

- Token read from `CROSSTALK_TOKEN` environment variable
- Used as `Authorization: Bearer <token>` for all REST calls
- If token is invalid/expired, client exits with clear error message
- No interactive login — CLI clients are always token-authenticated

## Connection Resilience

- On WebRTC disconnect: exponential backoff reconnect (1s, 2s, 4s, ... capped at 60s)
- On REST auth failure (401/403): exit immediately (don't retry with bad credentials)
- On network unreachable: retry with backoff
- WebRTC token refresh: request new token before current one expires (at ~23h mark)
- Log all connection state transitions for debugging
