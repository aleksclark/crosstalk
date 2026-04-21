# Protobuf Schema

[← Back to Index](../index.md) · [Data Model Overview](overview.md)

---

## Overview

All messages on the WebRTC `control` data channel use Protocol Buffers encoding. The `.proto` files are the source of truth, generating:
- **Go types** — used by server and CLI client
- **TypeScript types** — used by admin web UI

## Message Envelope

Every control channel message is wrapped in an envelope:

```protobuf
syntax = "proto3";
package crosstalk.v1;

message ControlMessage {
  oneof payload {
    // Client → Server
    Hello hello = 1;
    ClientStatus client_status = 2;
    ChannelStatus channel_status = 3;
    JoinSession join_session = 4;

    // Server → Client
    Welcome welcome = 10;
    BindChannel bind_channel = 11;
    UnbindChannel unbind_channel = 12;
    SessionEvent session_event = 13;

    // Bidirectional
    LogEntry log_entry = 20;
  }
}
```

## Client → Server Messages

```protobuf
message Hello {
  repeated SourceInfo sources = 1;
  repeated SinkInfo sinks = 2;
  repeated CodecInfo codecs = 3;
}

message SourceInfo {
  string name = 1;
  string type = 2;  // "audio", "video"
}

message SinkInfo {
  string name = 1;
  string type = 2;
}

message CodecInfo {
  string name = 1;       // e.g., "opus/48000/2"
  string media_type = 2; // "audio" or "video"
}

message ClientStatus {
  ClientState state = 1;
  repeated SourceInfo sources = 2;
  repeated SinkInfo sinks = 3;
  repeated CodecInfo codecs = 4;
}

enum ClientState {
  CLIENT_READY = 0;
  CLIENT_BUSY = 1;
  CLIENT_ERROR = 2;
}

message ChannelStatus {
  string channel_id = 1;
  ChannelState state = 2;
  string error_message = 3;
  uint64 bytes_transferred = 4;
}

enum ChannelState {
  CHANNEL_IDLE = 0;
  CHANNEL_BINDING = 1;
  CHANNEL_ACTIVE = 2;
  CHANNEL_ERROR = 3;
}

message JoinSession {
  string session_id = 1;
  string role = 2;
}
```

## Server → Client Messages

```protobuf
message Welcome {
  string client_id = 1;
  string server_version = 2;
}

message BindChannel {
  string channel_id = 1;
  string local_name = 2;
  Direction direction = 3;
  string track_id = 4;
}

message UnbindChannel {
  string channel_id = 1;
}

enum Direction {
  SOURCE = 0;
  SINK = 1;
}

message SessionEvent {
  SessionEventType type = 1;
  string message = 2;
  string session_id = 3;
}

enum SessionEventType {
  SESSION_CLIENT_JOINED = 0;
  SESSION_CLIENT_LEFT = 1;
  SESSION_CHANNEL_BOUND = 2;
  SESSION_CHANNEL_UNBOUND = 3;
  SESSION_ENDED = 4;
  SESSION_RECORDING_STARTED = 5;
  SESSION_RECORDING_STOPPED = 6;
  SESSION_BROADCAST_CLIENT_JOINED = 7;
  SESSION_BROADCAST_CLIENT_LEFT = 8;
  SESSION_ROLE_REJECTED = 9;          // e.g., single-client role already occupied
}
```

## Bidirectional Messages

```protobuf
message LogEntry {
  int64 timestamp = 1;     // Unix millis
  LogSeverity severity = 2;
  string source = 3;        // Client ID or "server"
  string message = 4;
}

enum LogSeverity {
  LOG_DEBUG = 0;
  LOG_INFO = 1;
  LOG_WARN = 2;
  LOG_ERROR = 3;
}
```

## Code Generation

```bash
# Via go-task
task generate:proto
```

Which runs:

```bash
protoc --go_out=proto/gen/go --go_opt=paths=source_relative \
       --ts_out=proto/gen/ts \
       proto/crosstalk/v1/*.proto
```

> See [Dev Environment > Taskfile](../dev-environment/taskfile.md) for all available tasks.

Generated code locations:
- `proto/gen/go/crosstalk/v1/` — Go types
- `proto/gen/ts/crosstalk/v1/` — TypeScript types
