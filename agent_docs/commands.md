# Commands

[← Back to AGENTS.md](../AGENTS.md)

All workflows use [go-task](https://taskfile.dev/). The `Taskfile.yml` is at the repo root.

The binary may be `task` or `go-task` depending on the system. Check with `which task || which go-task`.

## Everyday Commands

| Command | What it does |
|---------|-------------|
| `task test:unit:go` | Run Go tests for server + cli |
| `task test:unit:web` | Run Vitest for web UI |
| `task build:server` | Build `bin/ct-server` |
| `task build:cli` | Build `bin/ct-client` |
| `task lint:go` | golangci-lint on both Go modules |
| `task lint:web` | ESLint + typecheck on web/ |

## After Changing Code

**Go changes**: `task test:unit:go && task lint:go`

**Web changes**: `task test:unit:web && task lint:web`

**Protobuf changes**: `task generate:proto` then rebuild

**Both**: `task test`

## Full Task List

### Development
| Task | Description |
|------|-------------|
| `task dev` | Start Vite + server container in parallel |
| `task dev:vite` | Vite dev server on host (port 5173) |
| `task dev:server` | Server container via Docker Compose |
| `task dev:down` | Stop dev environment |
| `task dev:reset` | Stop + wipe volumes (DB, recordings) |

### Build
| Task | Description |
|------|-------------|
| `task build` | Full production build (web + server with embedded UI) |
| `task build:web` | `pnpm build` in web/ |
| `task build:server` | `go build` server (embeds web/dist/) |
| `task build:cli` | `go build` CLI client |
| `task build:cli-arm64` | Cross-compile CLI for ARM64 (K2B board) |

### Code Generation
| Task | Description |
|------|-------------|
| `task generate` | All code generation |
| `task generate:proto` | Protobuf → Go + TypeScript types |

### Test
| Task | Description |
|------|-------------|
| `task test` | All tests |
| `task test:unit` | Unit tests (Go + web) |
| `task test:unit:go` | Go tests only |
| `task test:unit:web` | Vitest only |
| `task test:integration` | Integration tests in Docker |
| `task test:e2e` | E2E golden tests (needs K2B board) |

### Deploy
| Task | Description |
|------|-------------|
| `task deploy:k2b` | Build ARM64 + deploy to K2B |
| `task deploy:k2b:watch` | Watch + auto-deploy on change |
| `task deploy:k2b:test` | Audio test harness on K2B |

### Utility
| Task | Description |
|------|-------------|
| `task clean` | Remove bin/ and web/dist/ |
| `task setup` | Install all dependencies |

## Running Go Commands Directly

The two Go modules have separate `go.mod` files. Always `cd` into the right directory:

```bash
cd server && go test ./...    # server tests
cd cli && go test ./...       # CLI tests
cd server && go build ./cmd/ct-server  # build server
```

## Running Web Commands Directly

```bash
cd web && pnpm test run       # vitest
cd web && pnpm run lint       # eslint
cd web && pnpm run typecheck  # tsc --noEmit
cd web && pnpm dev            # vite dev server
```
