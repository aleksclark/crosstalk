# Auth & Dashboard

[← Back to Index](../index.md) · [Admin Web Overview](overview.md)

---

## Authentication

- Username/password login via `POST /api/auth/login`
- Server returns session cookie
- All subsequent API calls include the session cookie
- Logout via `POST /api/auth/logout`
- Redirect to `/login` on 401 responses

## Dashboard (`/dashboard`)

The main landing page after login. Shows a real-time overview of the system.

### Server Status

- Server uptime
- Active sessions count
- Connected WebRTC clients count
- Recording status (active recordings, disk usage)

### Connected Clients

Table of currently connected WebRTC clients:

| Column | Description |
|--------|-------------|
| Client ID | Identifier |
| Role | Current session role (if any) |
| Session | Session name (if connected) |
| Sources | Available audio sources |
| Sinks | Available audio sinks |
| Codecs | Supported codecs |
| Status | Connection state |
| Connected Since | Timestamp |

- Auto-refreshes (polling or server-sent events)
- Click client row to see detailed status
