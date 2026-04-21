# PipeWire Integration

[← Back to Index](../index.md) · [CLI Client Overview](overview.md)

---

## Source/Sink Discovery

The CLI client uses PipeWire to enumerate available audio sources (inputs) and sinks (outputs) on the host system.

### Discovery Method

- Use PipeWire's D-Bus or `pw-cli` interface to list audio nodes
- Filter for audio source and sink nodes
- Watch for hotplug events (device added/removed)
- Report changes to server via control data channel

### Naming

By default, sources/sinks are reported using their PipeWire node names. These can be overridden:

| Env Variable | Overrides |
|---|---|
| `CROSSTALK_SOURCE_NAME` | Name for the primary audio source |
| `CROSSTALK_SINK_NAME` | Name for the primary audio sink |

When env vars are set, the client reports only the specified source/sink (ignoring others). When unset, all discovered sources/sinks are reported.

## Codec Reporting

The client reports supported audio codecs to the server using standard WebRTC SDP codec names:

- `opus/48000/2` (Opus, 48kHz, stereo) — expected default
- Additional codecs based on what Pion supports

This report is sent on initial connection and updated if capabilities change.

## PipeWire ↔ WebRTC Bridge

When the server commands a channel binding:

1. Client receives `BindChannel` command (source name, track ID)
2. Client locates the matching PipeWire node
3. Client reads PCM audio from PipeWire and feeds it to the Pion media track (for sources)
4. Client receives audio from the Pion media track and writes PCM to PipeWire (for sinks)

The bridge handles:
- Sample rate conversion (if PipeWire and WebRTC differ)
- Buffer management to minimize latency
- Graceful handling if the PipeWire node disappears mid-stream

## K2B Specifics

On the KickPi K2B, the TRRS jack appears as a single PipeWire audio node with both input and output. The client should handle this as separate source/sink even though it's one physical device.

> See [Hardware](hardware.md) for board-specific details.
