# Session Templates

[← Back to Index](../index.md) · [Data Model Overview](overview.md)

---

## Overview

A session template is a reusable blueprint that defines:
1. What **roles** exist in a session and their cardinality
2. How **channels are mapped** between roles

Templates are purely declarative — they don't reference specific clients or devices.

## Structure

```json
{
  "id": "01JSGXYZ...",
  "name": "Translation",
  "is_default": true,
  "roles": [
    { "name": "translator", "multi_client": false },
    { "name": "studio",     "multi_client": false }
  ],
  "mappings": [
    { "source": "translator:mic",   "sink": "studio:output" },
    { "source": "studio:input",     "sink": "translator:speakers" },
    { "source": "translator:mic",   "sink": "broadcast" },
    { "source": "translator:mic",   "sink": "record" },
    { "source": "studio:input",     "sink": "record" }
  ]
}
```

## Roles

Each role has:

| Property | Type | Description |
|----------|------|-------------|
| `name` | string | Arbitrary identifier (e.g., `translator`, `studio`) |
| `multi_client` | boolean | Whether multiple clients can fill this role simultaneously |

### Role Cardinality

- **`multi_client: false`** (default) — exactly zero or one client can fill this role at a time. If a second client tries to join the same role, the server rejects it.
- **`multi_client: true`** — any number of clients can fill this role. Each receives the same sink bindings.

### Multi-Client Send Restriction

**A multi-client role must not be mapped as a source in any mapping.** Only single-client roles can send. This prevents ambiguity about which client's audio to forward when multiple clients fill the same role.

Validation rejects templates where a `multi_client: true` role appears on the left side of any mapping source (e.g., `audience:mic → ...` is invalid if `audience` is multi-client).

## Mapping Syntax

Each mapping has a **source** and a **sink**:

### Source

Always `role:channel_name` — a named channel on a specific role's client. The role must have `multi_client: false`.

### Sink

One of three forms:

| Form | Example | Meaning |
|------|---------|---------|
| `role:channel_name` | `studio:output` | Route to a specific role's named sink |
| `record` | `record` | Server records this stream to disk |
| `broadcast` | `broadcast` | Stream to all connected broadcast clients (unauthenticated, listen-only) |

### Broadcast

`broadcast` is a first-class sink type. When a mapping targets `broadcast`:
- The server forwards that audio track to **all connected broadcast clients**
- Broadcast clients connect via `/ws/broadcast` with **no authentication**
- Broadcast clients are listen-only — they cannot send any audio
- A future broadcast client component will be built to consume these streams
- Multiple mappings can target `broadcast` — broadcast clients receive all of them as separate tracks

> See [Architecture > Protocols](../architecture/protocols.md) for broadcast client connection details.

## Default Template

- At most one template can be flagged `is_default`
- Used by the quick-test flow in the admin web UI
- Setting a new default clears the flag on the previous default

## Validation Rules

- All roles referenced in mappings must be listed in `roles`
- **Multi-client roles cannot appear as mapping sources** (only as sinks or not at all)
- Source channel names should match what clients are expected to report
- `record` and `broadcast` are valid sinks but not valid sources
- Duplicate mappings (same source → same sink) are rejected
