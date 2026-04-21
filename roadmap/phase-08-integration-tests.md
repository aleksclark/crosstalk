# Phase 8: Integration Tests

[← Roadmap](index.md)

**Status**: `NOT STARTED`  
**Depends on**: Phase 5 (sessions + audio forwarding) + Phase 7 (web UI)

Full-stack tests with real services in Docker. No mocks. Playwright for browser tests.

## Tasks

### 8.1 Docker Compose Test Environment
- [ ] `test/docker-compose.integration.yml` with server service (bridge networking)
- [ ] `test/Dockerfile.test` with Go test runner + Playwright
- [ ] Server uses embedded web UI (`web.dev_mode: false`)
- [ ] `POST /api/test/reset` endpoint (test mode only) to wipe DB between tests

**Test**: `docker compose up` starts server, test runner can reach it.

### 8.2 Go API Integration Tests
- [ ] Test: create user → create token → authenticate → CRUD templates → CRUD sessions
- [ ] Test: create session → connect two Pion clients → verify audio forwarding → end session
- [ ] Test: create session with `→ record` → send audio → end → verify recording file exists
- [ ] Test: role cardinality — second client to single-client role gets rejected

**Test**: `test/integration/*_test.go` — all pass against real server with real SQLite.

### 8.3 Playwright Web UI Tests
- [ ] Test: login flow → enter creds → redirected to dashboard
- [ ] Test: template CRUD → create → edit → delete via UI
- [ ] Test: create session → verify appears in list with `waiting` status
- [ ] Test: session connect view → mic selector renders, VU meter element exists, WebRTC debug panel populated
- [ ] Test: quick-test → button creates session + redirects to connect view

**Test**: `test/playwright/*` — all pass headless Chromium against real server.

### 8.4 task test:integration
- [ ] `task test:integration` runs full lifecycle: build → docker up → run tests → docker down
- [ ] Clean exit: always tear down containers, even on test failure

**Test**: `task test:integration` exits 0 when all tests pass, exits non-zero on failure, containers are always cleaned up.

## Exit Criteria

`task test:integration` passes:
1. All Go API integration tests (CRUD, sessions, audio forwarding, recording)
2. All Playwright tests (login, template CRUD, session creation, connect view)
3. Containers cleaned up automatically

## Spec Updates

- 7.2 Integration Tests → 5
- 2.3 Session Orchestration → 7
- 4.1-4.4 Admin Web → 5
