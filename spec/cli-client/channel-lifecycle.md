# Channel Lifecycle

[← Back to Index](../index.md) · [CLI Client Overview](overview.md)

---

## Overview

The CLI client does not autonomously bind channels. All channel binding is directed by the server via the control data channel. The client is a passive participant that executes commands and reports status.

## Command Flow

### BindChannel

Server instructs client to connect a local source/sink to a WebRTC track:

```protobuf
message BindChannel {
  string channel_id = 1;
  string local_name = 2;      // PipeWire source/sink name
  Direction direction = 3;     // SOURCE or SINK
  string track_id = 4;        // WebRTC track to bind to
}
```

Client response:
1. Locate the PipeWire node matching `local_name`
2. If found: start the PipeWire ↔ WebRTC bridge, report `ChannelStatus{ACTIVE}`
3. If not found: report `ChannelStatus{ERROR, "source not found"}`

### UnbindChannel

Server instructs client to disconnect a channel:

```protobuf
message UnbindChannel {
  string channel_id = 1;
}
```

Client tears down the bridge and reports `ChannelStatus{IDLE}`.

## Status Reporting

The client continuously reports channel status to the server:

```protobuf
enum ChannelState {
  IDLE = 0;
  BINDING = 1;
  ACTIVE = 2;
  ERROR = 3;
}

message ChannelStatus {
  string channel_id = 1;
  ChannelState state = 2;
  string error_message = 3;   // Only set when state == ERROR
  uint64 bytes_transferred = 4;
}
```

Status updates are sent on:
- State transitions (idle → binding → active)
- Errors (PipeWire node lost, track failure)
- Periodic heartbeat (every 10s while active, includes bytes_transferred)

## Client-Level Status

Separate from per-channel status, the client reports its overall state:

```protobuf
message ClientStatus {
  repeated SourceInfo sources = 1;
  repeated SinkInfo sinks = 2;
  repeated CodecInfo codecs = 3;
  ClientState state = 4;       // READY, BUSY, ERROR
}
```

Sent on:
- Initial connection (full capabilities report)
- PipeWire device changes (source/sink added or removed)
- Client state changes
