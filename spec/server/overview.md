# Server

[← Back to Index](../index.md)

The server is the central orchestrator — it manages auth, session lifecycle, WebRTC signaling, media routing, recording, and hosts the admin web UI. Written in Go using Pion for WebRTC. Single binary deployment via `go:embed`.

---

## Responsibilities

1. **REST API** — management plane for auth, CRUD
2. **WebSocket signaling** — WebRTC offer/answer/ICE exchange
3. **WebRTC hub** — Pion-based peer connection management, media track forwarding
4. **Session orchestration** — instantiate templates, assign roles, command channel bindings
5. **Recording** — capture designated streams to disk
6. **Persistence** — SQLite for all durable state
7. **Web hosting** — serve admin web UI (embedded in prod, Vite proxy in dev)
8. **Log aggregation** — receive structured logs from clients, forward to admin UI

## Sections

| Section | Description |
|---------|-------------|
| [REST API](rest-api.md) | Endpoints, auth, OpenAPI generation |
| [WebRTC Signaling](webrtc-signaling.md) | Connection lifecycle, ICE/STUN, token auth |
| [Sessions](sessions.md) | Template instantiation, role management, channel routing |
| [Recording](recording.md) | Stream capture, file format, storage |
| [Persistence](persistence.md) | SQLite schema, Goose migrations |
| [Configuration](configuration.md) | JSON config files, JSON Schema validation |
| [Logging](logging.md) | Structured JSON logging, client log aggregation |
| [Web Hosting](web-hosting.md) | go:embed for prod, Vite reverse proxy for dev |

## Key Design Decisions

- **Standard package layout** — root package has domain types + interfaces only (no deps). Subpackages grouped by dependency: `sqlite/`, `http/`, `ws/`, `pion/`. `mock/` for function-injection test mocks. `cmd/ct-server/` wires everything together.
- **Server forwards all media** — no direct client-to-client connections. This simplifies NAT traversal, enables server-side recording, and gives the server full visibility into session state.
- **OpenAPI generated from Go types** — struct tags and code generation produce the spec; no hand-written YAML.
- **Single binary** — `ct-server` embeds everything: HTTP server, Pion, SQLite, and the admin web UI assets.
- **Server hosts the web UI** — no separate web server. In production the UI is embedded via `go:embed`; in dev the server reverse-proxies to Vite for HMR.
- **JSON config + JSON Schema** — validated on startup, warns on mismatch, editors get autocomplete.
- **Structured JSON logs** — all components log JSON to stdout; clients also stream logs to the server.
