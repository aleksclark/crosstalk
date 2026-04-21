# Session Orchestration

[← Back to Index](../index.md) · [Server Overview](overview.md)

---

## Overview

Sessions are the central runtime concept. A session is a live instance of a [session template](../data-model/session-templates.md) — it defines which roles exist, how channels are mapped, and tracks which clients are connected in which roles.

## Lifecycle

```
Template ──► Create Session ──► Clients Join (per role) ──► Channels Bound ──► Active ──► End
```

### 1. Session Creation

- Admin creates session via REST: `POST /api/sessions { template_id, name }`
- Server instantiates the template's role and channel definitions
- Session enters `waiting` state

### 2. Client Role Assignment

- Client connects via WebRTC, then sends a `JoinSession` message on the control data channel specifying session ID and desired role
- Server validates the role exists and isn't already fully occupied (for single-client roles)
- Server updates session state, notifies other participants

### 3. Channel Binding

Once clients are connected in their roles, the server issues `BindChannel` commands:

```
Template mapping: translator:mic → studio:output
                           ↓
Server → translator client: "send your 'mic' source on track X"
Server → studio client: "play track X on your 'output' sink"
```

The server manages the Pion track forwarding — receiving media from one peer and forwarding to another. The server is a **pure forwarder**: it never decodes, mixes, or processes media content.

### 4. Active Session

- Media flows per the template mappings
- Server records designated streams
- Status changes (client disconnect, new client join) trigger re-evaluation of bindings
- Admin can monitor via REST or live via the session connect view

### 5. Session End

- Admin ends session via REST or by closing the session connect view
- Server sends disconnect commands to all clients
- Recording files are finalized
- Session state is archived in SQLite

## Routing Logic

The server acts as a pure media forwarder:

- For each template mapping `roleA:channel → roleB:channel`, the server:
  1. Receives the media track from roleA's client
  2. Forwards the RTP packets as-is to roleB's client(s)
  3. If roleB is `multi_client`, forwards to every client in that role
- For `→ record` mappings, the server writes the track to disk
- For `→ broadcast` mappings, the server forwards to all connected broadcast clients for this session (unauthenticated, listen-only, connected via `/ws/broadcast`)

### Broadcast Routing

Broadcast is handled differently from role-based routing:
- Broadcast clients are not in any role — they connect with just a session ID
- When a broadcast client connects, the server sends it all tracks that have `broadcast` as a sink in the template
- When a broadcast client disconnects, the server simply stops forwarding
- No control data channel, no status tracking for broadcast clients

## Error Handling

- **Client disconnects mid-session**: Server marks role as disconnected, pauses affected channel bindings, notifies admin. If client reconnects to a single-client role, bindings resume.
- **Codec mismatch**: Server should detect incompatible codecs during binding and report an error rather than silently failing.
- **Partial session**: Sessions can operate with missing roles — only the bindings involving connected roles are active.
- **Role cardinality violation**: If a second client tries to join a `multi_client: false` role that's already occupied, the server rejects with an error message on the control channel.
