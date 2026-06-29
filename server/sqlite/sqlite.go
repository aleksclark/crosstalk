// Package sqlite implements the persistence layer using SQLite.
package sqlite

import (
	"database/sql"
	"fmt"
	"log/slog"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps a sql.DB connection with CrossTalk-specific helpers.
type DB struct {
	*sql.DB
	log *slog.Logger
}

// Open creates a new SQLite connection and runs migrations.
func Open(dsn string, log *slog.Logger) (*DB, error) {
	db, err := sql.Open("sqlite3", dsn+"?_journal_mode=WAL&_foreign_keys=ON&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("sqlite ping: %w", err)
	}

	d := &DB{DB: db, log: log}
	if err := d.migrate(); err != nil {
		return nil, fmt.Errorf("sqlite migrate: %w", err)
	}

	return d, nil
}

// migrate runs all schema migrations in order.
func (d *DB) migrate() error {
	// Create migrations tracking table
	if _, err := d.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`); err != nil {
		return err
	}

	for _, m := range migrations {
		var exists int
		if err := d.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", m.version).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			continue
		}

		d.log.Info("applying migration", "version", m.version, "name", m.name)
		if _, err := d.Exec(m.sql); err != nil {
			return fmt.Errorf("migration %d (%s): %w", m.version, m.name, err)
		}
		if _, err := d.Exec("INSERT INTO schema_migrations (version) VALUES (?)", m.version); err != nil {
			return err
		}
	}

	return nil
}

type migration struct {
	version int
	name    string
	sql     string
}

var migrations = []migration{
	{
		version: 1,
		name:    "initial_schema",
		sql: `
CREATE TABLE sessions (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    description     TEXT DEFAULT '',
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    broadcast_token TEXT UNIQUE
);

CREATE TABLE channels (
    id         TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    type       TEXT NOT NULL CHECK(type IN ('feed', 'broadcast')),
    created_at TEXT NOT NULL,
    UNIQUE(session_id, name)
);

CREATE TABLE sources (
    id         TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    origin     TEXT NOT NULL CHECK(origin IN ('abc', 'translator', 'admin')),
    peer_id    TEXT,
    connected  INTEGER DEFAULT 0,
    first_seen TEXT NOT NULL,
    last_seen  TEXT NOT NULL,
    UNIQUE(session_id, name)
);

CREATE TABLE channel_mix (
    id         TEXT PRIMARY KEY,
    channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    source_id  TEXT NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    muted      INTEGER DEFAULT 0,
    level      REAL DEFAULT 1.0 CHECK(level >= 0.0 AND level <= 2.0),
    UNIQUE(channel_id, source_id)
);

CREATE TABLE abcs (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    token_hash   TEXT NOT NULL UNIQUE,
    session_id   TEXT REFERENCES sessions(id) ON DELETE SET NULL,
    connected    INTEGER DEFAULT 0,
    last_seen    TEXT,
    created_at   TEXT NOT NULL
);

CREATE TABLE users (
    id            TEXT PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL CHECK(role IN ('admin', 'translator')),
    created_at    TEXT NOT NULL
);

CREATE TABLE translator_sessions (
    translator_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id    TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    PRIMARY KEY(translator_id, session_id)
);

CREATE TABLE refresh_tokens (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE recordings (
    id          TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    source_id   TEXT REFERENCES sources(id) ON DELETE SET NULL,
    channel_id  TEXT REFERENCES channels(id) ON DELETE SET NULL,
    file_path   TEXT NOT NULL,
    started_at  TEXT NOT NULL,
    ended_at    TEXT,
    size_bytes  INTEGER DEFAULT 0
);
`,
	},
}
