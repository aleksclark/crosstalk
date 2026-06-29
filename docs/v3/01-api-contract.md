# Step 01: API Contract & OpenAPI Generation

## Goal

The server exposes a REST API that automatically generates an OpenAPI 3.1 spec from Go type annotations. This spec is the single source of truth for all client libraries.

## Approach

Use `swaggo/swag` or `danielgtaylor/huma` to generate OpenAPI from Go handler annotations. Huma is preferred — it generates the spec from Go struct types at compile time rather than relying on comment annotations that drift from code.

### Why Huma

- Types ARE the spec — request/response structs define the OpenAPI schema
- Built-in validation from struct tags
- chi-compatible router adapter
- JSON Schema generation for request/response bodies
- No separate spec file to maintain

## Tasks

### 1.1 Project scaffold
- [ ] Create new `server/` module with `go.mod`
- [ ] Add `huma` + `chi` dependencies
- [ ] Set up `cmd/ct-server/main.go` entry point
- [ ] Config loading (reuse pattern from v2's `config.go`)
- [ ] Structured logging with `slog`

### 1.2 Define API types
- [ ] Request/response structs for all endpoints (see Endpoints below)
- [ ] Error response type: `{"error": {"code": string, "message": string}}`
- [ ] Pagination envelope: `{"data": [...], "total": int, "page": int}`

### 1.3 Register endpoints with Huma
- [ ] All endpoints registered with proper HTTP methods, paths, tags
- [ ] Request validation via struct tags
- [ ] Auth metadata on operations (which role can access)

### 1.4 OpenAPI spec output
- [ ] `GET /api/openapi.json` serves the generated spec
- [ ] `GET /api/docs` serves Scalar or Swagger UI for interactive exploration
- [ ] Spec includes auth schemes (JWT Bearer)
- [ ] CI task to export spec to `api/openapi.json` for client generation

## Endpoints

### Auth
```
POST   /api/auth/login          # username + password → JWT
POST   /api/auth/refresh        # refresh token → new JWT
POST   /api/auth/logout         # invalidate refresh token
```

### Sessions (admin + translator read, admin write)
```
GET    /api/sessions            # list all sessions
POST   /api/sessions            # create session
GET    /api/sessions/{id}       # session detail (channels, sources, connections)
PUT    /api/sessions/{id}       # update session metadata
DELETE /api/sessions/{id}       # archive session

GET    /api/sessions/{id}/broadcast-url  # get/regenerate broadcast URL (admin/translator)
POST   /api/sessions/{id}/broadcast-url  # regenerate broadcast URL
```

### Channels (within a session)
```
GET    /api/sessions/{id}/channels          # list channels
POST   /api/sessions/{id}/channels          # add channel
PUT    /api/sessions/{id}/channels/{ch_id}  # update channel config
DELETE /api/sessions/{id}/channels/{ch_id}  # remove channel
```

### Mixing (admin + translator for assigned sessions)
```
GET    /api/sessions/{id}/channels/{ch_id}/mix     # current mix state
PUT    /api/sessions/{id}/channels/{ch_id}/mix     # update mix (mute/level per source)
```

### ABCs (admin only)
```
GET    /api/abcs               # list all ABCs
POST   /api/abcs               # register ABC (name, token)
GET    /api/abcs/{id}          # ABC detail + status
PUT    /api/abcs/{id}          # update ABC config (session assignment, name)
DELETE /api/abcs/{id}          # remove ABC
POST   /api/abcs/{id}/restart  # send restart command via control channel
```

### Translators (admin only for management)
```
GET    /api/translators                    # list translator accounts
POST   /api/translators                    # create translator
PUT    /api/translators/{id}               # update translator
DELETE /api/translators/{id}               # delete translator
PUT    /api/translators/{id}/sessions      # assign sessions to translator
```

### Users (admin only)
```
GET    /api/users              # list admin users
POST   /api/users              # create admin user
DELETE /api/users/{id}         # delete admin user
```

### Public (no auth)
```
GET    /api/sessions/{id}/broadcast   # broadcast info (WebRTC params, listener count)
```

### WebRTC
```
POST   /api/webrtc/token       # authenticated → get short-lived WebRTC signaling token
```

## Acceptance Criteria

- [ ] Server starts, `GET /api/openapi.json` returns valid OpenAPI 3.1 spec
- [ ] Spec includes all endpoints listed above with typed request/response schemas
- [ ] `go test ./...` passes with basic handler registration tests
- [ ] Spec validates clean with `npx @redocly/cli lint api/openapi.json`

## Reusable from v2

- `server/config.go` — config loading pattern (adapt for new fields)
- `server/http/handler.go` — chi router structure (replace with huma adapter)
- Error response pattern from `writeError()` helper
