# Phase 1: Server Foundation

[← Roadmap](index.md)

**Status**: `CURRENT`

The server boots, loads config, connects to SQLite, serves a REST API, and hosts the web UI shell. This phase has no WebRTC — it's pure HTTP + persistence.

## Tasks

### 1.1 Config Loading
- [ ] Implement JSON config loading in `cmd/ct-server/main.go`
- [ ] Parse `--config` flag and `CROSSTALK_CONFIG` env var
- [ ] Validate against `config.schema.json` at startup, log warnings for unknown fields
- [ ] Apply defaults for missing optional fields
- [ ] Configure `slog` JSON logger from `log_level` config

**Test**: `server/cmd/ct-server/config_test.go` — load a valid config, load a config with unknown fields (warns but doesn't fail), load a config with missing required field (exits).

### 1.2 SQLite + Goose Migrations
- [ ] Add `goose` dependency, create `server/sqlite/migrations/` directory
- [ ] Write initial migration: `users`, `api_tokens`, `session_templates`, `sessions`, `session_clients` tables per spec
- [ ] `sqlite.Open()` function: opens DB, enables WAL mode, runs migrations
- [ ] Implement `sqlite.UserService` (all methods on `crosstalk.UserService`)
- [ ] Implement `sqlite.TokenService`
- [ ] Implement `sqlite.SessionTemplateService`
- [ ] Implement `sqlite.SessionService`

**Test**: `server/sqlite/user_test.go` — create user, find by ID, find by username, delete. Uses a real temp SQLite DB. Same pattern for token, template, session tests.

### 1.3 Auth (Token + Password)
- [ ] Token hashing (SHA-256) and verification
- [ ] Password hashing (bcrypt) and verification
- [ ] HTTP middleware: extract `Authorization: Bearer <token>` header, validate against `TokenService`
- [ ] Login endpoint: `POST /api/auth/login` → verify username/password → set session cookie
- [ ] Logout endpoint: `POST /api/auth/logout`
- [ ] WebRTC token generation: `POST /api/webrtc/token` → short-lived token (24h)

**Test**: `server/http/auth_test.go` — using mock services: valid token passes middleware, invalid token returns 401, login with correct password returns cookie, login with wrong password returns 401.

### 1.4 REST API Endpoints
- [ ] Choose HTTP framework (huma, chi, or stdlib) — must support OpenAPI generation from Go types
- [ ] Implement all CRUD endpoints per `spec/server/rest-api.md`:
  - Users: GET list, POST create, PATCH update, DELETE
  - Tokens: GET list, POST create, DELETE revoke
  - Templates: GET list, POST create, GET detail, PUT update, DELETE
  - Sessions: GET list, POST create, GET detail, DELETE end
  - Clients: GET list, GET detail (returns empty for now — no WebRTC yet)
- [ ] Error envelope: consistent JSON error responses with status code + message
- [ ] OpenAPI spec generated from Go types (verify with `GET /api/openapi.json`)

**Test**: `server/http/handler_test.go` — using mock services: POST create template → 201 + valid JSON, GET templates → list, POST create session → 201, GET session → detail with status. Test auth required on all endpoints.

### 1.5 Web UI Hosting
- [ ] Implement `go:embed` for `web/dist/` in `http/` package
- [ ] SPA fallback: non-API, non-WS paths return `index.html`
- [ ] Dev mode: reverse proxy to Vite (`web.dev_proxy_url`), including WebSocket upgrade for HMR
- [ ] Wire into server startup based on `web.dev_mode` config

**Test**: `server/http/webhost_test.go` — in embed mode: GET `/` returns HTML, GET `/nonexistent` returns HTML (SPA fallback), GET `/api/users` does NOT return HTML.

### 1.6 Server Startup + Wiring
- [ ] `cmd/ct-server/main.go` wires all implementations: config → SQLite → services → HTTP handler → listen
- [ ] Graceful shutdown on SIGINT/SIGTERM
- [ ] Seed an initial admin user on first run (or via config)

**Test**: Build `ct-server`, start it with a temp config, hit `GET /api/templates` with a valid token, get 200 + empty list. Kill with SIGINT, confirm clean exit.

## Exit Criteria

All of the above tests pass via `task test:unit:go`. The server binary:
1. Starts with a JSON config file
2. Runs SQLite migrations automatically
3. Serves all REST CRUD endpoints with token/password auth
4. Serves the Vite-built web UI (or proxies to Vite in dev mode)
5. Shuts down cleanly on SIGINT

## Spec Updates

After completing this phase, update `spec/index.md` confidence scores:
- 2.1 REST API → 3
- 2.5 Persistence → 3
- 2.6 Configuration → 3
- 2.8 Web Hosting → 3
