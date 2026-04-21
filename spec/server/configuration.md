# Configuration

[← Back to Index](../index.md) · [Server Overview](overview.md)

---

## Overview

Both the server and CLI client use **JSON configuration files** validated against a **JSON Schema**. On startup, the application validates the config and logs warnings for any fields that don't match the schema — but still attempts to start (warn, don't crash) for forward-compatibility.

## Format

JSON. Not TOML, not YAML. JSON is universally parseable, has a mature schema ecosystem, and avoids the whitespace/quoting ambiguities of alternatives.

## Server Config

Default path: `ct-server.json` (override with `--config` flag or `CROSSTALK_CONFIG` env var).

```json
{
  "$schema": "./config.schema.json",
  "listen": ":8080",
  "db_path": "./data/crosstalk.db",
  "recording_path": "./data/recordings",
  "log_level": "info",
  "webrtc": {
    "stun_servers": ["stun:stun.l.google.com:19302"],
    "turn": {
      "enabled": false,
      "server": "",
      "username": "",
      "credential": ""
    }
  },
  "auth": {
    "session_secret": "change-me-in-production",
    "webrtc_token_lifetime": "24h"
  },
  "web": {
    "dev_mode": false,
    "dev_proxy_url": "http://localhost:5173"
  }
}
```

## CLI Client Config

Default path: `crosstalk.json` (override with `--config` flag or `CROSSTALK_CONFIG` env var).

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

Environment variables still work as overrides — if both a config file and an env var are set, the env var wins. This lets the config file hold defaults while systemd units or Docker containers override specific values.

| Config Field | Env Override |
|---|---|
| `server_url` | `CROSSTALK_SERVER` |
| `token` | `CROSSTALK_TOKEN` |
| `source_name` | `CROSSTALK_SOURCE_NAME` |
| `sink_name` | `CROSSTALK_SINK_NAME` |
| `log_level` | `CROSSTALK_LOG_LEVEL` |

## JSON Schema

Each component ships a JSON Schema file that describes the expected config structure:

- `server/config.schema.json`
- `cli/config.schema.json`

### Schema Usage

1. **Runtime validation**: On startup, load config → validate against schema → log warnings for unknown/invalid fields → continue with valid fields
2. **Editor support**: The `$schema` field in the config file gives editors (VS Code, etc.) autocomplete and inline validation
3. **Documentation**: The schema is the authoritative reference for all config options

### Validation Behavior

| Condition | Behavior |
|---|---|
| Config file missing | Use defaults + env overrides, log info |
| Valid config | Load normally |
| Unknown field | Log warning, ignore the field, continue |
| Wrong type for known field | Log warning, use default for that field |
| Missing required field (no default) | Log error, exit |
| Schema itself missing | Skip validation, log warning |

The philosophy: **warn and continue** wherever possible. A mistyped field name shouldn't prevent the server from starting — it should be obvious from the logs that something is off.

## Schema Versioning

The schema includes a `version` field. When the config format changes:
- Old fields are kept with deprecation warnings
- New fields have defaults so old configs still work
- Breaking changes bump the version and the app logs a clear migration message
