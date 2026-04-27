# CrossTalk Specification

Realtime audio/video/data bridge using WebRTC. Connects arbitrary WebRTC channels among clients — initially focused on audio streaming for live translation workflows, expandable to video.

---

## Maintaining This Spec

This specification is a living document. As implementation proceeds, the spec must stay in sync with reality.

### Confidence Scores

Every item in the table of contents carries a **confidence score** from 0–10:

| Score | Meaning |
|-------|---------|
| **0** | Initial design — untested idea, no code exists |
| **1–3** | Early implementation — code is being written but hasn't been validated |
| **4–6** | Partially validated — some code works as designed, some areas uncertain |
| **7–8** | Mostly proven — implementation matches spec, edge cases covered by tests |
| **9** | Fully implemented and tested — high confidence this is accurate |
| **10** | Battle-tested — running in production/real usage, confirmed correct |

Scores **increase** when:
- Code is written that embodies the design
- Tests pass that exercise the described behavior
- Real-world usage confirms the design works

Scores **decrease** when:
- Implementation reveals the design doesn't work as described
- A user corrects an inaccuracy
- A section is found to be incomplete or misleading
- An assumption is invalidated during development

When updating a score, also update the prose if the design has diverged from what's written.

### Commit Discipline

**Every change to `spec/` must be in its own commit**, separate from code changes. The commit message must indicate the provenance of the update:

```
spec: <section> — <what changed> (<provenance>)
```

Provenance tags:

| Tag | When to use |
|-----|-------------|
| `initial` | First draft of a section |
| `clarification` | User clarified an ambiguity |
| `correction` | Something was wrong, fixed it |
| `implemented` | Code now exists that matches this design — score increase |
| `tested` | Tests confirm this behavior — score increase |
| `redesign` | Implementation revealed the design doesn't work — score decrease + rewrite |
| `expansion` | New detail added to an existing section |
| `new-section` | Entirely new section added |

Examples:
```
spec: server/sessions — add broadcast routing logic (clarification)
spec: data-model/protobuf — update BindChannel fields to match implementation (correction)
spec: cli-client/pipewire — mark source discovery as validated, score 3→6 (tested)
spec: server/rest-api — redesign token refresh flow, score 4→2 (redesign)
spec: admin-web/session-connect — add keyboard shortcuts section (expansion)
```

### Adding Missing Areas

When you discover something not covered by the spec — a new subsystem, an edge case, a deployment concern — **add it**. Create the file in the appropriate subfolder, add an entry to this index with score `0`, and commit with the `new-section` tag.

Known gaps to fill as implementation starts:
- Error handling strategy and error codes
- Deployment / release process
- Broadcast client implementation details
- Monitoring / observability (metrics, health checks)
- Security considerations (rate limiting, input validation, CORS)

---

## Table of Contents

### [1. Architecture Overview](architecture/overview.md) `confidence: 1`

System topology, component boundaries, and data flow. How the server, CLI clients, admin web UI, and broadcast clients relate to each other and communicate.

| § | Section | Confidence | Notes |
|---|---------|------------|-------|
| 1.1 | [System Diagram](architecture/overview.md#system-diagram) | 1 | |
| 1.2 | [Communication Layers](architecture/overview.md#communication-layers) | 1 | |
| 1.3 | [Protocol Stack](architecture/protocols.md) | 1 | |
| 1.4 | [Project Layout](architecture/project-layout.md) | 0 | No code yet |

### [2. Server](server/overview.md) `confidence: 0`

Go server handling REST API, WebRTC signaling, session orchestration, recording, web hosting, configuration, and log aggregation.

| § | Section | Confidence | Notes |
|---|---------|------------|-------|
| 2.1 | [REST API](server/rest-api.md) | 0 | |
| 2.2 | [WebRTC Signaling](server/webrtc-signaling.md) | 0 | |
| 2.3 | [Session Orchestration](server/sessions.md) | 0 | |
| 2.4 | [Recording](server/recording.md) | 0 | |
| 2.5 | [Persistence](server/persistence.md) | 0 | |
| 2.6 | [Configuration](server/configuration.md) | 0 | JSON + JSON Schema, validate & warn |
| 2.7 | [Logging](server/logging.md) | 0 | Structured JSON; clients stream logs to server |
| 2.8 | [Web Hosting](server/web-hosting.md) | 0 | go:embed prod, Vite reverse proxy dev |

### [3. CLI Client](cli-client/overview.md) `confidence: 0`

Headless Go client for edge devices (primarily KickPi K2B). Connects to server, exposes local audio sources/sinks via PipeWire.

| § | Section | Confidence | Notes |
|---|---------|------------|-------|
| 3.1 | [Authentication & Connection](cli-client/auth-connection.md) | 0 | |
| 3.2 | [PipeWire Integration](cli-client/pipewire.md) | 0 | |
| 3.3 | [Channel Lifecycle](cli-client/channel-lifecycle.md) | 0 | |
| 3.4 | [Hardware: KickPi K2B](cli-client/hardware.md) | 1 | Board exists, scripts written |

### [4. Admin Web UI](admin-web/overview.md) `confidence: 0`

React dashboard for managing the system and connecting to sessions as a browser-based client. Hosted by the server — embedded via `go:embed` in production, proxied to Vite in dev.

| § | Section | Confidence | Notes |
|---|---------|------------|-------|
| 4.1 | [Auth & Dashboard](admin-web/auth-dashboard.md) | 0 | |
| 4.2 | [Management](admin-web/management.md) | 0 | |
| 4.3 | [Session Connect View](admin-web/session-connect.md) | 0 | |
| 4.4 | [Quick-Test Flow](admin-web/quick-test.md) | 0 | |

### [5. Data Model](data-model/overview.md) `confidence: 1`

Core domain objects — channels, sessions, templates, roles — and how they relate.

| § | Section | Confidence | Notes |
|---|---------|------------|-------|
| 5.1 | [Channels](data-model/channels.md) | 1 | |
| 5.2 | [Session Templates](data-model/session-templates.md) | 1 | Role cardinality + broadcast clarified |
| 5.3 | [Sessions](data-model/sessions.md) | 1 | |
| 5.4 | [Protobuf Schema](data-model/protobuf.md) | 0 | Draft messages, will change during impl |

### [6. Dev Environment](dev-environment/overview.md) `confidence: 1`

Docker-based local development with hot reload and hardware-in-the-loop testing.

| § | Section | Confidence | Notes |
|---|---------|------------|-------|
| 6.1 | [Taskfile](dev-environment/taskfile.md) | 0 | go-task: build, dev, test, lint, deploy |
| 6.2 | [Server Container](dev-environment/server-container.md) | 0 | Serves API + proxies web UI |
| 6.3 | [Vite Dev Server](dev-environment/vite-dev.md) | 0 | Runs on host, proxied by server |
| 6.4 | [K2B Deploy Loop](dev-environment/k2b-deploy.md) | 1 | Scripts exist in k2b-board/ |

### [7. Testing](testing/overview.md) `confidence: 1`

Three-tier testing strategy with an emphasis on proving real functionality, not mocking it.

| § | Section | Confidence | Notes |
|---|---------|------------|-------|
| 7.1 | [Unit Tests](testing/unit-tests.md) | 0 | |
| 7.2 | [Integration Tests](testing/integration-tests.md) | 0 | |
| 7.3 | [E2E / Golden Tests](testing/e2e-golden.md) | 0 | |

### [8. Broadcast Listeners](broadcast/overview.md) `confidence: 0`

Public, unauthenticated broadcast listeners — QR code generation, receive-only WebRTC, broadcast tokens, listener count tracking, and the public listener page.

| § | Section | Confidence | Notes |
|---|---------|------------|-------|
| 8.1 | [Overview](broadcast/overview.md) | 0 | Initial design, no code yet |

---

## Tech Stack Summary

| Component | Stack |
|-----------|-------|
| Server | Go (latest, asdf-managed), Pion, SQLite, Goose, testify |
| CLI Client | Go, PipeWire bindings |
| Broadcast Client | Lightweight listen-only client (future), no auth |
| Web UI | Node/TypeScript (latest, asdf + pnpm), React, Vite, shadcn (dark mode) |
| API Contract | OpenAPI (generated from Go types), Protobuf (data channel messages) |
| Signaling | WebSocket (`/ws/signaling` authenticated, `/ws/broadcast` unauthenticated) |
| Client Library | TypeScript, generated from OpenAPI spec |
| Configuration | JSON config files, validated against JSON Schema |
| Logging | Structured JSON to stdout/console + streamed to server |
| Web Hosting | `go:embed` (prod/test), Vite reverse proxy (dev) |
| Dev Environment | Docker, macvlan networking, hot reload via Vite proxy |
| Task Runner | [go-task](https://taskfile.dev/) `Taskfile.yml` — build, dev, test, lint, deploy |
