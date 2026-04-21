# Architecture Overview

[← Back to Index](../index.md)

CrossTalk is a four-component system: a central **server**, one or more **CLI clients** on edge devices, an **admin web UI** for management and browser-based participation, and a **broadcast client** for unauthenticated listen-only access.

---

## System Diagram

```
┌─────────────────────────────────────────────────────────┐
│                      Network                            │
│                                                         │
│  ┌──────────────┐    REST + WebRTC    ┌──────────────┐  │
│  │  CLI Client   │◄──────────────────►│    Server     │  │
│  │  (K2B board)  │                    │   (Go/Pion)   │  │
│  │              │    WebRTC data ch.  │              │  │
│  │  PipeWire    │◄──────────────────►│  SQLite       │  │
│  │  sources/    │                    │  Recording    │  │
│  │  sinks       │                    │              │  │
│  └──────────────┘                    └──────┬───────┘  │
│                                             │          │
│  ┌──────────────┐    REST + WebRTC          │          │
│  │  CLI Client   │◄────────────────────────┘│          │
│  │  (K2B #2)    │                           │          │
│  └──────────────┘                           │          │
│                                             │          │
│  ┌──────────────┐    REST + WebSocket/WR    │          │
│  │  Admin Web    │◄─────────────────────────┘          │
│  │  (Browser)   │                           │          │
│  └──────────────┘                           │          │
│                                             │          │
│  ┌──────────────┐    WebRTC only (no auth)  │          │
│  │  Broadcast    │◄─────────────────────────┘          │
│  │  Client(s)   │  listen-only                         │
│  └──────────────┘                                      │
└─────────────────────────────────────────────────────────┘
```

## Communication Layers

The system uses two distinct communication layers:

| Layer | Transport | Purpose |
|-------|-----------|---------|
| **Management** | REST over HTTPS | Auth, CRUD for users/tokens/templates/sessions, client status queries |
| **Signaling** | WebSocket | WebRTC offer/answer/ICE exchange |
| **Realtime** | WebRTC (Pion) | Audio/video media streams, data channel for control messages + log streaming |

Key principle: REST handles management CRUD. WebSocket handles WebRTC signaling (offer/answer/ICE). WebRTC handles everything that needs low-latency, peer-style connectivity — including the `control` data channel for live communication between server and clients.

## Data Flow: Translation Session Example

```
1. Admin creates session from "Translation" template via REST
2. CLI clients (translator device, studio device) connect via REST, get WebRTC tokens
3. Clients establish WebRTC connections to server via Pion
4. Server instructs clients to bind channels per template mappings:
     translator:mic  → studio:output
     studio:input    → translator:speakers
     translator:mic  → broadcast (unauthenticated listeners)
     translator:mic  → server:record
     studio:input    → server:record
5. Audio flows over WebRTC media tracks — server purely forwards, no mixing
6. Control/status messages flow over WebRTC data channel
7. Admin web UI can join as "translator" role using browser media
8. Broadcast clients connect without auth, receive listen-only streams
```

## Design Principles

- **Server is a pure forwarder** — clients don't connect to each other directly; the server forwards media tracks without mixing or processing
- **Channels are the primitive** — everything is modeled as named, typed, directional channels
- **Templates define topology** — session templates declaratively map roles to channel connections, with per-role cardinality settings
- **Generated contracts** — OpenAPI and Protobuf schemas are derived from code, never hand-written
- **Broadcast is a first-class concept** — unauthenticated listen-only clients can receive designated streams
