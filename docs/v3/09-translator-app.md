# Step 09: Translator Frontend App

## Goal

A standalone React SPA for translators. Completely separate from the admin UI. Focused on the translation workflow: see available sessions, connect, hear the feed, speak the translation.

## UX Flow

```
1. Translator opens app → Login screen (if not already authenticated)
2. After login → Session list (only sessions assigned to this translator)
3. Click session → Session connect view
4. Session connect view:
   - Audio input selector (mic)
   - "Connect" button
   - On connect: hears feed channel, mic goes to translator source
   - VU meters: input (mic) + output (feed)
   - Mute toggle for own mic
   - Session status indicators (who else is connected)
   - WebRTC debug panel (collapsible, verbose)
```

## Tasks

### 9.1 Project scaffold
- [ ] `apps/translator/` — Vite + React + TypeScript
- [ ] Import `@crosstalk/api-client` (generated, from Step 04)
- [ ] Tailwind + shadcn/ui for consistent dark-mode styling
- [ ] JWT auth flow with token refresh

### 9.2 Login page
- [ ] Username + password form
- [ ] Stores JWT in memory (not localStorage for security)
- [ ] Auto-redirect to session list on success
- [ ] Error handling for invalid credentials

### 9.3 Session list page
- [ ] Fetch sessions assigned to current user
- [ ] Show session name, status (active/idle), connected count
- [ ] Click to enter session connect view
- [ ] Auto-refresh session list periodically

### 9.4 Session connect view
- [ ] Mic selector dropdown (enumerate media devices)
- [ ] Connect button → WebSocket signaling → WebRTC connection
- [ ] On connect:
  - Send Hello with client_type="translator"
  - Receive Welcome with session assignment
  - Translator's mic becomes a source in the session
  - Translator receives feed channel audio
- [ ] VU meter for mic input (local, pre-WebRTC)
- [ ] VU meter for feed output (from received track)
- [ ] Mute toggle (stops sending audio, does NOT disconnect)
- [ ] Connection status indicator (connecting/connected/disconnected)

### 9.5 WebRTC debug panel
- [ ] Collapsible panel showing:
  - ICE connection state
  - Local/remote candidates (count + details)
  - SDP info (codecs, tracks)
  - Data channel state
  - Packet stats (sent/received, loss, jitter)
  - Event log (scrollable, color-coded by severity)
- [ ] All WebRTC events logged to this panel
- [ ] No information hidden — everything visible

### 9.6 Mix controls (if translator has permission)
- [ ] Show sources in the feed channel
- [ ] Mute toggle + level slider for each source
- [ ] Real-time updates via control channel MixUpdate messages
- [ ] Changes reflected immediately in the feed they're hearing

## Acceptance Criteria

- [ ] Translator can log in and see only their assigned sessions
- [ ] Translator can connect to a session and hear the feed channel
- [ ] Translator's mic audio arrives at the server as a source
- [ ] Mute toggle works (verified by checking source activity on server)
- [ ] WebRTC debug panel shows all ICE/SDP/track events
- [ ] Connection survives brief network interruption (ICE restart)
- [ ] Works on Chrome, Firefox, Safari (desktop)

## Reusable from v2

- `web/src/lib/use-webrtc.ts` — WebRTC hook pattern (adapt for new protocol)
- `web/src/lib/webrtc-types.ts` — type definitions for WebRTC state
- `web/src/pages/SessionConnectPage.tsx` — VU meter + mic selector pattern
- `web/src/pages/LoginPage.tsx` — login form pattern
