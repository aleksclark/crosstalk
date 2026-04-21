# REST API

[← Back to Index](../index.md) · [Server Overview](overview.md)

---

## Design Principles

- OpenAPI 3.1 spec **generated from Go types** — never hand-maintained
- TypeScript client library auto-generated from the OpenAPI spec
- JSON request/response bodies throughout
- Standard HTTP status codes and error envelope

## Authentication

Two auth mechanisms, both usable on any endpoint:

| Method | Mechanism | Use Case |
|--------|-----------|----------|
| **Token** | `Authorization: Bearer <token>` | CLI clients, programmatic access |
| **User/Password** | HTTP Basic or login endpoint → session cookie | Admin web UI |

### Token Management

- Tokens created/revoked via REST (admin only)
- Tokens are opaque strings stored hashed in SQLite
- Each token has a name/label for identification
- No expiration on API tokens (explicit revocation only)

### WebRTC Tokens

- Separate from API tokens — short-lived (24h), single-purpose
- Requested via `POST /api/webrtc/token` after authenticating with an API token or session
- Included in the WebRTC signaling offer
- One-time use or time-bounded (24h lifetime)

## Endpoint Groups

### Auth
| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/auth/login` | Username/password login, returns session |
| POST | `/api/auth/logout` | End session |
| POST | `/api/webrtc/token` | Request a 24h WebRTC connection token |

### Users
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/users` | List users |
| POST | `/api/users` | Create user |
| PATCH | `/api/users/:id` | Update user |
| DELETE | `/api/users/:id` | Delete user |

### API Tokens
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/tokens` | List API tokens |
| POST | `/api/tokens` | Create token (returns plaintext once) |
| DELETE | `/api/tokens/:id` | Revoke token |

### Session Templates
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/templates` | List templates |
| POST | `/api/templates` | Create template |
| GET | `/api/templates/:id` | Get template detail |
| PUT | `/api/templates/:id` | Update template |
| DELETE | `/api/templates/:id` | Delete template |

### Sessions
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/sessions` | List sessions (with status) |
| POST | `/api/sessions` | Create session from template |
| GET | `/api/sessions/:id` | Session detail + connected clients |
| DELETE | `/api/sessions/:id` | End session |

### Clients (read-only)
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/clients` | List connected WebRTC clients |
| GET | `/api/clients/:id` | Client detail: sources, sinks, codecs |

### WebRTC Signaling

Signaling uses **WebSocket**, not REST endpoints:

| Protocol | Path | Description |
|----------|------|-------------|
| WS | `/ws/signaling` | Authenticated WebSocket for SDP offer/answer + ICE trickle |
| WS | `/ws/broadcast` | Unauthenticated WebSocket for broadcast listen-only clients |

The `/ws/signaling` endpoint requires a valid auth token (query param or header). The `/ws/broadcast` endpoint requires no authentication — it accepts a `session` query parameter to identify which session's broadcast streams to receive.

> See [WebRTC Signaling](webrtc-signaling.md) for connection lifecycle details.

## OpenAPI Generation

The Go types in the `http/` package are the single source of truth:

```
Go struct definitions + handler signatures
        ↓ (build-time code generation)
server/http/openapi.json
        ↓ (openapi-typescript-codegen or similar)
web/src/lib/api/  (TypeScript client)
```

Candidate libraries for Go → OpenAPI generation: [huma](https://huma.rocks/), [ogen](https://ogen.dev/), or [swaggo](https://github.com/swaggo/swag). Final choice TBD during implementation.
