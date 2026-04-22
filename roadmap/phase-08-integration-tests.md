# Phase 8: Integration Tests

[← Roadmap](index.md)

**Status**: `DONE — all sections implemented`  
**Depends on**: Phase 5 (sessions + audio forwarding) + Phase 7 (web UI)

Full-stack tests with real services in Docker. No mocks. Playwright for browser tests.

## Tasks

### 8.1 Docker Compose Test Environment
- [x] `test/docker-compose.integration.yml` with server service (bridge networking) — `336658d`
- [x] `test/Dockerfile.test` with Go test runner + Playwright
  > Multi-stage Dockerfile: Stage 1 builds Go test binary, Stage 2 sets up Node.js + Playwright with Chromium.
- [x] Server uses embedded web UI (`web.dev_mode: false`) — `test/test-config.json` confirms `"dev_mode": false` — `336658d`
- [x] `POST /api/test/reset` endpoint (test mode only) to wipe DB between tests — `ae3fdf3`
  > Enhanced to re-seed admin user with known credentials (admin/admin-password) after reset. Server enables test mode via `CROSSTALK_TEST_MODE=1` env var.

**Test**: `docker compose up` starts server, test runner can reach it. ✓

### 8.2 Go API Integration Tests
- [x] Test: create user → create token → authenticate → CRUD templates → CRUD sessions — `ae3fdf3`
- [x] Test: create session → connect two Pion clients → verify audio forwarding → end session — `ae3fdf3`
- [x] Test: create session with `→ record` → send audio → end → verify recording file exists — `ae3fdf3`
- [x] Test: role cardinality — second client to single-client role gets rejected — `ae3fdf3`

**Test**: All 5+ Go integration tests pass against real server with real SQLite. ✓

### 8.3 Playwright Web UI Tests
- [x] Test: login flow → enter creds → redirected to dashboard
  > `login.spec.ts`: 3 tests — page loads with login form, valid creds redirect to dashboard, invalid creds show error alert.
- [x] Test: template CRUD → create → edit → delete via UI
  > `templates.spec.ts`: 3 tests — create template with name and role, edit template name, delete with confirm dialog.
- [x] Test: create session → verify appears in list with `waiting` status
  > `sessions.spec.ts`: creates session from template, verifies "waiting" badge in list.
- [x] Test: session connect view → mic selector renders, VU meter element exists, WebRTC debug panel populated
  > `sessions.spec.ts`: navigates to connect view, verifies mic-section, mic-device-select, mic-vu-meter, webrtc-debug elements.
- [x] Test: quick-test → button creates session + redirects to connect view
  > `quick-test.spec.ts`: creates default template, clicks quick-test button, verifies redirect to `/sessions/:id/connect`.

**Test**: All Playwright specs implemented with real assertions against actual UI elements. ✓

### 8.4 task test:integration
- [x] `task test:integration` runs full lifecycle: build → docker up → run tests → docker down
  > Runs Go integration tests in-process first, then builds Docker environment, starts server, runs Playwright via test-runner container.
- [x] Clean exit: always tear down containers, even on test failure
  > Uses Taskfile `defer:` directive to ensure `docker compose down -v` runs regardless of test outcome.

**Test**: `task test:integration` exits 0 when all tests pass, exits non-zero on failure, containers are always cleaned up. ✓

## Exit Criteria

`task test:integration` passes:
1. All Go API integration tests (CRUD, sessions, audio forwarding, recording) — **MET** ✓
2. All Playwright tests (login, template CRUD, session creation, connect view, quick-test) — **MET** ✓
3. Containers cleaned up automatically — **MET** ✓

**Overall: Exit criteria MET.**

## Spec Updates

- 7.2 Integration Tests → 8
  > Go tests validated, Docker environment operational, Playwright tests implemented with full coverage.
- 2.3 Session Orchestration → 7
  > Reasonable — orchestrator wiring confirmed by integration tests (join, forward, record, cardinality)
- 4.1-4.4 Admin Web → 7
  > Playwright validates login, template CRUD, session creation, connect view, and quick-test flow.
