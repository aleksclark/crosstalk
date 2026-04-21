# Phase 7: Admin Web UI

[← Roadmap](index.md)

**Status**: `NOT STARTED`  
**Depends on**: Phase 1 (REST API must work). Can proceed in parallel with Phases 2-6 for non-WebRTC pages.

React dashboard for managing the system and connecting to sessions as a browser-based client.

## Tasks

### 7.1 Project Setup
- [ ] Add shadcn/ui, install dark mode theme
- [ ] Add react-router for client-side routing
- [ ] Add TypeScript API client generation from OpenAPI spec (`task generate:api-client`)
- [ ] Configure Vite proxy for dev mode (already partially done in `vite.config.ts`)

**Test**: `task build:web` succeeds. `task lint:web` passes. Generated API client types compile.

### 7.2 Auth + Login Page
- [ ] Login page: username/password form → `POST /api/auth/login` → redirect to dashboard
- [ ] Auth context: store session, redirect to `/login` on 401
- [ ] Logout button

**Test (Vitest)**: Login component renders form, submits credentials, calls API. Auth context redirects on 401.

### 7.3 Dashboard
- [ ] Server status display (placeholder until real metrics exist)
- [ ] Connected clients table: fetch from `GET /api/clients`
- [ ] Quick-test button: `POST /api/sessions` with default template → redirect to session connect

**Test (Vitest)**: Dashboard renders client table from mock data. Quick-test button calls create session API.

### 7.4 Template Management
- [ ] List view: table of templates with name, roles, mapping count, default flag
- [ ] Editor view: name, default toggle, roles (with multi_client toggle), mappings editor
- [ ] Validation: multi-client roles can't be mapping sources (inline error)

**Test (Vitest)**: Template editor rejects invalid mapping (multi-client source). Create/edit/delete flows render correctly.

### 7.5 Session Management
- [ ] List view: sessions with status, client count, actions
- [ ] Detail view: connected clients per role, channel binding status
- [ ] End session button

**Test (Vitest)**: Session list renders from mock data. End session calls DELETE API.

### 7.6 Session Connect View
- [ ] Browser WebRTC connection via `/ws/signaling`
- [ ] Mic device selector (`getUserMedia` with device enumeration)
- [ ] VU meter per incoming audio channel (Web Audio API `AnalyserNode`)
- [ ] Volume control per channel (gain slider)
- [ ] WebRTC debug panel (`RTCPeerConnection.getStats()`)
- [ ] Session logs panel (control channel `LogEntry` messages from all clients)
- [ ] Send `JoinSession` on connect, send audio as source track

**Test (Vitest)**: Session connect view renders audio controls, debug panel, log panel. Mock WebRTC API interactions.

### 7.7 Quick-Test Flow
- [ ] Dashboard button creates session from default template, redirects to connect as `translator`
- [ ] Auto-request mic permission on connect
- [ ] End session button in connect view

**Test (Vitest)**: Quick-test button calls API, redirects to connect view with `role=translator`.

## Exit Criteria

1. `task build:web` produces `web/dist/` that's embeddable
2. `task lint:web` and `task test:unit:web` pass
3. All pages render and interact correctly with mocked API data
4. Session connect view can establish WebRTC connection to a real server (manual verification or Playwright in Phase 8)

## Spec Updates

- 4.1 Auth & Dashboard → 3
- 4.2 Management → 3
- 4.3 Session Connect View → 3
- 4.4 Quick-Test Flow → 3
