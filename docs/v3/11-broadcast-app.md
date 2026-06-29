# Step 11: Broadcast Listener App

## Goal

A minimal, public-facing React app that receives a broadcast stream. No authentication required — accessed via a unique URL per session. Loads instantly and plays audio immediately.

## UX Flow

```
1. User receives broadcast URL: https://ct.example.com/listen/{broadcast_token}
2. Browser loads the broadcast app
3. App fetches session info via public API (name, listener count)
4. User clicks "Listen" (required for browser autoplay policy)
5. WebSocket signaling → receive-only WebRTC connection
6. Audio plays immediately
7. UI shows: session name, "LIVE" indicator, listener count, volume control
```

## Design

- **Zero auth** — broadcast token in URL is the only credential
- **Receive-only** — no mic access, no data channel needed
- **Minimal UI** — session name, play/pause, volume, listener count
- **Mobile-first** — works on phone browsers
- **Instant load** — tiny bundle, no heavy dependencies

## Tasks

### 11.1 Project scaffold
- [ ] `apps/broadcast/` — Vite + React + TypeScript
- [ ] Minimal deps: just React + the generated API client types
- [ ] No routing library (single page)
- [ ] Tailwind for styling, minimal component library

### 11.2 Broadcast info fetch
- [ ] Extract broadcast token from URL path
- [ ] `GET /api/sessions/{id}/broadcast?token={token}` — public endpoint
- [ ] Returns: session name, listener count, WebRTC connection params
- [ ] Error handling: invalid/expired token → friendly error page

### 11.3 WebRTC receive-only connection
- [ ] WebSocket signaling at `/ws/broadcast?token={token}`
- [ ] Create RTCPeerConnection with receive-only config
- [ ] Complete SDP exchange (server sends offer with audio track)
- [ ] Receive audio track → connect to AudioContext → speakers
- [ ] No data channel, no mic, no sending

### 11.4 Playback UI
- [ ] Large "Listen" button (required for AudioContext activation)
- [ ] After playing:
  - Session name
  - "LIVE" badge (animated)
  - Volume slider
  - Listener count (updated periodically or via stats)
  - Pause/resume button
- [ ] Connection state indicator (connecting spinner → live → error)

### 11.5 Mobile optimization
- [ ] Responsive layout
- [ ] Touch-friendly volume slider
- [ ] Handle iOS AudioContext restrictions (resume on user gesture)
- [ ] Service worker for offline-capable shell (optional)
- [ ] Meta tags for PWA-like experience (add to home screen)

### 11.6 QR code support
- [ ] Admin UI generates QR code for broadcast URL
- [ ] QR code printable (high contrast, with session name label)
- [ ] URL is regeneratable (old QR codes stop working)

### 11.7 WebRTC debug (hidden)
- [ ] Debug info available via URL param `?debug=1`
- [ ] Shows ICE state, candidates, SDP, packet stats
- [ ] Helpful for troubleshooting listener connectivity issues

## Broadcast URL Format

```
https://ct.example.com/listen/{session_id}?t={broadcast_token}
```

- `session_id` identifies the session
- `broadcast_token` is a regenerable secret that authorizes access
- Regenerating the token invalidates all existing URLs

## Acceptance Criteria

- [ ] Load broadcast URL → see session name
- [ ] Click Listen → audio plays
- [ ] Volume slider works
- [ ] Listener count updates
- [ ] Works on mobile Chrome + Safari
- [ ] Invalid token shows error, doesn't crash
- [ ] Multiple listeners can connect simultaneously
- [ ] Page load time < 1s on 3G (small bundle)

## Reusable from v2

- `web/src/lib/use-broadcast-listener.ts` — receive-only WebRTC pattern
- `web/src/pages/ListenerPage.tsx` — broadcast UI layout
- `server/ws/broadcast.go` — broadcast WebSocket signaling handler
- `server/broadcast/token_store.go` — token validation pattern
