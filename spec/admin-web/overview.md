# Admin Web UI

[← Back to Index](../index.md)

React-based dashboard for managing CrossTalk — server status, user/token management, session control, and browser-based session participation.

**Hosted by the server** — in production the UI is embedded in the `ct-server` binary via `go:embed`. In development the server reverse-proxies to Vite for hot reload. There is no separate web server.

---

## Tech Stack

- React + Vite + TypeScript (strict mode)
- shadcn/ui components, dark mode
- Node/TypeScript managed with asdf + pnpm
- API client auto-generated from OpenAPI spec

## Deployment

| Mode | How It Works |
|------|-------------|
| **Production** | `pnpm build` → `web/dist/` → embedded in `ct-server` via `go:embed` |
| **Integration tests** | Same as production — single binary serves everything |
| **Development** | Vite dev server runs locally, Go server proxies non-API requests to it |

No CORS configuration needed in any mode — the API and UI always share the same origin.

> See [Server > Web Hosting](../server/web-hosting.md) for implementation details.

## Logging

Structured JSON to browser console (always) + Protobuf `LogEntry` to the server via the control data channel (when connected to a session). The session connect view also displays aggregated logs from all session participants.

> See [Server > Logging](../server/logging.md) for format details.

## Sections

| Section | Description |
|---------|-------------|
| [Auth & Dashboard](auth-dashboard.md) | Login, server status, connected clients |
| [Management](management.md) | Tokens, users, templates, sessions |
| [Session Connect](session-connect.md) | Browser-based session participation with audio, debug tools |
| [Quick-Test Flow](quick-test.md) | One-click session creation and connection for rapid iteration |

## Page Structure

```
/login                    — Username/password login
/dashboard                — Server status, connected clients overview
/tokens                   — API token management
/users                    — User management
/templates                — Session template CRUD
/templates/:id            — Template editor
/sessions                 — Session list
/sessions/:id             — Session detail + monitoring
/sessions/:id/connect     — Session connect view (browser as client)
```
