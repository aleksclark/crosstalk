-- +goose Up

CREATE TABLE users (
    id            TEXT PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at    TEXT NOT NULL
);

CREATE TABLE api_tokens (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL
);

CREATE TABLE session_templates (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    is_default INTEGER NOT NULL DEFAULT 0,
    roles      TEXT NOT NULL DEFAULT '[]',
    mappings   TEXT NOT NULL DEFAULT '[]',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE sessions (
    id          TEXT PRIMARY KEY,
    template_id TEXT NOT NULL REFERENCES session_templates(id),
    name        TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'waiting',
    created_at  TEXT NOT NULL,
    ended_at    TEXT
);

CREATE INDEX idx_sessions_status ON sessions(status);

CREATE TABLE session_clients (
    id              TEXT PRIMARY KEY,
    session_id      TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    role            TEXT NOT NULL,
    client_id       TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'connected',
    connected_at    TEXT NOT NULL,
    disconnected_at TEXT
);

CREATE INDEX idx_session_clients_session_id ON session_clients(session_id);
