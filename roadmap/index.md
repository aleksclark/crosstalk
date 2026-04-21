# Roadmap

Implementation roadmap from skeleton to the final golden audio↔audio acceptance test.

## How to Use This Roadmap

### For Agents

1. Check the **current phase** below — it's marked with `⬅ CURRENT`
2. Read the phase detail file for your tasks and acceptance criteria
3. Work through tasks in order — each has a specific test to prove it's done
4. When a phase's exit criteria are met, update this file: move `⬅ CURRENT` to the next phase

### Updating Progress

When completing a task, update its status in the phase file:
- `[ ]` → `[x]` when the acceptance test passes
- Add the commit hash or PR that completed it
- If a task turns out to be wrong or needs redesign, note it and update the spec

When all tasks in a phase are `[x]` and the exit criteria pass:
1. Move `⬅ CURRENT` to the next phase in this file
2. Update relevant `spec/index.md` confidence scores
3. Commit: `roadmap: complete phase N, begin phase N+1`

### Adding Tasks

If you discover work not covered:
- Add it to the appropriate phase (or create a new sub-phase)
- If it blocks other work, note the dependency
- If it's a spec gap, also add a `spec/` entry

---

## Dependency Graph

```
Phase 1: Server Foundation
    │
    ├──► Phase 2: WebRTC Signaling
    │        │
    │        ├──► Phase 3: Control Channel + Protobuf
    │        │        │
    │        │        └──► Phase 5: Session Orchestration
    │        │                 │
    │        │                 ├──► Phase 6: Server-Side Recording
    │        │                 │
    │        │                 └──► Phase 8: Integration Tests
    │        │                          │
    │        └──► Phase 4: CLI Client        └──► Phase 9: E2E Golden Tests
    │                 │
    │                 └──► Phase 5 (needs CLI to join sessions)
    │
    └──► Phase 7: Admin Web UI (can start after Phase 1, parallel with 2-6)
              │
              └──► Phase 8 + 9 (needs UI for Playwright tests)
```

## Phases

### [Phase 1: Server Foundation](phase-01-server-foundation.md) `⬅ CURRENT`

Config loading, SQLite persistence, REST API, web hosting shell. The server boots, serves an API, and persists data.

**Exit criteria**: `ct-server` starts, serves REST endpoints for users/tokens/templates/sessions, stores data in SQLite, serves the web UI shell. All tested with `go test`.

---

### [Phase 2: WebRTC Signaling](phase-02-webrtc-signaling.md)

WebSocket signaling endpoint, Pion peer connection lifecycle, ICE handling.

**Exit criteria**: A Go test client can connect via WebSocket, complete SDP offer/answer, establish a Pion peer connection, and exchange a data channel message. Tested with `go test` using Pion's in-process API.

---

### [Phase 3: Control Channel + Protobuf](phase-03-control-channel.md)

Protobuf code generation, control data channel message handling (Hello/Welcome, ClientStatus, ChannelStatus).

**Exit criteria**: After WebRTC connection, client sends `Hello` with capabilities, server responds with `Welcome`. Protobuf round-trip test passes. `LogEntry` messages flow bidirectionally.

---

### [Phase 4: CLI Client Core](phase-04-cli-client.md)

CLI client authenticates, connects via WebSocket, establishes WebRTC, sends Hello, discovers PipeWire sources/sinks.

**Exit criteria**: `ct-client` connects to a running `ct-server`, appears in `GET /api/clients` with reported sources/sinks. Tested with a real server process and PipeWire loopback.

---

### [Phase 5: Session Orchestration + Audio Forwarding](phase-05-sessions.md)

Create sessions from templates, join clients to roles, resolve channel bindings, forward audio tracks between peer connections.

**Exit criteria**: Two Pion test clients join a session in different roles. Audio sent by client A arrives at client B per the template mapping. Verified by comparing sent/received RTP payloads in a Go integration test.

---

### [Phase 6: Server-Side Recording](phase-06-recording.md)

Capture audio from `→ record` mappings to OGG/Opus files on disk.

**Exit criteria**: After a session with `→ record` mappings, an OGG file exists on disk. `ffprobe` confirms it's valid Opus audio with the expected duration (±1s). Tested with a Go integration test.

---

### [Phase 7: Admin Web UI](phase-07-admin-web.md)

Login, dashboard, template/session management, session connect view with audio, VU meters, WebRTC debug panel, quick-test button.

**Exit criteria**: Playwright tests cover: login flow, template CRUD, session creation, session connect view renders with mic selector and VU meter. Quick-test button creates a session and connects.

---

### [Phase 8: Integration Tests](phase-08-integration-tests.md)

Docker Compose test environment, full-stack tests with real SQLite/Pion/HTTP, Playwright for web UI.

**Exit criteria**: `task test:integration` passes — server + test runner in Docker, Playwright exercises login → create template → create session → connect → verify audio controls render.

---

### [Phase 9: E2E Golden Audio Tests](phase-09-e2e-golden.md)

Real audio through real hardware. Admin web → K2B device and K2B device → admin web, with waveform comparison.

**Exit criteria**: `task test:e2e` passes both golden tests:
1. Browser sends 1kHz tone via session → K2B captures it → cross-correlation > 0.9
2. K2B plays 1kHz tone → browser captures it → cross-correlation > 0.9
