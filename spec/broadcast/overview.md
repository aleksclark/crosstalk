# Public Broadcast Listeners

[← Back to Index](../index.md)

Public broadcast listeners allow unauthenticated users to receive audio from a CrossTalk session by scanning a QR code or opening a link. This enables live translation audiences, remote monitoring, and public streaming scenarios without requiring user accounts or admin access.

---

## Architecture Overview

```
┌─────────────┐    QR scan / link     ┌─────────────────┐
│ Admin Web UI │ ──────────────────►   │ ListenerPage     │
│ (SessionDetail)│                     │ /listen/:sid     │
└──────┬───────┘                       │ (no auth shell)  │
       │                               └───────┬──────────┘
       │ POST /api/sessions/:id/                │
       │   broadcast-token                      │ GET /api/broadcast/:sid/info?token=...
       │                                        │
       ▼                                        ▼
┌──────────────┐                       ┌──────────────────┐
│ Server       │                       │ Server           │
│ Token Store  │                       │ (public endpoint)│
│ (in-memory,  │                       │ returns session  │
│  time-expiry)│                       │ name + ICE cfg   │
└──────────────┘                       └───────┬──────────┘
                                               │
                                    WS /ws/broadcast?token=...
                                               │
                                               ▼
                                    ┌──────────────────────┐
                                    │ Server               │
                                    │ BroadcastSignaling   │
                                    │ Handler              │
                                    │ - Creates receive-   │
                                    │   only PeerConn      │
                                    │ - No control channel  │
                                    │ - Auto-joins as       │
                                    │   "listener" role     │
                                    │ - Receives broadcast  │
                                    │   sink tracks only    │
                                    └──────────────────────┘
```

### Data Flow

1. **Admin** views `SessionDetailPage` for a session whose template has `→ broadcast` mappings.
2. Admin clicks "Generate Broadcast Link". The UI calls `POST /api/sessions/{id}/broadcast-token` (authenticated).
3. Server generates a short-lived, opaque **broadcast token** (HMAC-signed, 15-minute default TTL). The token encodes `{session_id, expires_at}` and is stored in an in-memory map for validation.
4. The UI builds a URL: `https://{host}/listen/{session_id}?token={broadcast_token}` and renders it as a QR code.
5. **Listener** scans the QR code or clicks the link. The browser navigates to `/listen/{session_id}?token={broadcast_token}`.
6. `ListenerPage` fetches `GET /api/broadcast/{session_id}/info?token={broadcast_token}` to get the session title and ICE server configuration. This endpoint is **public** (no admin auth), but validates the broadcast token.
7. `ListenerPage` opens a WebSocket to `/ws/broadcast?token={broadcast_token}`.
8. `BroadcastSignalingHandler` validates the token, creates a **receive-only** `PeerConn` (no data channel, no mic), auto-registers the peer as a "listener" in the Orchestrator, and starts SDP/ICE signaling.
9. The Orchestrator detects the new listener and forwards all active `broadcast`-sink tracks to the new peer via `ForwardTrack`.
10. The listener hears the audio. The `ListenerPage` provides play/pause and volume controls.
11. When the listener disconnects or the session ends, the peer is removed and the listener count decrements.

---

## Broadcast Token Model

### Design Principles

- **No user account required** — listeners are anonymous.
- **Short-lived** — tokens expire after 15 minutes (configurable via `auth.broadcast_token_lifetime` in config). The WebSocket connection established before expiry remains active for the session duration.
- **Session-scoped** — each token is bound to exactly one session ID.
- **Not stored in SQLite** — broadcast tokens live in an in-memory map with TTL eviction. They are ephemeral and do not survive server restarts (a restart invalidates outstanding QR codes, which is acceptable).
- **Revocable** — ending a session invalidates all broadcast tokens for that session.

### Token Format

```
ctb_{base64url(HMAC-SHA256(session_id + expires_at, session_secret))}_{session_id}_{expires_unix}
```

The `ctb_` prefix distinguishes broadcast tokens from API tokens (`ct_`). The server validates by recomputing the HMAC. The token also exists as a key in the in-memory token map for O(1) lookup and explicit revocation.

### Token Lifecycle

```
Admin clicks "Generate Link"
    │
    ▼
POST /api/sessions/:id/broadcast-token
    │
    ├─► Server validates: session exists, status != ended, template has broadcast mappings
    ├─► Server generates token with 15min TTL
    ├─► Server stores in memory map: token → {session_id, expires_at}
    └─► Returns: { token, url, expires_at }

Listener opens URL
    │
    ▼
GET /api/broadcast/:id/info?token=...
    │
    ├─► Server validates token: HMAC check + expiry check + session_id match
    └─► Returns: { session_name, ice_servers }

WS /ws/broadcast?token=...
    │
    ├─► Server validates token (same as above)
    ├─► Upgrades to WebSocket
    ├─► Creates receive-only PeerConn
    └─► Connection remains open until session ends or client disconnects
        (token expiry does NOT close existing connections)
```

---

## QR Code Generation

QR codes are generated **client-side** in the React app using a library like `qrcode.react`. This avoids adding image-generation dependencies to the Go server.

### Flow

1. `SessionDetailPage` calls `POST /api/sessions/{id}/broadcast-token`.
2. The response includes the full URL: `https://{host}/listen/{session_id}?token={token}`.
3. The UI renders the URL as a QR code using `<QRCodeSVG>`.
4. The QR code is displayed in a modal or card alongside the plaintext URL and a "Copy Link" button.
5. The URL is also a clickable link for testing.

### URL Structure

```
/listen/{session_id}?token={broadcast_token}
```

The `session_id` is in the path (not just the token) so that:
- The page can display a "loading" state before token validation completes.
- The URL is human-readable and debuggable.
- The server can validate that the token matches the session ID in the path.

---

## WebRTC Flow for Receive-Only Listeners

### Differences from Authenticated Peers

| Aspect | Authenticated Peer | Broadcast Listener |
|--------|-------------------|-------------------|
| **Auth** | API token (admin) | Broadcast token (public) |
| **WebSocket** | `/ws/signaling` | `/ws/broadcast` |
| **Direction** | Send + receive | Receive only |
| **Data channel** | Yes (control channel) | No |
| **Mic access** | Yes | No |
| **Role** | Template-defined role | Virtual `__listener` role |
| **Track forwarding** | Bidirectional per mappings | Receives broadcast-sink tracks only |
| **Session join** | Explicit via control channel `joinSession` | Automatic on WebSocket connect |
| **Count** | Included in `client_count` | Separate `listener_count` field |

### Signaling Protocol

The broadcast WebSocket uses the **same JSON signaling protocol** as authenticated peers:

```json
// Client → Server
{ "type": "offer", "sdp": "..." }
{ "type": "ice", "candidate": { ... } }

// Server → Client
{ "type": "answer", "sdp": "..." }
{ "type": "ice", "candidate": { ... } }
{ "type": "offer", "sdp": "..." }  // renegotiation
```

The client sends an offer with `recvonly` transceivers for audio. The server adds tracks and triggers renegotiation as broadcast sources come online.

### PeerConn Setup

1. `BroadcastSignalingHandler.ServeHTTP` validates the broadcast token.
2. Creates a `PeerConn` via `PeerManager.CreatePeerConnection()` — but **skips** the control data channel creation (since listeners don't need it).
3. Registers the peer in the Orchestrator as a listener for the session.
4. The Orchestrator's `evaluateBindings` already resolves `broadcast` sinks. When a listener exists, `ForwardTrack` sends the broadcast audio to each listener peer.
5. On WebSocket close, `LeaveSession` and `RemovePeer` clean up.

### Listener Role in Orchestrator

Broadcast listeners are **not** assigned a template-defined role. Instead, they are tracked in a separate `Listeners` map on `LiveSession`:

```go
type LiveSession struct {
    // ... existing fields ...
    Listeners map[string]*PeerConn // keyed by peer ID
}
```

When `evaluateBindings` activates a `broadcast` sink binding, the Orchestrator iterates `ls.Listeners` and calls `ForwardTrack(sourcePeer, listenerPeer, channelID)` for each listener. When a new listener joins a session that already has active broadcast bindings, the Orchestrator immediately starts forwarding existing broadcast tracks to the new listener.

---

## Listener Count Tracking

### In-Memory Tracking

The `LiveSession.Listeners` map provides an instant count via `len(ls.Listeners)`.

### REST API Exposure

The `GET /api/sessions/{id}` response is extended with a `listener_count` field:

```json
{
  "id": "01HXYZ...",
  "name": "Sunday Service",
  "status": "active",
  "client_count": 2,
  "listener_count": 5,
  "total_roles": 2,
  "clients": [...],
  "recording": { ... }
}
```

`listener_count` is populated from `Orchestrator.ListenerCount(sessionID)`.

### Real-Time Updates

When a listener joins or leaves, the Orchestrator sends a `SessionEvent` to all **authenticated** peers in the session:

```json
{
  "sessionEvent": {
    "type": "SESSION_LISTENER_COUNT_CHANGED",
    "sessionId": "01HXYZ...",
    "message": "listener_count:5"
  }
}
```

The `SessionConnectPage` parses this event and updates the displayed listener count in real time.

---

## API Endpoints

### `POST /api/sessions/{id}/broadcast-token` (authenticated)

Generates a short-lived broadcast token for the given session.

**Request:**
```http
POST /api/sessions/01HXYZ.../broadcast-token
Authorization: Bearer ct_...
```

No request body required.

**Response (200):**
```json
{
  "token": "ctb_abc123..._01HXYZ..._1700000000",
  "url": "https://crosstalk.example.com/listen/01HXYZ...?token=ctb_abc123..._01HXYZ..._1700000000",
  "expires_at": "2024-01-15T12:15:00Z"
}
```

**Errors:**
- `404` — session not found
- `400` — session has no broadcast mappings in its template
- `400` — session is ended
- `401` — missing or invalid auth

### `GET /api/broadcast/{id}/info?token={broadcast_token}` (public)

Returns public session info for listeners. No admin auth required; validated via broadcast token.

**Response (200):**
```json
{
  "session_id": "01HXYZ...",
  "session_name": "Sunday Service",
  "ice_servers": [
    { "urls": ["stun:stun.l.google.com:19302"] }
  ]
}
```

**Errors:**
- `401` — invalid or expired broadcast token
- `404` — session not found or ended

### `WS /ws/broadcast?token={broadcast_token}` (public)

WebSocket signaling endpoint for broadcast listeners. Same JSON protocol as `/ws/signaling` but creates a receive-only peer.

**Query parameters:**
- `token` — broadcast token (required)

**Behavior:**
- Validates broadcast token
- Upgrades to WebSocket
- Creates receive-only PeerConn (no control channel)
- Auto-joins the session as a listener
- Runs signaling read loop until close

---

## Web Pages / Routes

### `ListenerPage` — `/listen/{session_id}`

Public page (outside the `<ProtectedRoute>` wrapper). No login required.

**Query parameters:**
- `token` — broadcast token (required, from QR code URL)

**Component:** `web/src/pages/ListenerPage.tsx`

**Behavior:**
1. Extract `session_id` from URL path and `token` from query string.
2. Fetch `GET /api/broadcast/{session_id}/info?token={token}` to get session name + ICE servers.
3. Display session name as page title.
4. Open WebSocket to `/ws/broadcast?token={token}`.
5. Perform SDP offer/answer exchange (recvonly audio).
6. When remote tracks arrive, connect to Web Audio API.
7. Display:
   - Session title
   - Play/Pause button (toggles `AudioContext.resume()` / `AudioContext.suspend()`)
   - Volume slider (controls `GainNode.gain`)
   - Connection status indicator (connecting / connected / disconnected)
   - "Powered by CrossTalk" footer
8. No mic access, no data channel UI, no debug panel.

**Hook:** `web/src/lib/use-broadcast-listener.ts` — simplified version of `use-webrtc.ts` for receive-only operation.

### `SessionDetailPage` — QR Code Addition

The existing `SessionDetailPage` gains a new card/section when the session template contains `→ broadcast` mappings:

**New UI elements:**
- "Broadcast" card with:
  - "Generate Link" button → calls `POST /api/sessions/{id}/broadcast-token`
  - QR code display (rendered via `qrcode.react`)
  - Plaintext URL with "Copy" button
  - Token expiry countdown
  - Current listener count (from `GET /api/sessions/{id}` response)

### `SessionConnectPage` — Listener Count Display

The existing `SessionConnectPage` gains a listener count badge in the header bar:

```
🔊 5 listeners
```

This is populated from the `listener_count` field in the session API response and updated in real time via `SESSION_LISTENER_COUNT_CHANGED` control channel events.

---

## React Route Configuration

```tsx
// In App.tsx — add outside ProtectedRoute wrapper
<Route path="/listen/:sessionId" element={<ListenerPage />} />
```

This route is **not** wrapped in `<ProtectedRoute>` and does **not** require `<Layout>` (no nav sidebar). The `ListenerPage` renders its own minimal chrome.

---

## Security Considerations

- **Token scoping**: Broadcast tokens are bound to a single session and cannot be used to access other sessions or admin endpoints.
- **Rate limiting**: The `POST /api/sessions/{id}/broadcast-token` endpoint should be rate-limited to prevent token flooding (future work, tracked in spec/server/security.md).
- **Token TTL**: Short-lived tokens limit the window for link sharing. Once a WebSocket connection is established, it persists for the session duration regardless of token expiry.
- **No data exfiltration**: Listeners cannot send audio or data back to the session. The `PeerConn` has no send tracks and no data channel.
- **Session ending**: When a session ends, all listener WebSocket connections are closed and all broadcast tokens for that session are invalidated.
- **CORS**: No CORS issues — the listener page and API are served from the same origin.
