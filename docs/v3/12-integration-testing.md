# Step 12: Integration & E2E Testing

## Goal

Comprehensive test suite proving the entire system works end-to-end. Three tiers: unit tests (pure logic), integration tests (real services in Docker), E2E tests (browser + hardware).

## Test Tiers

### Tier 1: Unit Tests
- Pure Go logic: mixer math, domain validation, config parsing
- Pure TS logic: API client type safety, component rendering
- Run fast, no external deps
- `task test:unit`

### Tier 2: Integration Tests
- Docker Compose: server + SQLite + test clients
- Go in-process tests: real HTTP + WebSocket + Pion
- Playwright browser tests: all three apps against real server
- `task test:integration`

### Tier 3: E2E Tests
- Real ABC hardware (or PipeWire loopback simulating it)
- Real audio through real WebRTC connections
- Verify mixed output matches expected signal
- `task test:e2e`

## Tasks

### 12.1 Unit test foundations
- [ ] Mixer unit tests: verify PCM mixing math, level scaling, muting
- [ ] Domain validation: session/channel/source constraints
- [ ] Auth tests: JWT sign/verify, role checking
- [ ] Config parsing: valid + invalid + unknown fields
- [ ] `task test:unit:go` and `task test:unit:ts`

### 12.2 Integration test infrastructure
- [ ] `test/docker-compose.yml` — server + test runner
- [ ] Test helper: create server, seed data, run assertions
- [ ] Playwright config for all three apps
- [ ] CI-compatible (headless Chrome, no GPU)

### 12.3 API integration tests
- [ ] Full CRUD lifecycle for sessions, channels, ABCs, translators
- [ ] Auth flow: login → use token → refresh → logout
- [ ] RBAC enforcement: translator can't access admin endpoints
- [ ] OpenAPI spec validation: responses match spec schemas

### 12.4 WebRTC integration tests (Go)
- [ ] Test client connects via WebSocket → SDP exchange → ICE connection
- [ ] Audio source sends packets → server receives (verified via recording)
- [ ] Multiple sources → mixer → verify mixed output
- [ ] ABC reconnection: disconnect → reconnect → same source identity
- [ ] Debug events: verify event ring buffer is populated

### 12.5 Playwright browser tests
- [ ] **Admin app**:
  - Login flow
  - Session CRUD
  - ABC management
  - Translator management
  - Mix controls (visual verification)
  - Debug panel renders events
- [ ] **Translator app**:
  - Login, see assigned sessions
  - Connect to session, verify audio indicators
  - Mute toggle
- [ ] **Broadcast app**:
  - Load with valid token → shows session info
  - Click listen → audio context activated
  - Invalid token → error state

### 12.6 E2E audio tests
- [ ] Generate known audio signal (1kHz sine)
- [ ] Send via test WebRTC client as source
- [ ] Receive via broadcast listener
- [ ] Cross-correlate input and output (correlation > 0.9)
- [ ] Verify recording file matches source audio
- [ ] Verify mix: two sources at different levels → output levels correct

### 12.7 WebRTC debug verification
- [ ] Connect a peer → verify `/api/debug/peers` shows it
- [ ] Verify ICE events are logged (at least: gathering, connected)
- [ ] Verify SDP is captured and retrievable
- [ ] Verify candidate details are stored
- [ ] Stress test: 10 simultaneous peers → all debug data accessible

### 12.8 Resilience tests
- [ ] Server restart: ABCs reconnect automatically
- [ ] Network partition: ICE restart recovers connection
- [ ] Source disconnect during recording: clean file, no corruption
- [ ] Token expiration during active session: re-auth without dropping WebRTC
- [ ] Database lock contention: concurrent writes don't deadlock (WAL mode)

## Test Infrastructure

### Docker Compose
```yaml
services:
  server:
    build: ./server
    ports: ["8080:8080"]
    environment:
      CT_CONFIG: /app/test-config.json
    volumes:
      - recordings:/data/recordings

  playwright:
    image: mcr.microsoft.com/playwright:v1.40.0
    depends_on: [server]
    volumes:
      - ./test:/tests
```

### Test Config
```json
{
  "listen": ":8080",
  "db_path": ":memory:",
  "recording_path": "/tmp/recordings",
  "log_level": "debug",
  "auth": {
    "session_secret": "test-secret-not-for-production"
  },
  "webrtc": {
    "stun_servers": ["stun:stun.l.google.com:19302"]
  }
}
```

## Acceptance Criteria

- [ ] `task test:unit` passes (< 30s)
- [ ] `task test:integration` passes (< 5min)
- [ ] All three apps tested via Playwright
- [ ] Audio round-trip verified with signal correlation
- [ ] Mix accuracy verified: 2 sources at 0.5 level → output at ~0.5 amplitude
- [ ] WebRTC debug data verified present and accurate
- [ ] Reconnection scenarios pass
- [ ] CI runs full suite on PR

## Reusable from v2

- `test/docker-compose.integration.yml` — Docker Compose pattern
- `server/cmd/ct-server/integration_test.go` — in-process integration test pattern
- `test/playwright/` — Playwright test structure, helpers, assertions
- `server/mock/` — mock implementation pattern for unit tests
