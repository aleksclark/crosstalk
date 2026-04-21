# Web Hosting

[← Back to Index](../index.md) · [Server Overview](overview.md)

---

## Overview

The server hosts the admin web UI directly — there is no separate web server or container for the frontend. The `ct-server` binary serves both the REST API and the web UI from the same HTTP listener.

## Modes

| Mode | Source | Use Case |
|------|--------|----------|
| **Production** | `go:embed` — web assets compiled into the binary | Deploys, integration tests |
| **Development** | Reverse proxy to Vite dev server | Local dev with hot reload |

### Production Mode (`web.dev_mode: false`, default)

The built web assets (`web/dist/`) are embedded into the server binary at compile time using `go:embed`:

```go
//go:embed all:web/dist
var webAssets embed.FS
```

The server serves these at the root path (`/`), with the API under `/api/` and WebSocket endpoints at `/ws/`. This means:

- Single binary deployment — no separate web server needed
- No CORS issues — API and UI share the same origin
- Integration tests use the same binary, no separate web container

### Development Mode (`web.dev_mode: true`)

In dev mode, the server **reverse-proxies** non-API requests to the Vite dev server:

```
Browser request
    │
    ├── /api/*          → handled by Go server directly
    ├── /ws/*           → handled by Go server directly (WebSocket)
    └── /* (everything else) → proxied to Vite dev server
```

The Vite dev server URL is configured via `web.dev_proxy_url` (default: `http://localhost:5173`).

This gives you:
- **Vite HMR** — hot module replacement works because the browser talks to the Go server, which proxies to Vite, and Vite's WebSocket for HMR is forwarded too
- **Same-origin API** — no CORS configuration needed in dev either
- **Single entry point** — developers open one URL (the server) for both API and UI

### Vite Configuration

Vite must be configured to work behind the server's reverse proxy:

```typescript
// web/vite.config.ts
export default defineConfig({
  server: {
    // Vite listens on 5173, but browser accesses via the Go server
    origin: 'http://localhost:5173',
    hmr: {
      // HMR WebSocket goes through the Go server's proxy
      port: 5173,
    },
  },
})
```

The Go server must proxy WebSocket upgrade requests for Vite's HMR path (typically `/__vite_hmr` or similar) in addition to regular HTTP requests.

## URL Routing

All modes use the same URL structure:

| Path | Handler |
|------|---------|
| `/api/*` | REST API (Go handlers) |
| `/ws/signaling` | WebSocket for WebRTC signaling |
| `/ws/broadcast` | WebSocket for broadcast clients |
| `/*` | Web UI (embedded assets or Vite proxy) |

SPA fallback: any path that doesn't match `/api/*` or `/ws/*` and doesn't match a static file returns `index.html` (for client-side routing).

## Build Pipeline

```
1. task build:web    → pnpm build → web/dist/  (Vite production build)
2. task build:server → go build   → embeds web/dist/ via go:embed
3. Single binary     → ct-server serves everything
```

Or simply `task build` which runs both steps in sequence.

> See [Dev Environment > Taskfile](../dev-environment/taskfile.md) for all available tasks.

## Impact on Dev Environment

With web hosting built into the server, the dev Docker setup simplifies:

- **No separate web UI container** — the server container runs the Go server (which proxies to Vite)
- **Vite runs on the host** (or inside the server container) for dev mode
- Only one container needed for the server + web UI in dev

> See [Dev Environment](../dev-environment/overview.md) for updated container setup.
