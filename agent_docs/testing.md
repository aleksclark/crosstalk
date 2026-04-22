# Testing

[← Back to AGENTS.md](../AGENTS.md)

Core philosophy: **prove real functionality, not mocks**. Agent-driven development makes it easy to claim something works when it doesn't. Every feature needs a test that exercises it for real.

## Three Tiers

| Tier | Mocks OK? | What it proves |
|------|-----------|----------------|
| **Unit** | Yes | Individual functions/packages work in isolation |
| **Integration** | No | Components work together with real services |
| **E2E** | No | Actual audio flows through the full system |

## Unit Tests

### Go

- `testify` for assertions (`assert`, `require`)
- `mock/` subpackage for service mocks (function injection, not a library)
- Test files adjacent to source: `foo.go` → `foo_test.go`
- Run: `task test:unit:go`

**What to test**:
- Domain logic (validation, routing resolution)
- Handler behavior (using mock services)
- Error paths
- Protobuf round-trips

**What NOT to test at unit level**:
- Database queries (that's integration)
- WebRTC connections (that's integration)
- "Does the server start" (that's integration)

### TypeScript

- Vitest with jsdom environment
- Test files adjacent to source: `App.tsx` → `App.test.tsx`
- Run: `task test:unit:web`

## Writing a Mock-Based Test

```go
package http_test

import (
    "testing"
    "net/http/httptest"

    "github.com/aleksclark/crosstalk/server/mock"
    crosstalkhttp "github.com/aleksclark/crosstalk/server/http"
)

func TestGetUser(t *testing.T) {
    var us mock.UserService
    us.FindUserByIDFn = func(id string) (*crosstalk.User, error) {
        return &crosstalk.User{ID: id, Username: "alice"}, nil
    }

    var h crosstalkhttp.Handler
    h.UserService = &us

    w := httptest.NewRecorder()
    r := httptest.NewRequest("GET", "/api/users/123", nil)
    h.ServeHTTP(w, r)

    if !us.FindUserByIDInvoked {
        t.Fatal("expected FindUserByID to be called")
    }
    // assert response...
}
```

## When to Write Tests

- **Before marking a task done** — if there's no test, it's not done
- **When fixing a bug** — write the failing test first, then fix
- **When adding domain logic** — test it directly on the domain type, no mocks needed
- **When adding a handler** — test it with mock services injected

## Integration Tests

Run in Docker Compose with real SQLite, real Pion, real HTTP. No mocks.

- Server binary includes embedded web UI
- Playwright for browser-based UI tests
- `task test:integration` handles lifecycle (build, up, run, teardown)

## E2E / Golden Tests

Real audio through real hardware (K2B board). Compares input/output audio via cross-correlation.

- Requires K2B board on the network
- `task test:e2e`
- Not for every code change — run before releases or after WebRTC/audio changes
