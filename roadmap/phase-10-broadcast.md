# Phase 10: Public Broadcast Listeners

Public, unauthenticated users can receive broadcast audio from a session by scanning a QR code or opening a link. Admin UI shows listener count in real time.

**Depends on**: Phase 5 (session orchestration), Phase 7 (admin web UI), Phase 8 (integration tests)

**Spec reference**: [spec/broadcast/overview.md](../spec/broadcast/overview.md)

**Exit criteria**: A Playwright test creates a session with a `→ broadcast` mapping, generates a broadcast token, opens the public listener page, verifies audio is received, and confirms the listener count is displayed on the admin connect page.

---

## Step 1: Server — Broadcast Token Endpoint

**Goal**: Admin can generate a short-lived broadcast token scoped to a session.

### Tasks

- [ ] Add `BroadcastToken` struct to `server/domain.go`:
  ```go
  type BroadcastToken struct {
      Token     string
      SessionID string
      ExpiresAt time.Time
  }
  ```
- [ ] Add `BroadcastTokenStore` interface to `server/domain.go`:
  ```go
  type BroadcastTokenStore interface {
      CreateBroadcastToken(sessionID string, ttl time.Duration) (*BroadcastToken, error)
      ValidateBroadcastToken(token string) (*BroadcastToken, error)
      RevokeBroadcastTokens(sessionID string)
  }
  ```
- [ ] Implement in-memory `BroadcastTokenStore` in `server/http/broadcast_token.go`:
  - Token format: `ctb_{hmac}_{session_id}_{expires_unix}`
  - HMAC computed using `Config.Auth.SessionSecret`
  - In-memory map with lazy TTL eviction
  - `RevokeBroadcastTokens` deletes all tokens for a session ID
- [ ] Add `broadcast_token_lifetime` field to `Config.Auth` in `server/config.go` (default: `"15m"`)
- [ ] Add `POST /api/sessions/{id}/broadcast-token` handler to `server/http/handler.go`:
  - Requires auth (inside the authenticated route group)
  - Validates: session exists, status != ended, template has at least one `→ broadcast` mapping
  - Returns `{ token, url, expires_at }`
  - The `url` is built from the `Host` header: `{scheme}://{host}/listen/{session_id}?token={token}`
- [ ] Wire `BroadcastTokenStore` into `Handler` struct
- [ ] Add mock `BroadcastTokenStore` to `server/mock/`

### Acceptance Criteria

```
Test: TestBroadcastTokenEndpoint
Given: An authenticated admin and a session whose template has a "→ broadcast" mapping
When:  POST /api/sessions/{id}/broadcast-token
Then:  Response 200 with { token: "ctb_...", url: "https://.../listen/...", expires_at: "..." }

Test: TestBroadcastTokenRejectsNoBroadcastMapping
Given: A session whose template has no "→ broadcast" mappings
When:  POST /api/sessions/{id}/broadcast-token
Then:  Response 400 with error message about no broadcast mappings

Test: TestBroadcastTokenRejectsEndedSession
Given: A session with status "ended"
When:  POST /api/sessions/{id}/broadcast-token
Then:  Response 400

Test: TestBroadcastTokenValidation
Given: A generated broadcast token
When:  Validated immediately → success
When:  Validated after TTL → failure
When:  Validated with tampered HMAC → failure

Test: TestBroadcastTokenRevocation
Given: A generated broadcast token for session X
When:  RevokeBroadcastTokens(session_x_id)
Then:  Token validation fails
```

---

## Step 2: Server — Broadcast Listener WebSocket Signaling

**Goal**: Public listeners can connect via WebSocket, complete WebRTC signaling, and receive broadcast audio tracks — all without admin authentication.

### Tasks

- [ ] Add `GET /api/broadcast/{id}/info?token={broadcast_token}` handler:
  - Public (no auth middleware)
  - Validates broadcast token
  - Returns `{ session_id, session_name, ice_servers }`
- [ ] Create `BroadcastSignalingHandler` in `server/ws/broadcast.go`:
  - `ServeHTTP` validates the broadcast token from query parameter
  - Upgrades to WebSocket
  - Creates a `PeerConn` via `PeerManager` — modified to **skip control channel creation** for listener peers
  - Registers the peer as a listener in the Orchestrator (new method: `AddListener`)
  - Runs the same signaling read loop as `SignalingHandler` (offer/answer/ice)
  - On close, calls `RemoveListener` and `RemovePeer`
- [ ] Add `PeerManager.CreateListenerPeerConnection()` method:
  - Same as `CreatePeerConnection` but without creating the "control" data channel
  - Sets the peer connection to recvonly (no send transceivers)
- [ ] Add `Orchestrator.AddListener(peer *PeerConn, sessionID string) error`:
  - Validates session exists and is not ended
  - Adds peer to `LiveSession.Listeners` map
  - If broadcast bindings are already active, immediately starts `ForwardTrack` for each one to the new listener
- [ ] Add `Orchestrator.RemoveListener(peer *PeerConn)`:
  - Removes peer from `LiveSession.Listeners`
  - Stops all forwarding to this listener
- [ ] Update `evaluateBindings` to forward broadcast-sink bindings to all listeners:
  - When a broadcast binding activates, iterate `ls.Listeners` and call `ForwardTrack` for each
  - When a broadcast binding deactivates, stop forwarding to all listeners
- [ ] Mount `BroadcastSignalingHandler` at `/ws/broadcast` in `Handler.Router()` (before API routes, like existing signaling handler)
- [ ] Mount broadcast info endpoint at `/api/broadcast/{id}/info` (public, outside auth group)
- [ ] Update `Orchestrator.EndSession` to close all listener connections and revoke broadcast tokens

### Acceptance Criteria

```
Test: TestBroadcastInfoEndpoint
Given: A valid broadcast token
When:  GET /api/broadcast/{id}/info?token={token}
Then:  Response 200 with { session_id, session_name, ice_servers }

Test: TestBroadcastInfoRejectsInvalidToken
When:  GET /api/broadcast/{id}/info?token=bad_token
Then:  Response 401

Test: TestBroadcastListenerWebRTC (Go integration test)
Given: A session with template mapping "studio:mic → broadcast"
  And: A studio peer connected and sending audio
When:  A broadcast listener connects via /ws/broadcast?token={valid_token}
  And: Completes SDP offer/answer exchange
Then:  The listener's PeerConnection receives an audio track
  And: RTP packets from the studio peer arrive at the listener

Test: TestBroadcastListenerReceiveOnly
Given: A connected broadcast listener
Then:  The listener peer has no send transceivers
  And: No data channel exists on the listener peer

Test: TestBroadcastListenerLateJoin
Given: A session where broadcast forwarding is already active
When:  A new listener connects
Then:  The new listener immediately receives the broadcast track
  And: Existing listeners are unaffected
```

---

## Step 3: Server — Listener Count Tracking

**Goal**: The REST API and control channel expose the count of connected broadcast listeners for each session.

### Tasks

- [ ] Add `Listeners map[string]*PeerConn` field to `LiveSession` struct in `server/pion/orchestrator.go`
- [ ] Add `Orchestrator.ListenerCount(sessionID string) int` method
- [ ] Extend `sessionResponse` in `server/http/handler.go` with `ListenerCount int` field:
  ```go
  type sessionResponse struct {
      // ... existing fields ...
      ListenerCount int `json:"listener_count"`
  }
  ```
- [ ] Update `toSessionResponse` to call `Orchestrator.ListenerCount(sessionID)` and populate the field
- [ ] Add `ListenerCount(sessionID string) int` to `SessionOrchestrator` interface in `domain.go`
- [ ] When a listener joins or leaves, send `SESSION_LISTENER_COUNT_CHANGED` event to all authenticated peers in the session:
  ```go
  sendSessionEvent(peer, crosstalkv1.SessionEventType_SESSION_LISTENER_COUNT_CHANGED,
      sessionID, fmt.Sprintf("listener_count:%d", len(ls.Listeners)))
  ```
- [ ] Add `SESSION_LISTENER_COUNT_CHANGED` to the protobuf `SessionEventType` enum in `proto/crosstalk/v1/control.proto`
- [ ] Regenerate protobuf Go + TypeScript code

### Acceptance Criteria

```
Test: TestListenerCountInSessionResponse
Given: A session with 3 connected broadcast listeners
When:  GET /api/sessions/{id}
Then:  Response includes "listener_count": 3

Test: TestListenerCountZeroWhenNoListeners
Given: A session with no broadcast listeners
When:  GET /api/sessions/{id}
Then:  Response includes "listener_count": 0

Test: TestListenerCountEventOnJoin
Given: An authenticated admin peer connected to a session
When:  A broadcast listener connects
Then:  The admin peer receives a SessionEvent with type SESSION_LISTENER_COUNT_CHANGED

Test: TestListenerCountEventOnLeave
Given: An authenticated admin peer and 2 broadcast listeners
When:  One listener disconnects
Then:  The admin peer receives a SessionEvent with updated count
```

---

## Step 4: Web — QR Code on SessionDetailPage

**Goal**: The session detail page shows a QR code and shareable link for broadcast sessions.

### Tasks

- [ ] Install `qrcode.react` package: `pnpm add qrcode.react`
- [ ] Add `createBroadcastToken(sessionId: string)` function to `web/src/lib/api/client.ts`:
  ```typescript
  export async function createBroadcastToken(sessionId: string): Promise<{
    token: string
    url: string
    expires_at: string
  }> {
    const res = await fetchWithAuth(`/api/sessions/${sessionId}/broadcast-token`, {
      method: 'POST',
    })
    return res.json()
  }
  ```
- [ ] Create `BroadcastCard` component in `web/src/components/BroadcastCard.tsx`:
  - Props: `sessionId: string, hasBroadcastMapping: boolean`
  - "Generate Broadcast Link" button
  - On click: calls `createBroadcastToken(sessionId)`
  - Displays QR code via `<QRCodeSVG value={url} size={200} />`
  - Shows plaintext URL with "Copy to Clipboard" button
  - Shows token expiry as countdown timer
  - Shows current listener count (from session data)
  - Disabled/hidden when `hasBroadcastMapping` is false
- [ ] Detect broadcast mappings: check if any mapping in the session's template has `sink === "broadcast"`
- [ ] Add `BroadcastCard` to `SessionDetailPage` between the "Connected Clients" and "Channel Bindings" cards
- [ ] Add `data-testid` attributes for Playwright: `broadcast-card`, `generate-link-button`, `qr-code`, `broadcast-url`, `copy-link-button`, `listener-count`

### Acceptance Criteria

```
Test (Playwright): TestBroadcastCardHiddenWithoutMapping
Given: A session whose template has no "→ broadcast" mappings
Then:  The broadcast card is not visible on the session detail page

Test (Playwright): TestBroadcastCardVisibleWithMapping
Given: A session whose template has a "→ broadcast" mapping
Then:  The broadcast card is visible with a "Generate Broadcast Link" button

Test (Playwright): TestGenerateBroadcastLink
Given: A session with broadcast mappings
When:  Admin clicks "Generate Broadcast Link"
Then:  A QR code is displayed
  And: A URL is displayed that starts with the server origin + "/listen/"
  And: A "Copy" button is present

Test (Playwright): TestCopyBroadcastLink
Given: A generated broadcast link
When:  Admin clicks "Copy"
Then:  The URL is copied to the clipboard
```

---

## Step 5: Web — Public ListenerPage

**Goal**: Unauthenticated users can listen to broadcast audio via a simple, mobile-friendly page.

### Tasks

- [ ] Create `web/src/lib/use-broadcast-listener.ts` hook:
  ```typescript
  interface UseBroadcastListenerOptions {
    sessionId: string
    token: string
  }

  interface UseBroadcastListenerReturn {
    status: 'loading' | 'connecting' | 'connected' | 'disconnected' | 'error'
    sessionName: string
    error: string | null
    volume: number
    setVolume: (v: number) => void
    isPlaying: boolean
    togglePlayPause: () => void
  }
  ```
  - Fetches `/api/broadcast/{sessionId}/info?token={token}` for session name + ICE config
  - Opens WebSocket to `/ws/broadcast?token={token}`
  - Creates `RTCPeerConnection` with `recvonly` audio transceiver
  - Performs SDP offer/answer exchange
  - Handles trickle ICE
  - When remote track arrives: creates `AudioContext` + `GainNode` + hidden `<audio>` element
  - Exposes play/pause (via `AudioContext.resume()`/`suspend()`) and volume (via `GainNode.gain`)
  - Handles renegotiation (server-initiated offers for late-added tracks)
  - No data channel, no mic, no stats polling

- [ ] Create `web/src/pages/ListenerPage.tsx`:
  - Extracts `sessionId` from URL param, `token` from query string
  - Uses `useBroadcastListener` hook
  - Renders minimal, mobile-friendly UI:
    ```
    ┌───────────────────────────────┐
    │                               │
    │     🎵 Sunday Service         │  ← session name
    │                               │
    │     ┌─────────────────────┐   │
    │     │   ▶ / ⏸  Play/Pause│   │  ← large play/pause button
    │     └─────────────────────┘   │
    │                               │
    │     🔊 ─────●───────────── 🔊 │  ← volume slider
    │                               │
    │     ● Connected               │  ← status indicator
    │                               │
    │     Powered by CrossTalk      │  ← footer
    │                               │
    └───────────────────────────────┘
    ```
  - Error states: "Invalid or expired link", "Session has ended", "Connection failed"
  - No navigation bar, no sidebar, no login prompt
  - Responsive: works on phone screens (min 320px wide)

- [ ] Add route to `App.tsx`:
  ```tsx
  <Route path="/listen/:sessionId" element={<ListenerPage />} />
  ```
  Place this **before** the catch-all route and **outside** `ProtectedRoute`.

- [ ] Add `data-testid` attributes: `listener-page`, `session-title`, `play-pause-button`, `volume-slider`, `connection-status`, `error-message`

### Acceptance Criteria

```
Test (Playwright): TestListenerPageLoads
Given: A valid broadcast URL with token
When:  Navigated to /listen/{session_id}?token={token}
Then:  The page loads without redirect to /login
  And: The session name is displayed
  And: A play/pause button is visible
  And: A volume slider is visible

Test (Playwright): TestListenerPageInvalidToken
Given: A URL with an invalid broadcast token
When:  Navigated to /listen/{session_id}?token=bad
Then:  An error message "Invalid or expired link" is displayed

Test (Playwright): TestListenerPagePlayPause
Given: A connected listener page
When:  User clicks the play/pause button
Then:  The button toggles between play and pause state

Test (Playwright): TestListenerPageVolume
Given: A connected listener page
When:  User adjusts the volume slider
Then:  The volume changes (verified by checking GainNode.gain.value)

Test (Playwright): TestListenerPageMobileFriendly
Given: A viewport of 320x568 (iPhone SE)
When:  Navigated to a valid listener URL
Then:  All controls are visible and usable without horizontal scrolling
```

---

## Step 6: Web — Listener Count on SessionConnectPage

**Goal**: The admin's session connect view shows the current number of broadcast listeners.

### Tasks

- [ ] Parse `SESSION_LISTENER_COUNT_CHANGED` events in `use-webrtc.ts`:
  ```typescript
  if (msg['sessionEvent']) {
    const evt = msg['sessionEvent'] as { type: string; message: string; sessionId: string }
    if (evt.type === 'SESSION_LISTENER_COUNT_CHANGED') {
      const count = parseInt(evt.message.split(':')[1], 10)
      setListenerCount(count)
    }
    // ... existing event handling
  }
  ```
- [ ] Add `listenerCount` state to `useWebRTC` return type
- [ ] Add listener count badge to `SessionConnectPage` header bar:
  ```tsx
  {webrtc.listenerCount > 0 && (
    <Badge variant="secondary" data-testid="listener-count-badge">
      🔊 {webrtc.listenerCount} listener{webrtc.listenerCount !== 1 ? 's' : ''}
    </Badge>
  )}
  ```
- [ ] Also fetch initial listener count from `GET /api/sessions/{id}` response on page load and seed the state

### Acceptance Criteria

```
Test (Playwright): TestListenerCountDisplayed
Given: An admin connected to a session via SessionConnectPage
  And: 3 broadcast listeners are connected
Then:  The header shows "🔊 3 listeners"

Test (Playwright): TestListenerCountUpdatesInRealTime
Given: An admin connected to a session
When:  A new broadcast listener connects
Then:  The listener count badge increments without page refresh

Test (Playwright): TestListenerCountHiddenWhenZero
Given: An admin connected to a session with no broadcast listeners
Then:  No listener count badge is visible
```

---

## Step 7: Integration Test — Full Broadcast Flow

**Goal**: End-to-end Playwright test proves the complete broadcast listener flow works.

### Tasks

- [ ] Create `web/e2e/broadcast-listener.spec.ts`:
  ```typescript
  test('broadcast listener receives audio and count is displayed', async ({ page, context }) => {
    // 1. Reset server state
    // 2. Login as admin
    // 3. Create template with "studio:mic → broadcast" mapping
    // 4. Create session from template
    // 5. Navigate to session detail page
    // 6. Verify broadcast card is visible
    // 7. Click "Generate Broadcast Link"
    // 8. Extract the broadcast URL from the page
    // 9. Open the broadcast URL in a new page (no auth)
    // 10. Verify listener page loads with session name
    // 11. Verify connection status becomes "Connected"
    // 12. Navigate admin to session connect page
    // 13. Connect admin as "studio" role (with mic sending tone)
    // 14. Verify listener count shows "1 listener" on admin page
    // 15. Open a second listener in another page
    // 16. Verify listener count updates to "2 listeners"
    // 17. Close one listener page
    // 18. Verify listener count decrements to "1 listener"
    // 19. End session
    // 20. Verify listener page shows "Session has ended"
  })
  ```

- [ ] Create helper functions in `web/e2e/helpers/broadcast.ts`:
  ```typescript
  export async function createBroadcastTemplate(page: Page, token: string): Promise<string>
  export async function generateBroadcastLink(page: Page, sessionId: string): Promise<string>
  export async function openListenerPage(context: BrowserContext, url: string): Promise<Page>
  ```

- [ ] Add test to CI pipeline (runs with existing Playwright infrastructure in Docker)

### Acceptance Criteria

```
Test: broadcast-listener.spec.ts
Run:  task test:e2e (or pnpm --filter web exec playwright test broadcast-listener)
Pass: All assertions in the test pass:
  - Template with broadcast mapping created
  - Session created from template
  - Broadcast link generated (QR code visible)
  - Listener page loads without auth
  - Listener page shows session name
  - Listener page connects successfully
  - Admin connect page shows listener count
  - Listener count updates as listeners join/leave
  - Session end closes listener connections
```

---

## Summary of New/Modified Files

### New Files

| File | Description |
|------|-------------|
| `server/http/broadcast_token.go` | In-memory broadcast token store + token generation |
| `server/ws/broadcast.go` | `BroadcastSignalingHandler` for public WebSocket signaling |
| `server/mock/broadcast_token.go` | Mock `BroadcastTokenStore` for testing |
| `web/src/pages/ListenerPage.tsx` | Public listener page component |
| `web/src/lib/use-broadcast-listener.ts` | WebRTC hook for receive-only broadcast |
| `web/src/components/BroadcastCard.tsx` | QR code + broadcast link card component |
| `web/e2e/broadcast-listener.spec.ts` | End-to-end Playwright test |
| `web/e2e/helpers/broadcast.ts` | Test helper functions |
| `spec/broadcast/overview.md` | Feature specification |

### Modified Files

| File | Change |
|------|--------|
| `server/domain.go` | Add `BroadcastToken`, `BroadcastTokenStore` interface, extend `SessionOrchestrator` |
| `server/config.go` | Add `broadcast_token_lifetime` to `AuthConfig` |
| `server/http/handler.go` | Add broadcast token endpoint, broadcast info endpoint, extend `sessionResponse` |
| `server/pion/orchestrator.go` | Add `Listeners` to `LiveSession`, `AddListener`, `RemoveListener`, `ListenerCount`, update `evaluateBindings` |
| `server/pion/peer.go` | Add `CreateListenerPeerConnection` to `PeerManager` |
| `server/ws/signaling.go` | (minor refactor) Extract shared signaling loop for reuse |
| `proto/crosstalk/v1/control.proto` | Add `SESSION_LISTENER_COUNT_CHANGED` event type |
| `web/src/App.tsx` | Add `/listen/:sessionId` route |
| `web/src/lib/use-webrtc.ts` | Add `listenerCount` state, parse listener count events |
| `web/src/pages/SessionDetailPage.tsx` | Add `BroadcastCard` |
| `web/src/pages/SessionConnectPage.tsx` | Add listener count badge |
| `web/src/lib/api/client.ts` | Add `createBroadcastToken` function |
| `spec/index.md` | Add broadcast section to table of contents |
| `roadmap/index.md` | Add Phase 10 |
