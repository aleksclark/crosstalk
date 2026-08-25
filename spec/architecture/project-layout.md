# Project Layout

[← Back to Index](../index.md) · [Architecture Overview](overview.md)

---

## Repository Structure

Follows [Ben Johnson's Standard Package Layout](https://medium.com/@benbjohnson/standard-package-layout-7cdbc8391fc1) for Go code. Monorepo containing all components.

The four tenets:
1. **Root package is for domain types** — only data types and service interfaces, no external dependencies
2. **Group subpackages by dependency** — each subpackage wraps exactly one external dependency
3. **Shared mock subpackage** — hand-written function-injection mocks for all service interfaces
4. **Main package ties together dependencies** — wires concrete implementations to domain interfaces

```
crosstalk/
├── spec/                    # This specification
├── proto/                   # Protobuf definitions
│   └── crosstalk/
│       └── v1/
│           └── control.proto
├── abc/                     # Published ABC transport module
│   ├── go.mod               # github.com/aleksclark/crosstalk/abc
│   └── internal/controlv2/  # Generated v2 proto (distinct package)
├── server/                  # Go server
│   ├── domain.go            # Domain types + service interfaces (no deps)
│   ├── cmd/
│   │   └── ct-server/       # Main package — wires dependencies
│   │       └── main.go
│   ├── sqlite/              # Wraps database/sql + goose
│   ├── http/                # Wraps net/http — REST API + web UI serving
│   ├── ws/                  # Wraps websocket — WebRTC signaling
│   ├── pion/                # Wraps pion/webrtc — media track forwarding
│   ├── mock/                # Function-injection mocks for testing
│   ├── config.schema.json   # JSON Schema for server config
│   ├── go.mod
│   └── go.sum
├── cli/                     # Go CLI client
│   ├── domain.go            # Domain types (no deps)
│   ├── cmd/
│   │   └── crosstalk/       # Main package — wires dependencies
│   │       └── main.go
│   ├── pipewire/            # Wraps PipeWire (D-Bus / pw-cli)
│   ├── pion/                # Wraps pion/webrtc — client-side WebRTC
│   ├── config.schema.json   # JSON Schema for CLI config
│   ├── go.mod
│   └── go.sum
├── web/                     # Admin Web UI
│   ├── src/
│   │   ├── components/
│   │   ├── pages/
│   │   ├── lib/
│   │   │   └── api/         # Generated TypeScript client
│   │   └── main.tsx
│   ├── dist/                # Production build output (go:embed source)
│   ├── package.json
│   ├── vite.config.ts
│   └── tsconfig.json
├── k2b-board/               # Hardware deployment scripts + image
├── dev/                     # Dev environment configs
│   ├── docker-compose.yml
│   ├── Dockerfile.server
│   └── scripts/
├── .tool-versions           # asdf version pins (Go, Node)
└── Taskfile.yml             # go-task: build, dev, test, lint, deploy
```

## Package Design

### Root package (`server/`, `cli/`)

Contains **only** domain types (structs) and service interfaces. Zero external dependencies — only `time`, `errors`, and other stdlib types that have no I/O. This is the shared language of the application.

```go
package crosstalk

type User struct { ... }
type UserService interface { ... }
type SessionTemplate struct { ... }
type SessionTemplateService interface { ... }
```

### Dependency subpackages (`sqlite/`, `http/`, `pion/`, `ws/`, `pipewire/`)

Each subpackage wraps exactly one external dependency and implements domain interfaces:

- **`sqlite/`** wraps `database/sql` + goose → implements `UserService`, `TokenService`, `SessionService`, etc.
- **`http/`** wraps `net/http` → REST API handlers + embedded web UI serving (via `go:embed`)
- **`ws/`** wraps a WebSocket library → WebRTC signaling endpoints
- **`pion/`** wraps `github.com/pion/webrtc` → media track forwarding
- **`pipewire/`** (CLI only) wraps PipeWire D-Bus/CLI → source/sink discovery

Dependencies between subpackages communicate through domain interfaces, never directly. For example, `http.Handler` holds a `crosstalk.UserService` field — it doesn't import `sqlite/` directly.

### Mock subpackage (`mock/`)

Hand-written mocks using function injection (not a mocking library):

```go
package mock

type UserService struct {
    FindUserByIDFn      func(id string) (*crosstalk.User, error)
    FindUserByIDInvoked bool
}

func (s *UserService) FindUserByID(id string) (*crosstalk.User, error) {
    s.FindUserByIDInvoked = true
    return s.FindUserByIDFn(id)
}
```

Tests inject behavior via `XxxFn` and verify calls via `XxxInvoked`. No external mocking dependencies.

### Main package (`cmd/ct-server/`, `cmd/ct-client/`)

The only place where concrete implementations are wired to domain interfaces:

```go
func run() error {
    db := sqlite.Open(cfg.DBPath)
    defer db.Close()

    var userService crosstalk.UserService = &sqlite.UserService{DB: db}

    var handler http.Handler
    handler.UserService = userService

    // start server...
}
```

Main is also an adapter — it connects the terminal (flags, env, signals) to the domain.

## Other Conventions

**Web UI is embedded in the server binary**:
- `web/dist/` is the Vite production build output
- `http/` package uses `go:embed` to bundle `web/dist/` into the binary
- In dev mode, `http/` reverse-proxies to Vite dev server instead

**Configuration**:
- `server/config.schema.json` and `cli/config.schema.json` define the config format
- Config files are JSON with a `$schema` reference for editor support
- Config loading happens in `cmd/` (main package), not in a config subpackage

**Generated code** lives alongside the source that consumes it:
- Go Protobuf types → `proto/gen/go/`
- TypeScript Protobuf types → `proto/gen/ts/`
- TypeScript API client → `web/src/lib/api/`
- OpenAPI spec → `server/http/openapi.json` (generated on build)
- ABC transport v2 control codec → `abc/internal/controlv2/`

**ABC transport module (`abc/`)**:
- Standalone Go module `github.com/aleksclark/crosstalk/abc`
- Owns signaling, ICE/SDP, protobuf-v2 control, and RTP tracks
- Device-independent: no capture, playback, or encoder processes
- Consumed by `ct-abc` and external clients (Qol) that pin a commit
- Generated v2 lives in `abc/internal/controlv2` with proto package
  `abc.internal.controlv2` so it can link beside `server/proto/v2`
  (`crosstalk.v2`). Wire field numbers stay in sync via
  `task generate:proto:v2`.
- In-tree `cli/` and `server/` may `replace` the module with `../abc`.
  External consumers must not depend on gitignored `proto/gen/go`.

**Version management**:
- `.tool-versions` pins Go and Node versions for asdf
- pnpm for Node package management
- Go modules for Go dependency management
