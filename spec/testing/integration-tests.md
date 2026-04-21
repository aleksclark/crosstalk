# Integration Tests

[← Back to Index](../index.md) · [Testing Overview](overview.md)

---

## Scope

Tests that exercise multiple components together using real services — real SQLite, real Pion, real HTTP. No mocks at this level.

## Environment

A separate Docker Compose setup dedicated to integration testing:

```yaml
# test/docker-compose.integration.yml
services:
  server:
    build: { context: .., dockerfile: dev/Dockerfile.server }
    environment:
      - CROSSTALK_CONFIG=/app/test.json
    networks:
      test-net:

  test-runner:
    build: { context: .., dockerfile: test/Dockerfile.test }
    depends_on: [server]
    networks:
      test-net:

networks:
  test-net:
    driver: bridge  # bridge is fine for test isolation
```

The server binary includes the embedded web UI via `go:embed`, so no separate web container is needed. The test server uses a config with `web.dev_mode: false` (production mode).

## Setup / Teardown

- **Per-suite**: `task test:integration` handles full lifecycle (build, up, run, down)
- **Per-test**: Server exposes a `POST /api/test/reset` endpoint (only in test mode) that wipes DB and resets state
- No state leaks between tests — each test starts clean

## Go Integration Tests

Tests that hit the real HTTP API and exercise the full server stack:

```go
// test/integration/session_test.go
func TestCreateSessionAndJoinAsRole(t *testing.T) {
    client := api.NewClient(serverURL, adminToken)

    // Create template
    tmpl, err := client.CreateTemplate(api.CreateTemplateRequest{...})
    require.NoError(t, err)

    // Create session
    sess, err := client.CreateSession(api.CreateSessionRequest{TemplateID: tmpl.ID})
    require.NoError(t, err)
    assert.Equal(t, "waiting", sess.Status)

    // Simulate WebRTC client joining
    // (use Pion client library for real WebRTC connection)
    ...
}
```

## Playwright Tests (Web UI)

[Playwright](https://playwright.dev/) for browser-based integration tests:

### What to Test

| Scenario | Description |
|----------|-------------|
| Login flow | Enter credentials, verify redirect to dashboard |
| Template CRUD | Create, edit, delete templates via the UI |
| Session creation | Create session from template, verify status |
| Session connect | Open session connect view, verify audio controls render |
| Token management | Create and revoke API tokens |

### Approach

- Tests run headless Chromium against the server (which serves the embedded web UI)
- API calls verified by checking UI state (not by sniffing network)
- Audio tests at this level can verify UI elements (VU meter moves, device selector populates) without verifying actual audio content — that's for E2E

### Run Command

```bash
task test:integration
```

Which runs:

```bash
task build
docker compose -f test/docker-compose.integration.yml run --rm test-runner
docker compose -f test/docker-compose.integration.yml down -v
```

## Cleanup

- Integration test Docker Compose is entirely separate from dev environment
- `task test:integration` handles the full lifecycle (up, run, teardown)
- CI pipeline runs these in isolated Docker-in-Docker
