# Go Patterns

[← Back to AGENTS.md](../AGENTS.md)

## Standard Package Layout

This project follows [Ben Johnson's Standard Package Layout](https://medium.com/@benbjohnson/standard-package-layout-7cdbc8391fc1). Four rules:

### 1. Root package is for domain types

`server/domain.go` and `cli/domain.go` contain **only** structs, interfaces, constants, and pure functions. No external dependencies — not even `database/sql` or `net/http`. Only stdlib types with no I/O (`time`, `errors`, `strings`).

```go
package crosstalk

type User struct { ... }
type UserService interface {
    FindUserByID(id string) (*User, error)
    CreateUser(user *User) error
}
```

Domain logic that depends only on other domain types belongs here too (e.g., `SessionTemplate.Validate()`).

### 2. Subpackages grouped by dependency

Each subpackage wraps **one** external dependency and implements domain interfaces:

| Package | Wraps | Implements |
|---------|-------|------------|
| `sqlite/` | `database/sql` + goose | `UserService`, `TokenService`, `SessionService`, etc. |
| `http/` | `net/http` | REST API handlers, web UI serving |
| `ws/` | WebSocket library | WebRTC signaling |
| `pion/` | `github.com/pion/webrtc` | Media track forwarding |
| `pipewire/` (CLI) | PipeWire D-Bus/CLI | Source/sink discovery |

**Critical**: subpackages never import each other. They communicate only through domain interfaces. `http.Handler` holds a `crosstalk.UserService` — it never imports `sqlite/`.

```go
package sqlite

type UserService struct {
    DB *sql.DB
}

func (s *UserService) FindUserByID(id string) (*crosstalk.User, error) {
    // SQL query here — this is the only place that knows about SQL
}
```

When adding a new external dependency, create a new subpackage named after that dependency. Don't put it in an existing package.

### 3. Mock subpackage

`server/mock/` has hand-written mocks using function injection. No mocking libraries.

Pattern for each service interface:

```go
type UserService struct {
    FindUserByIDFn      func(id string) (*crosstalk.User, error)
    FindUserByIDInvoked bool
}

func (s *UserService) FindUserByID(id string) (*crosstalk.User, error) {
    s.FindUserByIDInvoked = true
    return s.FindUserByIDFn(id)
}
```

In tests:

```go
var us mock.UserService
us.FindUserByIDFn = func(id string) (*crosstalk.User, error) {
    if id != "123" {
        t.Fatalf("unexpected id: %s", id)
    }
    return &crosstalk.User{ID: "123", Username: "alice"}, nil
}

handler.UserService = &us
// ... exercise handler ...

if !us.FindUserByIDInvoked {
    t.Fatal("expected FindUserByID to be called")
}
```

When adding a new service interface to `domain.go`, add the corresponding mock to `mock/mock.go` in the same commit.

### 4. Main package wires dependencies

`cmd/ct-server/main.go` is the only place where concrete implementations are assigned to domain interfaces:

```go
func run() error {
    db := sqlite.Open(cfg.DBPath)
    var userService crosstalk.UserService = &sqlite.UserService{DB: db}

    var h http.Handler
    h.UserService = userService
    // ...
}
```

Config loading, flag parsing, and signal handling also live here.

## Logging

Use `log/slog` with JSON handler. Never `fmt.Println` for operational output.

```go
slog.Info("channel bound",
    "channel_id", channelID,
    "track_id", trackID,
    "role", role,
)
```

Always use structured fields. Never concatenate values into the message string.

## Error Handling

- Return `error` from all fallible operations
- Wrap errors with context: `fmt.Errorf("finding user %s: %w", id, err)`
- Use `ValidationError` (defined in `domain.go`) for domain validation failures
- Never silently ignore errors

## Adding a New Feature

1. Read the relevant `spec/` section
2. Add/modify domain types and interfaces in `domain.go`
3. Add mock implementations in `mock/mock.go`
4. Implement in the appropriate dependency subpackage
5. Wire in `cmd/ct-server/main.go` (or `cmd/ct-client/main.go`)
6. Write tests — domain logic tests in root package, handler tests use mocks
7. Run `task test:unit:go` and `task lint:go`
