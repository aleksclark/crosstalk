# Phase 3: Control Channel + Protobuf

[← Roadmap](index.md)

**Status**: `NOT STARTED`  
**Depends on**: Phase 2 (WebRTC data channel must work)

Protobuf code generation and control data channel message handling. After this phase, connected clients exchange typed messages with the server.

## Tasks

### 3.1 Protobuf Code Generation
- [ ] Install `protoc` + `protoc-gen-go` as project tooling
- [ ] `task generate:proto` produces Go types in `proto/gen/go/crosstalk/v1/`
- [ ] Generated code compiles and is importable from `server/` and `cli/`
- [ ] Add generated code to `.gitignore` OR commit it (decide and document)

**Test**: `task generate:proto` succeeds. `server/` and `cli/` compile with the generated types imported.

### 3.2 Control Channel Message Handler
- [ ] Replace data channel echo (Phase 2) with Protobuf `ControlMessage` parsing
- [ ] Server-side dispatcher: unmarshal `ControlMessage`, switch on `payload` oneof
- [ ] Handle `Hello` → register client capabilities (sources, sinks, codecs), respond with `Welcome`
- [ ] Handle `ClientStatus` → update client state in registry
- [ ] Handle `ChannelStatus` → update channel binding state
- [ ] Handle `LogEntry` → forward to session log subscribers (store in memory for now)

**Test**: `server/pion/control_test.go` — in-process peer sends `Hello` protobuf, server responds with `Welcome` protobuf. Verify fields round-trip correctly.

### 3.3 Client-Side Control Channel (Go Library)
- [ ] Shared control channel client code (usable by both `cli/` and test helpers)
- [ ] Send `Hello` with source/sink/codec info
- [ ] Receive and parse `Welcome`, `BindChannel`, `UnbindChannel`, `SessionEvent`
- [ ] Send `LogEntry` messages

**Test**: In-process test: client sends `Hello`, receives `Welcome` with correct `client_id`.

### 3.4 JoinSession Message
- [ ] Handle `JoinSession` on the control channel
- [ ] Validate session exists, role exists, cardinality not exceeded
- [ ] Register client in session, respond with `SessionEvent{CLIENT_JOINED}`
- [ ] Reject with `SessionEvent{ROLE_REJECTED}` if role is full

**Test**: Create a session via REST, connect a client via WebRTC, send `JoinSession`, verify client appears in `GET /api/sessions/:id` response. Try joining a full single-client role → receive ROLE_REJECTED.

## Exit Criteria

1. `task generate:proto` produces valid Go types
2. Connected client sends `Hello`, server responds with `Welcome`
3. `JoinSession` message adds client to a session with role validation
4. `LogEntry` messages flow from client to server
5. All verified in `go test`

## Spec Updates

- 5.4 Protobuf Schema → 3
- 1.3 Protocol Stack → 3
