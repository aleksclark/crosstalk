# Phase 5: Session Orchestration + Audio Forwarding

[← Roadmap](index.md)

**Status**: `NOT STARTED`  
**Depends on**: Phase 3 (control channel) + Phase 4 (CLI client can connect)

The heart of CrossTalk — create sessions from templates, assign clients to roles, resolve channel bindings, and forward audio tracks between peer connections.

## Tasks

### 5.1 Session Lifecycle
- [ ] `POST /api/sessions` creates session from template, status `waiting`
- [ ] When first `JoinSession` arrives and a binding activates → status `active`
- [ ] `DELETE /api/sessions/:id` → sends `SessionEnded` to all clients, status `ended`
- [ ] Partial sessions: missing roles don't block — only available bindings activate

**Test**: Create session via REST → status is `waiting`. Client joins → status is `active`. End session via REST → status is `ended`, client receives `SessionEnded` event.

### 5.2 Binding Resolution
- [ ] Given connected clients and template mappings, compute which bindings can activate
- [ ] Only activate bindings where both source and sink roles are filled
- [ ] For `→ record` bindings, activate when the source role is filled
- [ ] For `→ broadcast` bindings, activate when the source role is filled
- [ ] When a new client joins or disconnects, re-evaluate all bindings

**Test**: `server/domain_test.go` (pure domain logic, no deps) — given a template and set of connected roles, `ResolveBindings()` returns the correct active binding list. Test partial states, role add/remove.

### 5.3 BindChannel Commands
- [ ] When a binding activates, server sends `BindChannel` to both source and sink clients
- [ ] Source client receives: "send your 'mic' source on track X"
- [ ] Sink client receives: "play track X on your 'output' sink"
- [ ] Track allocation: server creates Pion tracks for each active binding

**Test**: Two in-process Pion clients join a session. Client A receives `BindChannel{direction: SOURCE}`, client B receives `BindChannel{direction: SINK}`.

### 5.4 Audio Track Forwarding (SFU)
- [ ] Server receives audio track from source client's PeerConnection
- [ ] Server forwards RTP packets as-is to sink client's PeerConnection
- [ ] No transcoding, no mixing — pure forwarding
- [ ] Handle track lifecycle: remove forwarding when binding deactivates

**Test**: Two in-process Pion clients. Client A sends Opus audio track. Client B receives audio track. Compare sent RTP packets to received RTP packets — payload bytes must match (proving pure forwarding).

### 5.5 CLI Client: PipeWire ↔ WebRTC Bridge
- [ ] On `BindChannel{direction: SOURCE}`: read PCM from PipeWire node, encode Opus, send as Pion track
- [ ] On `BindChannel{direction: SINK}`: receive Pion track, decode Opus, write PCM to PipeWire node
- [ ] On `UnbindChannel`: tear down the bridge
- [ ] Report `ChannelStatus{ACTIVE}` when bridge is running

**Test**: Start server + CLI client with PipeWire loopback. Create session, join client. Play a 1kHz tone into loopback source. Verify `ChannelStatus{ACTIVE}` and `bytes_transferred > 0`.

### 5.6 Role Cardinality Enforcement
- [ ] Single-client role: reject second client with `SESSION_ROLE_REJECTED`
- [ ] Multi-client role: accept multiple clients, each gets sink bindings
- [ ] Multi-client roles cannot be mapping sources (validated at template creation, but also enforced at runtime)

**Test**: Create template with single-client role. Client A joins → success. Client B tries same role → receives `ROLE_REJECTED`. Create template with multi-client role, two clients join → both succeed.

## Exit Criteria

Two Pion test clients join a session in different roles. Audio sent by client A arrives at client B per the template mapping. Verified by comparing sent/received RTP payloads.

CLI client with PipeWire loopback: audio played into loopback source → sent via WebRTC → received by second client.

## Spec Updates

- 2.3 Session Orchestration → 5
- 3.3 Channel Lifecycle → 4
- 5.2 Session Templates → 4
- 5.3 Sessions → 4
