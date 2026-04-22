# Phase 7: Admin Web UI

[← Roadmap](index.md)

**Status**: `PARTIAL — 4/7 tasks done`  
**Depends on**: Phase 1 (REST API must work). Can proceed in parallel with Phases 2-6 for non-WebRTC pages.

React dashboard for managing the system and connecting to sessions as a browser-based client.

## Tasks

### 7.1 Project Setup
- [x] Add shadcn/ui, install dark mode theme `ae4cac7`
- [x] Add react-router for client-side routing `ae4cac7`
- [ ] Add TypeScript API client generation from OpenAPI spec (`task generate:api-client`)
- [x] Configure Vite proxy for dev mode (already partially done in `vite.config.ts`) `ae4cac7`

**Test**: `task build:web` succeeds. `task lint:web` passes. Generated API client types compile.

> **Review**: Build succeeds (`pnpm build` produces `web/dist/`). Typecheck passes. **However, lint fails with 2 errors**: (1) `auth.tsx` exports non-component `useAuth` alongside `AuthProvider` triggering `react-refresh/only-export-components`, (2) `DashboardPage.tsx` has `react-hooks/set-state-in-effect` for calling `fetchData()` inside `useEffect`. The API client at `src/lib/api/client.ts` is **hand-written**, not auto-generated from OpenAPI — there is no `generate:api-client` task in the Taskfile and no OpenAPI spec to generate from. Types in `types.ts` are manually defined.

### 7.2 Auth + Login Page
- [x] Login page: username/password form → `POST /api/auth/login` → redirect to dashboard `6ec48d5`
- [x] Auth context: store session, redirect to `/login` on 401 `6ec48d5`
- [x] Logout button `8a5f7f9`

**Test (Vitest)**: Login component renders form, submits credentials, calls API. Auth context redirects on 401.

> **Review**: All implemented. `LoginPage.test.tsx` (3 tests) validates: form renders, credentials submit and redirect to `/dashboard`, error display on failure. `auth.test.tsx` (4 tests) validates: starts unauthenticated, authenticates after login, clears on logout, redirects when unauthenticated. Auth context uses `sessionStorage` and `setOnUnauthorized` callback for 401 handling. Solid coverage.

### 7.3 Dashboard
- [x] Server status display (placeholder until real metrics exist) `6a52aae`
- [x] Connected clients table: fetch from `GET /api/clients` `6a52aae`
- [x] Quick-test button: `POST /api/sessions` with default template → redirect to session connect `6a52aae`

**Test (Vitest)**: Dashboard renders client table from mock data. Quick-test button calls create session API.

> **Review**: All implemented. Dashboard shows active sessions count, connected clients count, and templates count as summary cards. Client table has all columns from spec 4.1 (Client ID, Role, Session, Sources, Sinks, Codecs, Status, Connected Since). Quick-test button disabled when no default template, enabled when one exists, creates session and navigates to `/sessions/:id/connect?role=translator`. `DashboardPage.test.tsx` (3 tests) validates: counts render, client table with mock data, quick-test creates session and navigates. **Gap**: Spec 4.1 says dashboard should show server uptime and recording status — only session/client/template counts are shown. No `getServerStatus()` call despite API function existing in `client.ts`. No auto-refresh/polling as spec mentions.

### 7.4 Template Management
- [x] List view: table of templates with name, roles, mapping count, default flag `666d835`
- [x] Editor view: name, default toggle, roles (with multi_client toggle), mappings editor `666d835`
- [x] Validation: multi-client roles can't be mapping sources (inline error) `666d835`

**Test (Vitest)**: Template editor rejects invalid mapping (multi-client source). Create/edit/delete flows render correctly.

> **Review**: All implemented. `TemplateListPage` shows table with Name, Roles (count), Mappings (count), Default badge, and Edit/Delete actions. `TemplateEditorPage` has name input, default toggle, role list with multi_client toggles, and mapping editor with role/channel dropdowns and record/broadcast target types. `validateTemplate()` checks: roles referenced in mappings must exist, multi-client roles can't be mapping sources. `TemplateListPage.test.tsx` (4 tests): list renders, default badge shown, navigate to create, delete with confirm. `TemplateEditorPage.test.tsx` (3 tests): new form renders, existing template loads, multi-client source rejection validated. Good coverage.

### 7.5 Session Management
- [x] List view: sessions with status, client count, actions `ef2bcb0`
- [x] Detail view: connected clients per role, channel binding status `ef2bcb0`
- [x] End session button `ef2bcb0`

**Test (Vitest)**: Session list renders from mock data. End session calls DELETE API.

> **Review**: `SessionListPage` shows table with Name, Template, Status (badge), Clients (count/total), Created, and Connect/End actions. Includes inline create-session form. `SessionDetailPage` shows session metadata, connected clients table (ID, Role, Status, Connected Since), channel bindings table (Source, Target, Active status), Connect and End Session buttons. `SessionListPage.test.tsx` (2 tests): list renders from mock data, end session calls API. **Gap**: No test file for `SessionDetailPage` — detail view is untested. Spec 4.2 also mentions "Connect" and "End Session" actions on detail view — the UI has them but they're not tested.

### 7.6 Session Connect View
- [ ] Browser WebRTC connection via `/ws/signaling`
- [ ] Mic device selector (`getUserMedia` with device enumeration)
- [ ] VU meter per incoming audio channel (Web Audio API `AnalyserNode`)
- [ ] Volume control per channel (gain slider)
- [ ] WebRTC debug panel (`RTCPeerConnection.getStats()`)
- [ ] Session logs panel (control channel `LogEntry` messages from all clients)
- [ ] Send `JoinSession` on connect, send audio as source track

**Test (Vitest)**: Session connect view renders audio controls, debug panel, log panel. Mock WebRTC API interactions.

> **Review**: The UI shell exists with the correct layout (audio panel left, debug panel right, logs bottom) but **all functionality is placeholder/static**:
> - **No WebRTC connection** — no WebSocket to `/ws/signaling`, no `RTCPeerConnection` created, no `JoinSession` message sent.
> - **Mic selector** — UI exists with device `<select>`, `getUserMedia` is called for permission, `enumerateDevices` populates the dropdown. **However**, selected device is never connected to a WebRTC track. Mute button toggles state but doesn't affect any stream.
> - **VU meter** — UI bar exists but is hardcoded to 0%. No `AnalyserNode`, no Web Audio API usage.
> - **Volume controls** — Section exists as placeholder text ("Connect to a session to see volume controls"). No actual `GainNode` or sliders per channel.
> - **WebRTC debug panel** — Renders all stat fields (ICE state, candidates, bytes, packet loss, jitter, RTT) but all values are static zeros from `useState`. No `getStats()` polling.
> - **Session logs** — Renders log entries with severity filtering, but only contains one hardcoded "Waiting for WebRTC connection..." entry. No data channel subscription.
> - **Tests** (`SessionConnectPage.test.tsx`, 2 tests): Only verify that DOM elements render (testids exist) and end-session button is present. **No mock WebRTC API interactions** as the acceptance criteria require. Tests don't validate any functional behavior.

### 7.7 Quick-Test Flow
- [x] Dashboard button creates session from default template, redirects to connect as `translator` `6a52aae`
- [ ] Auto-request mic permission on connect
- [x] End session button in connect view `1a0bda8`

**Test (Vitest)**: Quick-test button calls API, redirects to connect view with `role=translator`.

> **Review**: Dashboard quick-test button works: creates session via `POST /api/sessions` with default template ID and auto-generated name, navigates to `/sessions/:id/connect?role=translator`. Button is disabled with tooltip when no default template exists (matches spec 4.4 requirement). End session button is present in the connect view. **Gap**: Mic permission is requested in `SessionConnectPage` `useEffect` unconditionally (not specific to quick-test flow), but since the connect page always tries `getUserMedia`, this effectively works. The spec says "auto-request mic permission on connect" which the page does attempt. However, the mic stream is never actually used for WebRTC, so "auto-request" is only partial — permission is asked but the audio goes nowhere. `DashboardPage.test.tsx` validates the API call and redirect correctly.

## Exit Criteria

1. `task build:web` produces `web/dist/` that's embeddable — **MET** ✓
2. `task lint:web` and `task test:unit:web` pass — **PARTIALLY MET** (tests pass: 8 files, 22 tests all green; lint fails with 2 errors)
3. All pages render and interact correctly with mocked API data — **MOSTLY MET** (all pages render, session connect view interactions are placeholder-only)
4. Session connect view can establish WebRTC connection to a real server (manual verification or Playwright in Phase 8) — **NOT MET** (no WebRTC implementation exists)

## Spec Updates

- 4.1 Auth & Dashboard → 3
- 4.2 Management → 3
- 4.3 Session Connect View → 3
- 4.4 Quick-Test Flow → 3

## Summary of Gaps

| Area | Gap | Severity |
|------|-----|----------|
| 7.1 Lint | 2 ESLint errors (`react-refresh/only-export-components`, `react-hooks/set-state-in-effect`) | Medium |
| 7.1 API client | Hand-written API client, no OpenAPI generation pipeline | Low (functional but not to spec) |
| 7.3 Dashboard | Missing server uptime/recording status display, no auto-refresh | Low |
| 7.5 Detail test | `SessionDetailPage` has no test file | Medium |
| 7.6 WebRTC | No WebRTC connection, signaling, or `JoinSession` — entire real-time layer missing | **High** |
| 7.6 Audio | VU meters hardcoded to 0%, no `AnalyserNode` or `GainNode` | **High** |
| 7.6 Debug | Stats panel shows static zeros, no `getStats()` polling | High |
| 7.6 Logs | Hardcoded placeholder log, no data channel subscription | High |
| 7.6 Tests | Tests only check DOM element existence, no mock WebRTC interactions | High |
| 7.7 Mic | Permission requested but stream unused (no WebRTC track) | High (depends on 7.6) |
