# Dev Environment

[← Back to Index](../index.md)

Docker-based local development setup with hot reload for all components and hardware-in-the-loop testing via the KickPi K2B board.

---

## Sections

| Section | Description |
|---------|-------------|
| [Taskfile](taskfile.md) | All build/dev/test/lint/deploy commands via go-task |
| [Server Container](server-container.md) | Go server with web UI proxy, hot restart |
| [Vite Dev Server](vite-dev.md) | Hot reload for the web UI, proxied through the server |
| [K2B Deploy Loop](k2b-deploy.md) | Watcher script, auto-deploy, test harness |

## Architecture

The server hosts the web UI in all environments (see [Server > Web Hosting](../server/web-hosting.md)). In dev mode, the server reverse-proxies non-API requests to the Vite dev server running on the host:

```
Browser → Server container (:8080)
              ├── /api/*     → Go handlers
              ├── /ws/*      → WebSocket handlers
              └── /*         → proxy to Vite on host (:5173)
                                  └── HMR WebSocket forwarded
```

## Network Architecture

All containers use **macvlan networking** — each container gets its own IP on the local network, making it routable from other devices (including the K2B board) without port mapping.

```
Local Network (e.g., 192.168.1.0/24)
  ├── 192.168.1.100  Host machine (Vite dev server on :5173)
  ├── 192.168.1.102  Server container (Go + SQLite + web proxy)
  ├── 192.168.1.103  Integration test environment
  └── 192.168.1.200  KickPi K2B board
```

## Docker Compose

```yaml
# dev/docker-compose.yml
version: "3.8"

networks:
  crosstalk-dev:
    driver: macvlan
    driver_opts:
      parent: ${MACVLAN_PARENT:?Set MACVLAN_PARENT in .env to your host interface}
    ipam:
      config:
        - subnet: 192.168.1.0/24

services:
  server:
    build:
      context: ..
      dockerfile: dev/Dockerfile.server
    volumes:
      - ../server:/app/server
      - server-data:/data
    networks:
      crosstalk-dev:
        ipv4_address: 192.168.1.102
    environment:
      - CROSSTALK_CONFIG=/app/server/dev.json

volumes:
  server-data:
```

Server dev config (`server/dev.json`):

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

Requires a `dev/.env` file:

```bash
# dev/.env
MACVLAN_PARENT=eth0   # Set to your host's physical interface (e.g., enp3s0)
```

## Workflow

```
Terminal 1: task dev             (starts Vite + server container in parallel)
Terminal 2: task deploy:k2b:watch (watches CLI binary, deploys to K2B)
Terminal 3: task deploy:k2b:test  (plays/records audio for testing)
```

Or start components individually:

```
task dev:vite       — Vite dev server on host
task dev:server     — Server container via Docker Compose
task deploy:k2b     — One-shot build + deploy to K2B
```

> See [Taskfile](taskfile.md) for the full task reference.

Open `http://192.168.1.102:8080` in the browser — the server proxies UI requests to Vite, so HMR works. API calls go directly to the Go server.
