# Taskfile

[← Back to Index](../index.md) · [Dev Environment Overview](overview.md)

---

## Overview

All build, dev, test, lint, deploy, and code generation commands are defined in a [go-task](https://taskfile.dev/) `Taskfile.yml` at the repository root. No Makefile — `task` is the single entry point for all workflows.

Install: `go install github.com/go-task/task/v3/cmd/task@latest` (or via asdf/brew).

## Task Reference

Run `task --list` to see all available tasks. Below is the canonical list:

### Development

| Task | Description |
|------|-------------|
| `task dev` | Start full dev environment (Vite + server container) |
| `task dev:vite` | Start Vite dev server on host |
| `task dev:server` | Start server container via Docker Compose |
| `task dev:down` | Stop dev environment, remove containers |
| `task dev:reset` | Stop dev environment and wipe volumes (DB + recordings) |

### Build

| Task | Description |
|------|-------------|
| `task build` | Full production build (web + server binary with embedded UI) |
| `task build:web` | Build web UI (`pnpm build` → `web/dist/`) |
| `task build:server` | Build server binary (embeds `web/dist/` via `go:embed`) |
| `task build:cli` | Build CLI client binary |
| `task build:cli-arm64` | Cross-compile CLI client for ARM64 (K2B target) |

### Code Generation

| Task | Description |
|------|-------------|
| `task generate` | Run all code generation (proto + API client) |
| `task generate:proto` | Generate Go + TypeScript types from Protobuf definitions |
| `task generate:api-client` | Generate TypeScript API client from OpenAPI spec |

### Lint

| Task | Description |
|------|-------------|
| `task lint` | Run all linters |
| `task lint:go` | Run golangci-lint on server + CLI |
| `task lint:web` | Run ESLint + TypeScript type-check on web UI |

### Test

| Task | Description |
|------|-------------|
| `task test` | Run all tests (unit + integration) |
| `task test:unit` | Run all unit tests (Go + TypeScript) |
| `task test:unit:go` | Run Go unit tests (`go test ./...` for server + CLI) |
| `task test:unit:web` | Run TypeScript unit tests (Vitest) |
| `task test:integration` | Run integration tests in Docker Compose |
| `task test:e2e` | Run E2E golden tests (requires K2B board) |

### Deploy

| Task | Description |
|------|-------------|
| `task deploy:k2b` | Build CLI for ARM64 and deploy to K2B board |
| `task deploy:k2b:watch` | Watch CLI source, auto-build and deploy on change |
| `task deploy:k2b:test` | Run audio test harness on K2B board |

### Utility

| Task | Description |
|------|-------------|
| `task clean` | Remove all build artifacts (`bin/`, `web/dist/`) |
| `task setup` | Install all dependencies (Go modules, pnpm, protoc plugins) |

## Taskfile Structure

```yaml
# Taskfile.yml
version: '3'

vars:
  K2B_HOST: '{{.K2B_HOST | default "192.168.1.200"}}'
  SERVER_BIN: bin/ct-server
  CLI_BIN: bin/ct-client
  CLI_BIN_ARM64: bin/ct-client-arm64

dotenv: ['dev/.env']

tasks:
  # ── Development ──────────────────────────────────────────
  dev:
    desc: Start full dev environment (Vite + server container)
    deps: [dev:vite, dev:server]

  dev:vite:
    desc: Start Vite dev server on host
    dir: web
    cmds:
      - pnpm install
      - pnpm dev --host 0.0.0.0

  dev:server:
    desc: Start server container via Docker Compose
    cmds:
      - docker compose -f dev/docker-compose.yml up --build

  dev:down:
    desc: Stop dev environment
    cmds:
      - docker compose -f dev/docker-compose.yml down

  dev:reset:
    desc: Stop dev environment and wipe volumes
    cmds:
      - docker compose -f dev/docker-compose.yml down -v

  # ── Build ────────────────────────────────────────────────
  build:
    desc: Full production build (web + server with embedded UI)
    cmds:
      - task: build:web
      - task: build:server

  build:web:
    desc: Build web UI for production
    dir: web
    cmds:
      - pnpm install --frozen-lockfile
      - pnpm build
    sources:
      - web/src/**/*
      - web/package.json
    generates:
      - web/dist/**/*

  build:server:
    desc: Build server binary
    cmds:
      - go build -o {{.SERVER_BIN}} ./cmd/ct-server
    sources:
      - server/**/*.go
      - web/dist/**/*
    generates:
      - '{{.SERVER_BIN}}'

  build:cli:
    desc: Build CLI client binary
    cmds:
      - go build -o {{.CLI_BIN}} ./cli/cmd/crosstalk

  build:cli-arm64:
    desc: Cross-compile CLI client for ARM64 (K2B)
    env:
      GOOS: linux
      GOARCH: arm64
    cmds:
      - go build -o {{.CLI_BIN_ARM64}} ./cli/cmd/crosstalk

  # ── Code Generation ──────────────────────────────────────
  generate:
    desc: Run all code generation
    cmds:
      - task: generate:proto
      - task: generate:api-client

  generate:proto:
    desc: Generate Go + TypeScript types from Protobuf
    cmds:
      - >-
        protoc --go_out=proto/gen/go --go_opt=paths=source_relative
        --ts_out=proto/gen/ts
        proto/crosstalk/v1/*.proto
    sources:
      - proto/crosstalk/v1/*.proto
    generates:
      - proto/gen/go/**/*
      - proto/gen/ts/**/*

  generate:api-client:
    desc: Generate TypeScript API client from OpenAPI spec
    dir: web
    cmds:
      - pnpm run generate:api

  # ── Lint ─────────────────────────────────────────────────
  lint:
    desc: Run all linters
    cmds:
      - task: lint:go
      - task: lint:web

  lint:go:
    desc: Run golangci-lint on Go code
    cmds:
      - cd server && golangci-lint run ./...
      - cd cli && golangci-lint run ./...

  lint:web:
    desc: Run ESLint + TypeScript type-check
    dir: web
    cmds:
      - pnpm run lint
      - pnpm run typecheck

  # ── Test ─────────────────────────────────────────────────
  test:
    desc: Run all tests (unit + integration)
    cmds:
      - task: test:unit
      - task: test:integration

  test:unit:
    desc: Run all unit tests
    cmds:
      - task: test:unit:go
      - task: test:unit:web

  test:unit:go:
    desc: Run Go unit tests
    cmds:
      - cd server && go test ./...
      - cd cli && go test ./...

  test:unit:web:
    desc: Run TypeScript unit tests
    dir: web
    cmds:
      - pnpm test

  test:integration:
    desc: Run integration tests in Docker Compose
    cmds:
      - task: build
      - >-
        docker compose -f test/docker-compose.integration.yml
        run --rm test-runner
      - docker compose -f test/docker-compose.integration.yml down -v

  test:e2e:
    desc: Run E2E golden tests (requires K2B board)
    cmds:
      - dev/scripts/run-e2e-tests.sh

  # ── Deploy ───────────────────────────────────────────────
  deploy:k2b:
    desc: Build and deploy CLI to K2B board
    cmds:
      - task: build:cli-arm64
      - k2b-board/scripts/deploy.sh {{.K2B_HOST}} {{.CLI_BIN_ARM64}}

  deploy:k2b:watch:
    desc: Watch CLI source, auto-build and deploy to K2B on change
    cmds:
      - dev/scripts/watch-deploy-k2b.sh

  deploy:k2b:test:
    desc: Run audio test harness on K2B
    cmds:
      - dev/scripts/k2b-test.sh

  # ── Utility ──────────────────────────────────────────────
  clean:
    desc: Remove build artifacts
    cmds:
      - rm -rf bin/ web/dist/

  setup:
    desc: Install all dependencies
    cmds:
      - cd server && go mod download
      - cd cli && go mod download
      - cd web && pnpm install
```

## Conventions

- **Namespaced tasks** — use `:` to group related tasks (`build:web`, `test:unit:go`)
- **`sources` / `generates`** — go-task skips tasks when outputs are up-to-date
- **`dotenv`** — loads `dev/.env` for variables like `MACVLAN_PARENT`
- **`vars`** — defaults like `K2B_HOST` can be overridden: `task deploy:k2b K2B_HOST=192.168.1.201`
- **Serial by default** — build:web runs before build:server (server embeds web/dist)
- **`dev` uses `deps`** — Vite and server start in parallel
