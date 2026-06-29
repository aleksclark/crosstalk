# Step 10: Admin Frontend App

## Goal

Full-featured admin SPA for managing the CrossTalk system: sessions, channels, ABCs, translators, mix controls, and system monitoring.

## Pages

### Dashboard
- Active sessions with connection counts
- ABC status overview (connected/disconnected)
- Recent recordings
- System health (server version, uptime, active peers)

### Session Management
- Create/edit/archive sessions
- Configure channels per session (add feed/broadcast channels)
- View live session topology (who's connected, what's flowing)
- Broadcast URL management (view, regenerate)
- Mix control panel per channel

### ABC Management
- List all ABCs with connection status
- Create new ABC → generate token → show once
- Assign ABC to session
- Send restart command
- View ABC debug info (last connection, audio levels)

### Translator Management
- CRUD translator accounts
- Assign translators to sessions (multi-select)
- View translator connection status

### Mix Control
- Per-session, per-channel mixing interface
- Source list with mute toggles + level sliders
- Real-time VU meters per source
- Changes applied instantly via control channel
- Visual indication of source state (connected/disconnected/muted)

### WebRTC Debug
- List all active peers
- Per-peer: ICE state, candidates, SDP, events
- Session topology graph
- Signaling message log

## Tasks

### 10.1 Project scaffold
- [ ] `apps/admin/` — Vite + React + TypeScript
- [ ] Import `@crosstalk/api-client` (generated)
- [ ] Tailwind + shadcn/ui dark mode
- [ ] JWT auth flow, admin-role guard
- [ ] Router: react-router with protected routes

### 10.2 Dashboard page
- [ ] Server status card
- [ ] Active sessions list (name, channel count, source count)
- [ ] ABC status summary
- [ ] Quick actions: create session, view debug

### 10.3 Session pages
- [ ] Session list (with create button)
- [ ] Session create form (name, description, initial channels)
- [ ] Session detail: channels, sources, recordings, broadcast URL
- [ ] Session connect: admin can connect as a source or just monitor

### 10.4 Channel & mix pages
- [ ] Channel list within session detail
- [ ] Add/remove channels
- [ ] Mix control panel:
  - List all sources (current + historical)
  - Mute toggle per source
  - Level slider (0.0 - 2.0) per source
  - VU meter per source (real-time via WebRTC stats or control channel)
  - Visual distinction: connected (green) vs disconnected (gray) sources

### 10.5 ABC management pages
- [ ] ABC list with status indicators
- [ ] Create ABC form → token display (show once)
- [ ] ABC detail: assigned session, connection history, restart button
- [ ] Assign session dropdown

### 10.6 Translator management pages
- [ ] Translator list
- [ ] Create/edit translator form
- [ ] Session assignment (multi-select checkboxes)
- [ ] Connection status per translator

### 10.7 Debug pages
- [ ] Peer list with ICE state color coding
- [ ] Peer detail: full event log, SDP viewer, candidate list
- [ ] Session topology: visual graph of peers ↔ sources ↔ channels
- [ ] Signaling log viewer (filterable)

### 10.8 Admin connect view
- [ ] Admin can connect to any session (monitor or as source)
- [ ] Same WebRTC connect flow as translator but with full mix controls
- [ ] Can hear any channel (not just feed)

## Acceptance Criteria

- [ ] Admin can create session with channels via UI
- [ ] Admin can create ABC, get token, assign to session
- [ ] Admin can create translator, assign to sessions
- [ ] Mix controls update in real-time (slider change → immediate audio effect)
- [ ] Debug panel shows all WebRTC events for any peer
- [ ] Broadcast URL is displayed and regeneratable
- [ ] All pages responsive and functional in Chrome/Firefox

## Reusable from v2

- `web/src/pages/` — page structure patterns (list, detail, editor)
- `web/src/components/layout.tsx` — nav layout with sidebar
- `web/src/lib/use-webrtc.ts` — WebRTC connection hook
- `web/src/pages/SessionConnectPage.tsx` — connect view with VU meters
- `web/src/pages/TemplateEditorPage.tsx` — form patterns with validation
- `web/src/components/ui/` — shadcn component library
