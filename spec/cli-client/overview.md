# CLI Client

[← Back to Index](../index.md)

Headless Go client that runs on edge devices (primarily KickPi K2B). Connects to the CrossTalk server, exposes local PipeWire audio sources/sinks, and binds channels on command from the server.

---

## Responsibilities

1. Authenticate to server and establish WebRTC connection
2. Discover and report local PipeWire sources/sinks
3. Report supported audio codecs
4. Bind/unbind local audio devices to WebRTC tracks on server command
5. Report connection status continuously
6. Stream structured logs to server over control channel

## Sections

| Section | Description |
|---------|-------------|
| [Auth & Connection](auth-connection.md) | REST auth, WebRTC establishment |
| [PipeWire Integration](pipewire.md) | Source/sink discovery, naming, codec reporting |
| [Channel Lifecycle](channel-lifecycle.md) | Server-commanded binding, status reporting |
| [Hardware: KickPi K2B](hardware.md) | Board specifics, TRRS audio, deployment |

## Configuration

JSON config file with JSON Schema validation (same approach as server). Default path: `crosstalk.json` (override with `--config` or `CROSSTALK_CONFIG` env var).

```json
{
  "$schema": "./config.schema.json",
  "server_url": "https://crosstalk.local",
  "token": "ct_abc123...",
  "source_name": "translator-mic",
  "sink_name": "translator-speakers",
  "log_level": "info"
}
```

Environment variables override config file values when both are set:

| Config Field | Env Override |
|---|---|
| `server_url` | `CROSSTALK_SERVER` |
| `token` | `CROSSTALK_TOKEN` |
| `source_name` | `CROSSTALK_SOURCE_NAME` |
| `sink_name` | `CROSSTALK_SINK_NAME` |
| `log_level` | `CROSSTALK_LOG_LEVEL` |

> See [Server > Configuration](../server/configuration.md) for full config/schema details.

## Logging

Structured JSON to stdout (always) + Protobuf `LogEntry` to the server via the control data channel (when connected). If the WebRTC connection drops, logs continue to stdout only.

> See [Server > Logging](../server/logging.md) for format details.

## Runtime Behavior

```
Startup:
  1. Load config (JSON file + env overrides), validate against schema
  2. Authenticate to REST API
  3. Request WebRTC token
  4. Open signaling WebSocket, establish WebRTC connection + control data channel
  5. Discover PipeWire sources/sinks
  6. Report capabilities to server via control channel

Steady state:
  - Listen for server commands on control channel
  - React to PipeWire device changes (hotplug)
  - Report status changes to server
  - Stream logs to server over control channel
  - Reconnect on connection loss (exponential backoff)
```
