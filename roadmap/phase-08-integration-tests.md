# Phase 8: Integration Tests

[← Roadmap](index.md)

**Status**: `PARTIAL — 2/4 sections done (8.1 partial, 8.2 done, 8.3 stubs only, 8.4 partial)`  
**Depends on**: Phase 5 (sessions + audio forwarding) + Phase 7 (web UI)

Full-stack tests with real services in Docker. No mocks. Playwright for browser tests.

## Tasks

### 8.1 Docker Compose Test Environment
- [x] `test/docker-compose.integration.yml` with server service (bridge networking) — `336658d`
- [ ] `test/Dockerfile.test` with Go test runner + Playwright
  > **GAP**: `docker-compose.integration.yml:27` references `test/Dockerfile.test` but this file does not exist. Only `test/Dockerfile.server` is present. The compose file cannot `docker compose up` without this file.
- [x] Server uses embedded web UI (`web.dev_mode: false`) — `test/test-config.json` confirms `"dev_mode": false` — `336658d`
- [x] `POST /api/test/reset` endpoint (test mode only) to wipe DB between tests — `ae3fdf3`
  > Implemented in `server/http/handler.go` (Handler.TestMode + handleTestReset). Truncates tables in FK-safe order. Validated by `TestIntegration_TestResetEndpoint` which confirms 204 response and token invalidation.

**Test**: `docker compose up` starts server, test runner can reach it.
> **VERDICT**: Cannot run. `Dockerfile.test` is missing, so `docker compose up` will fail. The server Dockerfile and compose config are structurally correct, but the test-runner image cannot be built.

### 8.2 Go API Integration Tests
- [x] Test: create user → create token → authenticate → CRUD templates → CRUD sessions — `ae3fdf3`
  > `TestIntegration_FullCRUD` (all pass): Creates user, creates token, authenticates with new token, creates/lists/updates templates, creates/gets/deletes sessions, verifies session status transitions (waiting → ended). Comprehensive.
- [x] Test: create session → connect two Pion clients → verify audio forwarding → end session — `ae3fdf3`
  > `TestIntegration_SessionWithAudioForwarding` (passes): Two WebRTC clients connect, orchestrator activates binding, 20 Opus RTP packets sent. **Note**: Track delivery to Client B via SFU times out (renegotiation doesn't propagate in in-process test); test falls back to verifying orchestrator state via REST API. The actual RTP payload comparison described in Phase 5 exit criteria is not validated here — test confirms orchestrator wiring but not end-to-end audio delivery to the receiving client.
- [x] Test: create session with `→ record` → send audio → end → verify recording file exists — `ae3fdf3`
  > `TestIntegration_SessionWithRecording` (passes): Sends 100 Opus silence frames (~2s), ends session via `orch.EndSession()`, verifies OGG file + `session-meta.json` exist, validates with `ffprobe` (confirms Ogg format, ~2s duration, Opus codec).
- [x] Test: role cardinality — second client to single-client role gets rejected — `ae3fdf3`
  > `TestIntegration_RoleCardinality` (passes): Client A joins "translator" role successfully (SESSION_CLIENT_JOINED), Client B attempts same role and receives SESSION_ROLE_REJECTED with message containing "single-client".

**Test**: `test/integration/*_test.go` — all pass against real server with real SQLite.
> **VERDICT**: All 5 Go integration tests pass (including TestResetEndpoint). Tests live in `server/cmd/ct-server/integration_test.go` (not `test/integration/` as spec describes). Tests use in-process server with real SQLite + Pion, no Docker required.

### 8.3 Playwright Web UI Tests
- [ ] Test: login flow → enter creds → redirected to dashboard
  > **GAP**: `test/playwright/specs/login.spec.ts` has 3 test cases (page loads, valid login redirects, invalid login shows error) but these have never been run against a real server — `Dockerfile.test` doesn't exist so there's no way to run them in the Docker environment. The tests themselves are structurally sound but **unvalidated**. Login credentials are hardcoded (`admin`/`admin-password`) but it's unclear if the server seeds this user or if the test runner should create it.
- [ ] Test: template CRUD → create → edit → delete via UI
  > **GAP**: `test/playwright/specs/templates.spec.ts` — all 3 tests are `test.skip()` with empty bodies (only TODO comments). No implementation.
- [ ] Test: create session → verify appears in list with `waiting` status
  > **GAP**: `test/playwright/specs/sessions.spec.ts` — all 3 tests are `test.skip()` with empty bodies. No implementation.
- [ ] Test: session connect view → mic selector renders, VU meter element exists, WebRTC debug panel populated
  > **GAP**: No test exists for this. The sessions.spec.ts has a skipped stub for "session connect view with audio controls" but the body is empty.
- [ ] Test: quick-test → button creates session + redirects to connect view
  > **GAP**: No test file or stub exists for the quick-test flow at all.

**Test**: `test/playwright/*` — all pass headless Chromium against real server.
> **VERDICT**: Not met. Only login.spec.ts has real test code (unvalidated). Template and session specs are empty stubs. Quick-test has no coverage. Playwright config (`playwright.config.ts`) is properly set up with Chromium, fake media devices, and `CT_SERVER_URL` support.

### 8.4 task test:integration
- [ ] `task test:integration` runs full lifecycle: build → docker up → run tests → docker down
  > **GAP**: `task test:integration` exists in `Taskfile.yml:150` but runs Go integration tests in-process (`go test -run TestIntegration`), not via Docker. The Docker-based commands are commented out. This is a pragmatic shortcut that works for Go tests but cannot run Playwright tests.
- [ ] Clean exit: always tear down containers, even on test failure
  > **GAP**: No container teardown logic since Docker isn't used. The commented-out Docker commands don't include `|| true` or trap-based cleanup for failure cases.

**Test**: `task test:integration` exits 0 when all tests pass, exits non-zero on failure, containers are always cleaned up.
> **VERDICT**: `task test:integration` does exit 0 when Go tests pass and non-zero on failure. But it does not exercise the Docker lifecycle or Playwright tests. No container cleanup is needed or implemented since containers aren't used.

## Exit Criteria

`task test:integration` passes:
1. All Go API integration tests (CRUD, sessions, audio forwarding, recording) — **MET** ✓ (5/5 tests pass)
2. All Playwright tests (login, template CRUD, session creation, connect view) — **NOT MET** ✗ (6/9 tests are empty stubs, 3 login tests unvalidated, quick-test missing entirely)
3. Containers cleaned up automatically — **NOT MET** ✗ (Docker not used; `Dockerfile.test` missing)

**Overall: Exit criteria NOT met.** Go integration tests are solid and passing. Docker environment is partially built (missing `Dockerfile.test`). Playwright tests are mostly empty stubs awaiting Phase 7 web UI completion.

## Spec Updates

- 7.2 Integration Tests → 5
  > **Recommended**: 7.2 → 3 (Go tests validated, but Docker + Playwright are stubs/broken)
- 2.3 Session Orchestration → 7
  > Reasonable — orchestrator wiring confirmed by integration tests (join, forward, record, cardinality)
- 4.1-4.4 Admin Web → 5
  > **Recommended**: Keep at current score — no Playwright validation of web UI exists
