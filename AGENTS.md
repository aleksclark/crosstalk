# AGENTS.md

CrossTalk — realtime audio/video/data bridge using WebRTC.

## Quick Reference

```
task test:unit:go        # Go unit tests (server + cli)
task test:unit:web       # TypeScript unit tests (vitest)
task test:integration    # Go in-process + Playwright browser tests (Docker)
task build:server        # Build server binary (bin/ct-server)
task build:cli           # Build CLI binary (bin/ct-client)
task lint:go             # golangci-lint on server + cli
task lint:web            # ESLint + typecheck on web/
```

## Project Structure

Monorepo with three components + shared protobuf:

```
server/          Go server (REST, WebRTC, SQLite, serves web UI)
cli/             Go CLI client (PipeWire audio, WebRTC)
web/             React + Vite + TypeScript admin UI
proto/           Protobuf definitions (control channel messages)
spec/            Design specification (living document)
```

## Rules

1. **Read `spec/` before implementing** — the spec is the source of truth for design decisions. Check `spec/index.md` for the table of contents and confidence scores.

2. **Follow the standard package layout** — root package = domain types only (no deps). Subpackages grouped by dependency. Never import between subpackages directly.
   → [agent_docs/go-patterns.md](agent_docs/go-patterns.md)

3. **Test everything for real** — mocks only at unit level. Integration and E2E tests use real services. Never claim something works without a test proving it.
   → [agent_docs/testing.md](agent_docs/testing.md)

4. **Spec changes get their own commits** — separate from code. Use the provenance tags. Update confidence scores when implementation validates or invalidates the design.
   → [agent_docs/spec-maintenance.md](agent_docs/spec-maintenance.md)

5. **Use structured JSON logging** — `log/slog` in Go, structured console in TypeScript. Never `fmt.Println` for operational output.

6. **Don't add dependencies without justification** — check what's already in `go.mod` / `package.json`. Prefer stdlib. If you must add a dep, it gets its own subpackage.

7. **Run `task test:integration` after any server/ or web/ change** — the Playwright browser suite exercises every admin SPA view against a real server and database. It must pass before merging. See "When to Run Integration Tests" below.

## When to Run Integration Tests

**Any change to `server/` or `web/` must pass `task test:integration` before merging.** This runs both Go in-process integration tests and the full Playwright browser suite against a real server with real SQLite.

The Playwright suite (`test/playwright/specs/`) exercises every routed view of the admin SPA:

| Spec file | Views covered |
|---|---|
| `login.spec.ts` | `/login` — auth flow, invalid credentials |
| `navigation.spec.ts` | Layout nav bar, logout, auth guard, active link |
| `dashboard.spec.ts` | `/dashboard` — stat cards, recent sessions, quick test |
| `template-editor.spec.ts` | `/templates`, `/templates/:id` — CRUD, roles, mappings, validation |
| `session-list.spec.ts` | `/sessions` — list, create, end, empty state |
| `session-detail.spec.ts` | `/sessions/:id` — status, clients, bindings, assign peers |
| `session-connect.spec.ts` | `/sessions/:id/connect` — WebRTC debug, mic, logs, volume |
| `templates.spec.ts` | `/templates` — create, edit, delete via UI |
| `sessions.spec.ts` | `/sessions` — create from template, connect view |
| `quick-test.spec.ts` | `/dashboard` — quick test button → connect |
| `assign-peer.spec.ts` | API + assign UI on detail/connect pages |

Run the suite locally:
```
task test:integration
```

## Commands

All workflows go through `go-task`. Run `task --list` for the full list.
→ [agent_docs/commands.md](agent_docs/commands.md)

## Go Code

- Two separate Go modules: `server/` and `cli/` (separate `go.mod` files)
- Binaries: `server/cmd/ct-server/` and `cli/cmd/ct-client/`
- Config: JSON files validated against JSON Schema (`config.schema.json`)
- Testify for assertions (`assert`, `require`), hand-written mocks in `mock/`

→ [agent_docs/go-patterns.md](agent_docs/go-patterns.md)

## Web Code

- React + Vite + TypeScript (strict mode) + shadcn dark mode
- Vitest for testing, ESLint for linting
- API client auto-generated from OpenAPI spec
- Hosted by the server (`go:embed` in prod, Vite proxy in dev)

→ [agent_docs/web.md](agent_docs/web.md)

## Roadmap

Implementation is organized into 9 phases, from server foundation to the final golden audio↔audio acceptance test. Check the current phase before starting work.
→ [roadmap/index.md](roadmap/index.md)

## Spec

Design lives in `spec/`, indexed at `spec/index.md`. Each section tracks a confidence score (0–10). Read the relevant spec section before implementing a feature.
→ [agent_docs/spec-maintenance.md](agent_docs/spec-maintenance.md)
