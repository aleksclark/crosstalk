# Step 07: Audio Booth Connector (ABC) Management

## Goal

ABCs are hardware boxes (KickPi boards) with audio I/O that connect to the server as persistent WebRTC peers. Admins configure them through the server UI, and the boxes auto-connect using their stored credentials.

## ABC Lifecycle

```
1. Admin creates ABC in server UI → gets a token
2. Admin configures the ABC hardware: server URL + token
3. ABC boots → reads config → connects to server via WebSocket → WebRTC
4. Server recognizes ABC by token → assigns to configured session
5. ABC's audio input becomes a source in the session
6. ABC's audio output receives the feed channel (or a custom channel)
7. If connection drops → ABC reconnects with exponential backoff
8. Admin can send "restart" command via control channel
```

## Tasks

### 7.1 Server-side ABC management
- [ ] CRUD API for ABCs (see Step 01 endpoints)
- [ ] Token generation: opaque bearer token (ct_abc_xxxx format)
- [ ] Session assignment: `PUT /api/abcs/{id}` with `session_id` field
- [ ] Status tracking: connected/disconnected, last_seen, current peer_id
- [ ] Restart command: `POST /api/abcs/{id}/restart` → sends RestartCommand via control channel

### 7.2 ABC authentication flow
- [ ] ABC connects to `/ws/signaling` with token in query param
- [ ] Server validates token hash against `abcs` table
- [ ] On valid: create PeerConnection, send Welcome with assigned session_id
- [ ] ABC sends Hello with client_type="abc" and audio capabilities
- [ ] Server auto-joins ABC to assigned session (no JoinSession message needed)

### 7.3 ABC client (Go CLI)
- [ ] Reuse v2 CLI client structure: `cmd/ct-abc/main.go`
- [ ] Config: `config.json` with server_url, token, source_name, sink_name
- [ ] PipeWire integration for audio I/O (reuse v2's `pipewire/` package)
- [ ] Auto-reconnect with exponential backoff (reuse v2's `Client` pattern)
- [ ] Handle RestartCommand: graceful shutdown + immediate reconnect
- [ ] Handle SessionAssignment: if session changes, rebind audio

### 7.4 ABC control channel
- [ ] WebRTC data channel for server ↔ ABC communication
- [ ] Messages: RestartCommand, SessionAssignment, MixUpdate (for output channel selection)
- [ ] ABC reports: SourceStatus (audio levels), connection quality metrics
- [ ] Server can reconfigure ABC without physical access

### 7.5 ABC hardware integration
- [ ] Reuse v2's k2b-board setup scripts
- [ ] systemd service for auto-start on boot
- [ ] Display service (ILI9341) shows connection status + audio levels
- [ ] Watchdog: restart service if no audio for configurable timeout

## ABC Config File

```json
{
  "server_url": "https://ct.example.com",
  "token": "ct_abc_xxxxxxxxxxxxx",
  "source_name": "Booth A",
  "sink_name": "Booth A Output",
  "audio": {
    "input_device": "alsa_input.usb-xxxx",
    "output_device": "alsa_output.usb-xxxx",
    "sample_rate": 48000
  },
  "display": {
    "enabled": true,
    "spi_device": "/dev/spidev1.0"
  },
  "log_level": "info"
}
```

## Acceptance Criteria

- [ ] Admin creates ABC via API → gets token
- [ ] ABC client connects with token → appears as connected in API
- [ ] ABC audio input becomes a source in the assigned session
- [ ] ABC receives feed channel audio on its output
- [ ] Admin changes session assignment → ABC rebinds to new session
- [ ] Admin sends restart → ABC disconnects and reconnects
- [ ] ABC handles network loss → reconnects automatically
- [ ] ABC connection status visible in admin UI

## Reusable from v2

- `cli/pion/client.go` — Client struct with reconnection logic, factory pattern
- `cli/pion/connection.go` — WebSocket + PeerConnection lifecycle
- `cli/pion/auth.go` — WebRTC token request
- `cli/pipewire/` — PipeWire source/sink discovery
- `cli/pion/audio.go` — Audio track capture/playback
- `cli/display/` — ILI9341 display rendering
- `k2b-board/` — Hardware setup scripts, systemd services
