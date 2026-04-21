# Protocol Stack

[← Back to Index](../index.md) · [Architecture Overview](overview.md)

---

## Layers

```
┌─────────────────────────────────┐
│  Application (sessions, roles)  │
├─────────────────────────────────┤
│  Protobuf (data channel msgs)  │
├─────────────────────────────────┤
│  WebRTC (media + data tracks)   │
├─────────────────────────────────┤
│  REST/HTTPS (management API)    │
├─────────────────────────────────┤
│  TCP/UDP + TLS                  │
└─────────────────────────────────┘
```

## REST API (Management Plane)

- HTTPS with JSON request/response bodies
- OpenAPI 3.1 spec **generated from Go types** — the Go struct definitions are the source of truth
- TypeScript client library auto-generated from the OpenAPI spec
- Auth: Bearer token or HTTP Basic (username/password)

> See [Server > REST API](../server/rest-api.md) for endpoint details.

## WebRTC (Realtime Plane)

Pion handles all WebRTC operations server-side.

**Signaling** is performed over a **WebSocket** connection at `/ws/signaling`. After authenticating via REST, clients open a WebSocket for the SDP offer/answer exchange and ICE candidate trickle. This avoids polling and allows the server to push ICE candidates immediately.

> **Broadcast clients** connect to a separate WebSocket endpoint `/ws/broadcast` that requires no authentication. They receive a server-generated SDP offer for listen-only media tracks.

**Media tracks**: Standard WebRTC audio (and later video) tracks for stream data.

**Data channel** (`control`): A single reliable data channel per client used for:
- Server → client commands (bind channel, disconnect, etc.)
- Client → server status reports (connection state, codec support, source/sink changes)
- Session log streaming (all clients' logs forwarded to admin UI)

## Protobuf (Data Channel Messages)

All messages on the `control` data channel use Protobuf encoding.

Generated outputs:
- Go types (server + CLI client)
- TypeScript types (admin web UI)

Message categories:
- **Command messages** — server instructs client to perform actions
- **Status messages** — client reports state to server
- **Log messages** — structured log entries (timestamp, severity, message) streamed to session participants

> See [Data Model > Protobuf Schema](../data-model/protobuf.md) for message definitions.

## WebRTC Token Auth

1. Client authenticates to REST API (token or username/password)
2. Client opens WebSocket to `/ws/signaling` (auth token in query param or header)
3. Client requests a WebRTC connection token over the WebSocket
4. Token has 24-hour lifetime
5. Client sends SDP offer over the WebSocket, server responds with SDP answer
6. ICE candidates are trickled bidirectionally over the same WebSocket
7. Server validates token before accepting the peer connection

**Broadcast clients** skip steps 1-2 and connect directly to `/ws/broadcast` with no auth. They receive listen-only streams designated by `broadcast` mappings in the session template.

This keeps authenticated signaling on one WebSocket and broadcast on a separate, unauthenticated path.
