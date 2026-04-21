# Vite Dev Server

[← Back to Index](../index.md) · [Dev Environment Overview](overview.md)

---

## Purpose

Runs the Vite dev server **on the host machine** (not in a container) for hot module replacement. The Go server container reverse-proxies non-API requests to Vite.

This replaces the previous standalone web UI container. Since the server hosts the web UI in all environments, the dev setup mirrors production: one entry point, API and UI on the same origin.

## Setup

```bash
task dev:vite
```

This runs `pnpm install` and `pnpm dev --host 0.0.0.0` in the `web/` directory.

Vite listens on `0.0.0.0:5173` so the server container (on the macvlan network) can reach it.

## Vite Configuration

```typescript
// web/vite.config.ts
export default defineConfig({
  server: {
    host: '0.0.0.0',
    port: 5173,
    // HMR must work through the Go server's reverse proxy
    hmr: {
      port: 5173,
    },
  },
})
```

## How It Connects

```
Browser → http://192.168.1.102:8080  (Go server container)
              │
              ├── /api/*, /ws/*  → Go handlers
              └── /*             → proxy to http://192.168.1.100:5173 (Vite on host)
                                      ├── JS/CSS/HTML assets
                                      └── /__vite_hmr (WebSocket for HMR)
```

The Go server proxies:
- Regular HTTP requests (HTML, JS, CSS)
- WebSocket upgrade requests for Vite's HMR channel (`/__vite_hmr`)

## Notes

- Runs on the host, not in Docker — avoids node_modules platform mismatches and keeps HMR fast
- `node_modules/` is local to the host machine
- Generated API client (`web/src/lib/api/`) must be regenerated when the OpenAPI spec changes: `task generate:api-client`
- No CORS needed — browser sees a single origin (the Go server)
