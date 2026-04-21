# Data Model

[← Back to Index](../index.md)

Core domain objects — channels, sessions, templates, roles — and how they compose to define the system's runtime behavior.

---

## Sections

| Section | Description |
|---------|-------------|
| [Channels](channels.md) | Types, directions, the control channel |
| [Session Templates](session-templates.md) | Roles, channel mapping DSL |
| [Sessions](sessions.md) | Runtime instances, role assignment, status |
| [Protobuf Schema](protobuf.md) | Wire format for data channel messages |

## Entity Relationships

```
SessionTemplate
  ├── roles: [Role]
  │     ├── name: string
  │     └── multi_client: boolean
  └── mappings: [Mapping]
        ├── source: {role, channel_name}   (role must be single-client)
        └── sink: {role, channel_name} | "record" | "broadcast"

Session
  ├── template: SessionTemplate
  ├── status: waiting | active | ended
  └── clients: [SessionClient]
        ├── role: string
        ├── status: connected | disconnected
        └── channels: [ChannelBinding]
              ├── channel_id: string
              ├── local_name: string (PipeWire node)
              ├── direction: source | sink
              └── state: idle | binding | active | error
```

## Design Notes

- **Templates are declarative** — they describe topology, not runtime state
- **Sessions are stateful** — they track what's actually connected right now
- **Channels are named** — names come from the template mappings and are matched to client-reported PipeWire names
- **Roles have cardinality** — each role is either single-client or multi-client, enforced at join time
- **Multi-client roles are receive-only** — they cannot be mapping sources (avoids ambiguity)
- **Broadcast is a sink type** — unauthenticated listen-only clients, separate from roles
