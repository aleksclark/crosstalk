# Sessions

[← Back to Index](../index.md) · [Data Model Overview](overview.md)

---

## Overview

A session is a runtime instance of a [session template](session-templates.md). It tracks which clients are connected, what roles they fill, and the status of each channel binding.

## States

```
waiting ──► active ──► ended
              │
              ├──► (client disconnects) ──► still active (partial)
              └──► (admin ends) ──► ended
```

| State | Meaning |
|-------|---------|
| `waiting` | Session created, no clients connected yet |
| `active` | At least one client connected and channels bound |
| `ended` | Session terminated, all connections closed, recordings finalized |

A session becomes `active` when the first channel binding succeeds. It remains `active` even if some clients disconnect (partial operation is expected).

## Client Role Assignment

When a client joins a session:

1. Client sends `JoinSession { session_id, role }` on the control data channel
2. Server validates:
   - Session exists and is not `ended`
   - Role exists in the session's template
   - If role has `multi_client: false`, no other client currently holds that role
3. Server registers the client in the session with the specified role
4. Server evaluates which template mappings can now be activated
5. Server sends `BindChannel` commands to affected clients

## Multi-Client Roles

Roles with `multi_client: true` can be filled by multiple clients simultaneously:

- Each client in a multi-client role gets the same sink bindings (e.g., all receive `translator:mic` audio)
- **Multi-client roles cannot be mapping sources** — only sinks. This is enforced at template validation time.
- Server creates separate WebRTC track forwarding for each client in the role
- Clients joining a multi-client role never displace existing clients

## Session Monitoring

The REST API exposes session state:

```json
{
  "id": "01JSGABC...",
  "name": "Translation #5",
  "template": { "id": "...", "name": "Translation" },
  "status": "active",
  "created_at": "2026-04-21T10:30:00Z",
  "clients": [
    {
      "role": "translator",
      "client_id": "01JSG...",
      "status": "connected",
      "channels": [
        { "name": "mic", "direction": "source", "state": "active" }
      ]
    },
    {
      "role": "studio",
      "client_id": "01JSG...",
      "status": "connected",
      "channels": [
        { "name": "input", "direction": "source", "state": "active" },
        { "name": "output", "direction": "sink", "state": "active" }
      ]
    }
  ],
  "recordings": [
    { "source": "translator:mic", "status": "recording", "bytes": 1048576 }
  ]
}
```

## Session End

When a session is ended (via REST or admin UI):
1. Server sends `UnbindChannel` to all connected clients
2. Server sends `SessionEnded` event on control data channel
3. All recordings are finalized (file closed, metadata written)
4. Session status set to `ended` with `ended_at` timestamp
5. Client WebRTC connections for this session are closed
