# ct-play

`ct-play` authenticates to a CrossTalk server, lists assigned sessions and
channels, and streams a WAV or MP3 file into a selected channel over the
production one-time media-ticket WebRTC path.

## Build

```bash
task build:play   # → bin/ct-play
```

## Runtime dependency

Playback requires `ffmpeg` on `PATH`. Session and channel listing do not.

## Configuration

Precedence: **explicit flag > `CT_PLAY_*` environment > config file > default**.

| Setting | Flag | Environment | Config key |
|---|---|---|---|
| Server base URL | `--host` | `CT_PLAY_HOST` | `host` |
| Username | `--username` | `CT_PLAY_USERNAME` | `username` |
| Password | `--password` | `CT_PLAY_PASSWORD` | `password` |
| Session ID | `--session` | `CT_PLAY_SESSION_ID` | `session_id` |
| Channel ID | `--channel` | `CT_PLAY_CHANNEL_ID` | `channel_id` |
| Config path | `--config` | `CT_PLAY_CONFIG` | n/a |

Default config search (when `--config` is omitted):

```text
$XDG_CONFIG_HOME/crosstalk/ct-play.{yaml,yml,json,toml}
$HOME/.config/crosstalk/ct-play.*
```

Prefer `CT_PLAY_PASSWORD` or a mode-`0600` config file over `--password` so the
secret does not appear in process listings. World/group-readable password
config files produce a warning.

TLS verification stays enabled for `https://` hosts.

## Commands

### List assigned sessions

```bash
ct-play --host https://ct.example --username alice --password "$CT_PLAY_PASSWORD" session
# or
CT_PLAY_HOST=https://ct.example CT_PLAY_USERNAME=alice CT_PLAY_PASSWORD=... ct-play session
```

Stdout is tabular: `ID\tNAME` (sorted by name, then ID). Translators see only
assigned sessions; admins see all.

### List channels

```bash
ct-play --session SESSION_ID channel
```

Stdout: `ID\tNAME\tTYPE`. Requires a resolved session ID. A channel listed here
is not necessarily publishable for the authenticated role.

### Play a file

```bash
ct-play --session SESSION_ID --channel CHANNEL_ID tone.wav
ct-play speech.mp3   # session/channel from env or config
```

Supported extensions: `.wav`, `.mp3` (case-insensitive).

Playback:

1. Logs in (access token kept in memory only).
2. Verifies the session is assigned and the channel belongs to it.
3. Mints a one-time media ticket (`POST /api/webrtc/token`) scoped to the channel.
4. Rejects playback if the ticket's `produce_channel_ids` does not include the channel.
5. Connects to `/api/sessions/{id}/ws` with that ticket.
6. Streams real-time Opus RTP via ffmpeg (`-re`, 48 kHz, 20 ms frames).
7. Exits zero at EOF after clean transport shutdown.

Tickets are never printed or persisted. A single pre-media remint is allowed if
admission fails before any RTP is sent.

## Authorization caveat

Server RBAC decides publishability. Typical defaults:

| Role | May produce |
|---|---|
| Translator / Admin | broadcast channels |
| ABC | feed channels |
| Listener | none |

A translator can list a feed channel but cannot publish into it. `ct-play`
surfaces that as a clear non-zero error.

## Examples

Config-only:

```yaml
# ~/.config/crosstalk/ct-play.yaml  (chmod 600)
host: https://ct.example
username: alice
password: "..."
session_id: 01H...
channel_id: 01H...
```

```bash
ct-play ./announcement.mp3
```

Environment-only:

```bash
export CT_PLAY_HOST=https://ct.example
export CT_PLAY_USERNAME=alice
export CT_PLAY_PASSWORD=...
export CT_PLAY_SESSION_ID=01H...
export CT_PLAY_CHANNEL_ID=01H...
ct-play ./tone.wav
```

## Tests

```bash
task test:e2e:play
```

Launches the real `bin/ct-play` against a PostgreSQL-backed server, proves
session/channel discovery, WAV 1 kHz content at a real listener, MP3 non-silence
delivery, RBAC denial, and secret redaction.
