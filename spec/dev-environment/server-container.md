# Server Container

[← Back to Index](../index.md) · [Dev Environment Overview](overview.md)

---

## Purpose

Runs the Go server in a Docker container. The server handles everything: REST API, WebSocket signaling, WebRTC, and reverse-proxying the web UI to the Vite dev server on the host.

## Dockerfile

```dockerfile
# dev/Dockerfile.server
FROM golang:1.24-bookworm

# Runtime dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    sqlite3 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Binary and data are volume-mounted
CMD ["/app/bin/ct-server"]
```

## Volume Mounts

```yaml
volumes:
  - ../server:/app/server        # Source code (for build)
  - server-data:/data            # SQLite DB + recordings
```

## Hot Reload Strategy

The server is a compiled Go binary — true hot reload isn't practical. Instead:

1. **Host-side file watcher** (e.g., `air`, `entr`, or a simple script) detects `.go` file changes
2. Builds the binary: `task build:server`
3. Restarts the container: `docker compose -f dev/docker-compose.yml restart server`

Alternatively, build inside the container:
1. Mount source code into container
2. Use [air](https://github.com/air-verse/air) inside the container for automatic rebuild + restart
3. This avoids cross-compilation issues

## Network

- macvlan with dedicated IP (e.g., `192.168.1.102`)
- Server listens on `0.0.0.0:8080`
- Accessible from host browser and K2B device
- In dev mode, proxies web UI requests to Vite on the host (`192.168.1.100:5173`)

## Configuration

Uses `server/dev.json` (mounted from host):

```json
{
  "$schema": "./config.schema.json",
  "listen": ":8080",
  "db_path": "/data/crosstalk.db",
  "recording_path": "/data/recordings",
  "log_level": "debug",
  "web": {
    "dev_mode": true,
    "dev_proxy_url": "http://192.168.1.100:5173"
  }
}
```

> See [Server > Configuration](../server/configuration.md) for all config options.

## Data Persistence

- SQLite file persists in a named Docker volume (`server-data`)
- Survives container restarts and rebuilds
- `task dev:reset` to wipe everything (DB + recordings)
- Goose migrations run on server startup, so schema is always current
