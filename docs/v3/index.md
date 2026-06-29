# CrossTalk v3 — Implementation Plan

A clean-slate rebuild of CrossTalk focused on: REST API with OpenAPI code generation, Audio Booth Connectors (ABCs), multi-session translator workflow, channel-based audio mixing, and aggressive WebRTC debug observability.

## Architecture Summary

```
┌─────────────────────────────────────────────────────────────────┐
│                      ct-server (Go)                              │
│  ┌──────────┐  ┌──────────┐  ┌───────────┐  ┌───────────────┐  │
│  │ REST API │  │  WebRTC  │  │  Session  │  │   Recording   │  │
│  │ (chi +   │  │  (Pion)  │  │ Orchestr. │  │   (OGG/Opus)  │  │
│  │ OpenAPI) │  │          │  │           │  │               │  │
│  └──────────┘  └──────────┘  └───────────┘  └───────────────┘  │
│  JWT Auth (admin/translator/anon) │ SQLite                      │
└───────────────────────────────────┼─────────────────────────────┘
                                    │
        ┌───────────────────────────┼───────────────────────┐
        │                           │                       │
   ┌────▼────┐              ┌───────▼──────┐        ┌──────▼──────┐
   │Admin SPA│              │Translator SPA│        │Broadcast SPA│
   │ (React) │              │   (React)    │        │  (React)    │
   └─────────┘              └──────────────┘        └─────────────┘
        │                           │
        │    ┌──────────────────────┤
        │    │                      │
   ┌────▼────▼──┐           ┌──────▼──────┐
   │ TS Client  │           │  Go Client  │
   │ (generated)│           │ (generated) │
   └────────────┘           └─────────────┘
                                    │
                             ┌──────▼──────┐
                             │     ABC     │
                             │  (KickPi)   │
                             └─────────────┘
```

## Reusable Code from v2

These components are solid and can be carried forward with minimal changes:

| Component | Location | What to Keep | What to Change |
|-----------|----------|--------------|----------------|
| Pion WebRTC core | `server/pion/peer.go` | PeerManager, ICE config, UDP mux | Add verbose debug logging |
| RTP forwarding | `server/pion/forward.go` | ForwardTrack pattern | Extend for mixing (multi-source) |
| Recording | `server/pion/recording.go` | OGG/Opus recorder | Per-source recording + timestamps |
| SQLite patterns | `server/sqlite/` | Client, migrations, time helpers | New schema (sessions, channels, users) |
| Protobuf control channel | `proto/` + `server/pion/control.go` | Message framing pattern | New message set for v3 |
| WebSocket signaling | `server/ws/signaling.go` | SDP/ICE exchange loop | Extract into reusable handler |
| CLI PipeWire integration | `cli/pipewire/` | Source/sink discovery | Keep as-is |
| CLI reconnection logic | `cli/pion/client.go` | Exp backoff, factory pattern | Adapt for new control protocol |
| K2B board support | `k2b-board/` | All of it | Just point at new binary |
| Broadcast token store | `server/broadcast/` | In-memory token + validation | Move to per-session URLs |
| Config loading | `server/config.go` | JSON + validation pattern | New schema for v3 |

## Steps

| # | File | Title | Status |
|---|------|-------|--------|
| 01 | [01-api-contract.md](01-api-contract.md) | API Contract & OpenAPI Generation | ⬜ |
| 02 | [02-auth-rbac.md](02-auth-rbac.md) | JWT Auth & Simplified RBAC | ⬜ |
| 03 | [03-data-model.md](03-data-model.md) | Data Model (Sessions, Channels, Sources) | ⬜ |
| 04 | [04-client-codegen.md](04-client-codegen.md) | TypeScript + Go Client Generation | ⬜ |
| 05 | [05-webrtc-foundation.md](05-webrtc-foundation.md) | WebRTC Foundation & Debug Logging | ⬜ |
| 06 | [06-session-orchestration.md](06-session-orchestration.md) | Session Orchestration & Channel Mixing | ⬜ |
| 07 | [07-abc-management.md](07-abc-management.md) | Audio Booth Connector (ABC) Management | ⬜ |
| 08 | [08-recording.md](08-recording.md) | Per-Source + Per-Channel Recording | ⬜ |
| 09 | [09-translator-app.md](09-translator-app.md) | Translator Frontend App | ⬜ |
| 10 | [10-admin-app.md](10-admin-app.md) | Admin Frontend App | ⬜ |
| 11 | [11-broadcast-app.md](11-broadcast-app.md) | Broadcast Listener App | ⬜ |
| 12 | [12-integration-testing.md](12-integration-testing.md) | Integration & E2E Testing | ⬜ |

Status legend: ⬜ Not started | 🔨 In progress | ✅ Complete
