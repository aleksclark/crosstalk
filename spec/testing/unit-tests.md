# Unit Tests

[← Back to Index](../index.md) · [Testing Overview](overview.md)

---

## Scope

Fast, isolated tests for individual functions and packages. Mocks/stubs are acceptable here for speed.

## Go (Server + CLI)

- **Framework**: [testify](https://github.com/stretchr/testify) — `assert`, `require`, `suite`, `mock`
- **Convention**: Test files adjacent to source (`foo.go` → `foo_test.go`)
- **Run**: `task test:unit:go` (or `go test ./...` from the respective module root)

### What to Unit Test

| Area | Examples |
|------|----------|
| Domain logic | Session template validation, mapping resolution |
| Auth | Token hashing, validation, expiry checks |
| Protobuf | Serialize/deserialize round-trips |
| Channel routing | Given template + connected roles, which bindings activate? |
| Config parsing | Env var parsing, config file loading |

### Mocking Policy

- Mock external I/O: database, WebRTC, PipeWire, network
- Don't mock domain logic — test it directly
- Use the `mock/` subpackage with hand-written function-injection mocks (per standard package layout)
- No external mocking libraries — mocks use `XxxFn` fields for behavior and `XxxInvoked` booleans for verification

### Example Pattern

```go
// session/routing_test.go
func TestRoutingActivatesBindingsForConnectedRoles(t *testing.T) {
    template := domain.SessionTemplate{
        Roles: []string{"translator", "studio"},
        Mappings: []domain.Mapping{
            {Source: "translator:mic", Sink: "studio:output"},
            {Source: "studio:input", Sink: "translator:speakers"},
        },
    }

    // Only translator connected
    connected := map[string]bool{"translator": true}

    bindings := session.ResolveBindings(template, connected)

    // No bindings should activate — studio isn't connected
    assert.Empty(t, bindings)
}
```

## TypeScript (Web UI)

- **Framework**: Vitest (bundled with Vite)
- **Convention**: `*.test.ts` or `*.test.tsx` adjacent to source
- **Run**: `task test:unit:web` (or `pnpm test` in `web/`)

### What to Unit Test

| Area | Examples |
|------|----------|
| Components | Render, user interaction (React Testing Library) |
| Hooks | Custom hooks for WebRTC, audio processing |
| Utils | Channel name parsing, template validation |
| API client | Request formation (mock fetch) |

## Coverage Targets

No hard percentage — focus on testing behavior that matters:
- All domain logic paths
- All error handling paths
- All validation rules
- Protobuf round-trips
