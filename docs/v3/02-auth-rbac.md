# Step 02: JWT Auth & Simplified RBAC

## Goal

Replace the v2 opaque-token auth with JWT-based authentication supporting three roles: admin, translator, anonymous. The RBAC model is deliberately simple — no per-resource ACLs, just role-based access.

## Roles

| Role | Can Do |
|------|--------|
| **admin** | Everything — CRUD sessions, manage ABCs, manage translators, mix any channel |
| **translator** | Read all sessions, connect to assigned sessions, mix channels in assigned sessions |
| **anonymous** | Read broadcast info (WebRTC params, listener count) for a specific session via broadcast URL |

## Token Structure

```json
{
  "sub": "user-id",
  "role": "admin|translator",
  "exp": 1234567890,
  "iat": 1234567890
}
```

- Access tokens: short-lived (15 min)
- Refresh tokens: longer-lived (7 days), stored server-side, revocable
- ABC tokens: long-lived API tokens (no expiry, revocable by admin) — these are NOT JWTs, they're opaque bearer tokens like v2

## Tasks

### 2.1 JWT infrastructure
- [ ] `auth/` package: sign, verify, refresh logic
- [ ] HMAC-SHA256 signing with configurable secret
- [ ] Access token + refresh token pair on login
- [ ] Refresh endpoint rotates both tokens
- [ ] Revocation: store refresh token hashes in SQLite, delete on logout

### 2.2 Middleware
- [ ] `RequireAuth` middleware — extracts JWT from `Authorization: Bearer` header, validates, injects claims into context
- [ ] `RequireRole(roles ...string)` — checks role claim against allowed roles
- [ ] `RequireSessionAccess(paramName string)` — for translator endpoints, checks if the user has access to the session ID in the URL

### 2.3 User management
- [ ] `users` table: id, username, password_hash, role, created_at
- [ ] Admin can create/delete users (admins + translators)
- [ ] Password hashing with bcrypt (reuse from v2)
- [ ] `translator_sessions` table: translator_id, session_id (many-to-many)

### 2.4 ABC token auth
- [ ] `abc_tokens` table: id, name, token_hash, abc_id, created_at
- [ ] ABCs authenticate with `Authorization: Bearer <opaque-token>` on WebSocket upgrade
- [ ] Token validated by hash lookup (reuse v2 pattern from `server/http/auth.go`)
- [ ] Middleware detects token type (JWT vs opaque) and routes accordingly

### 2.5 Login flow
- [ ] `POST /api/auth/login` — username + password → {access_token, refresh_token}
- [ ] `POST /api/auth/refresh` — refresh_token → new {access_token, refresh_token}
- [ ] `POST /api/auth/logout` — revokes refresh token

## Acceptance Criteria

- [ ] Admin can log in, get JWT, use it to access all endpoints
- [ ] Translator can log in, but gets 403 on admin-only endpoints
- [ ] Translator can access their assigned sessions but not others
- [ ] Anonymous requests to non-public endpoints get 401
- [ ] Expired JWTs get 401, refresh endpoint issues new pair
- [ ] ABC opaque tokens work alongside JWTs without conflict

## Reusable from v2

- `server/http/auth.go` — bcrypt hashing, token generation, SHA-256 hash storage pattern
- `server/sqlite/user.go` — user CRUD pattern
- `server/sqlite/token.go` — token hash storage pattern
