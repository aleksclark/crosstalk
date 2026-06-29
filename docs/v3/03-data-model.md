# Step 03: Data Model (Sessions, Channels, Sources)

## Goal

Define the core data model for v3. Sessions are long-lived, channels are audio streams within a session, sources are connection points that produce or consume audio.

## Key Design Decisions

1. **Sessions are persistent** — created once for a recurring event ("10am Sunday service"), not per-occurrence
2. **Channels are defined at session creation** — minimum: 1 "feed" + 1 "broadcast"
3. **Sources are ephemeral** — they come and go as devices connect/disconnect, but their mix state persists
4. **Mix state is per-channel, per-source** — each channel remembers mute/level for every source that ever connected

## Schema

### sessions
```sql
CREATE TABLE sessions (
    id          TEXT PRIMARY KEY,  -- ULID
    name        TEXT NOT NULL,
    description TEXT DEFAULT '',
    created_at  TEXT NOT NULL,     -- RFC3339
    updated_at  TEXT NOT NULL,
    broadcast_token TEXT UNIQUE    -- regenerable unique token for broadcast URL
);
```

### channels
```sql
CREATE TABLE channels (
    id         TEXT PRIMARY KEY,  -- ULID
    session_id TEXT NOT NULL REFERENCES sessions(id),
    name       TEXT NOT NULL,
    type       TEXT NOT NULL CHECK(type IN ('feed', 'broadcast')),
    created_at TEXT NOT NULL,
    UNIQUE(session_id, name)
);
```

### sources
Sources represent any audio-producing connection. They are created when a WebRTC peer sends audio and persist for mix state.

```sql
CREATE TABLE sources (
    id         TEXT PRIMARY KEY,  -- ULID
    session_id TEXT NOT NULL REFERENCES sessions(id),
    name       TEXT NOT NULL,     -- human-readable (e.g. "Booth A mic", "Translator: Maria")
    origin     TEXT NOT NULL,     -- "abc", "translator", "admin"
    peer_id    TEXT,              -- current WebRTC peer ID (NULL if disconnected)
    connected  INTEGER DEFAULT 0,
    first_seen TEXT NOT NULL,
    last_seen  TEXT NOT NULL,
    UNIQUE(session_id, name)
);
```

### channel_mix
Per-channel mix state for each source.

```sql
CREATE TABLE channel_mix (
    id         TEXT PRIMARY KEY,
    channel_id TEXT NOT NULL REFERENCES channels(id),
    source_id  TEXT NOT NULL REFERENCES sources(id),
    muted      INTEGER DEFAULT 0,
    level      REAL DEFAULT 1.0 CHECK(level >= 0.0 AND level <= 2.0),
    UNIQUE(channel_id, source_id)
);
```

### abcs
```sql
CREATE TABLE abcs (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    token_hash   TEXT NOT NULL UNIQUE,
    session_id   TEXT REFERENCES sessions(id),  -- assigned session (nullable)
    connected    INTEGER DEFAULT 0,
    last_seen    TEXT,
    created_at   TEXT NOT NULL
);
```

### users
```sql
CREATE TABLE users (
    id            TEXT PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL CHECK(role IN ('admin', 'translator')),
    created_at    TEXT NOT NULL
);
```

### translator_sessions
```sql
CREATE TABLE translator_sessions (
    translator_id TEXT NOT NULL REFERENCES users(id),
    session_id    TEXT NOT NULL REFERENCES sessions(id),
    PRIMARY KEY(translator_id, session_id)
);
```

### recordings
```sql
CREATE TABLE recordings (
    id          TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL REFERENCES sessions(id),
    source_id   TEXT REFERENCES sources(id),   -- NULL for channel recordings
    channel_id  TEXT REFERENCES channels(id),  -- NULL for source recordings
    file_path   TEXT NOT NULL,
    started_at  TEXT NOT NULL,
    ended_at    TEXT,
    size_bytes  INTEGER DEFAULT 0
);
```

## Domain Types (Go)

```go
type Session struct {
    ID             string
    Name           string
    Description    string
    BroadcastToken string
    CreatedAt      time.Time
    UpdatedAt      time.Time
}

type Channel struct {
    ID        string
    SessionID string
    Name      string
    Type      ChannelType  // "feed" or "broadcast"
    CreatedAt time.Time
}

type ChannelType string
const (
    ChannelFeed      ChannelType = "feed"
    ChannelBroadcast ChannelType = "broadcast"
)

type Source struct {
    ID        string
    SessionID string
    Name      string
    Origin    SourceOrigin  // "abc", "translator", "admin"
    PeerID    *string       // nil if disconnected
    Connected bool
    FirstSeen time.Time
    LastSeen  time.Time
}

type SourceOrigin string
const (
    OriginABC        SourceOrigin = "abc"
    OriginTranslator SourceOrigin = "translator"
    OriginAdmin      SourceOrigin = "admin"
)

type MixEntry struct {
    ID        string
    ChannelID string
    SourceID  string
    Muted     bool
    Level     float64  // 0.0 - 2.0, default 1.0
}

type ABC struct {
    ID        string
    Name      string
    TokenHash string
    SessionID *string  // assigned session
    Connected bool
    LastSeen  *time.Time
    CreatedAt time.Time
}
```

## Tasks

### 3.1 Domain package
- [ ] Define all types in root `crosstalk` package (zero external deps)
- [ ] Define service interfaces: `SessionService`, `ChannelService`, `SourceService`, `MixService`, `ABCService`
- [ ] Validation methods on domain types

### 3.2 SQLite implementation
- [ ] Schema migration (Goose or manual versioned SQL)
- [ ] Implement all service interfaces against SQLite
- [ ] Unit tests for each service

### 3.3 Wire up to API handlers
- [ ] Connect Huma handlers to service layer
- [ ] Ensure OpenAPI spec reflects all types correctly

## Acceptance Criteria

- [ ] All tables created via migration
- [ ] CRUD operations work for sessions, channels, sources, mix entries, ABCs
- [ ] Mix state persists across source disconnect/reconnect (source matched by session_id + name)
- [ ] `go test ./sqlite/...` passes with full coverage of service interfaces

## Reusable from v2

- `server/sqlite/sqlite.go` — connection setup, WAL mode, migrations pattern
- `server/sqlite/time.go` — RFC3339 time helpers
- `server/domain.go` — pattern of zero-dep domain package with service interfaces
