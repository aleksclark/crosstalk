// Package postgres implements the persistence layer using Bun ORM on top of
// PostgreSQL. It replaces the previous SQLite-backed implementation while
// preserving the crosstalk service interfaces.
package postgres

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/extra/bundebug"
)

// DB wraps a *bun.DB connection with CrossTalk-specific helpers.
type DB struct {
	*bun.DB
	log *slog.Logger
}

// Open creates a new PostgreSQL connection via Bun and runs migrations.
//
// The dsn is a standard PostgreSQL connection string, e.g.
// "postgres://user:pass@host:5432/dbname?sslmode=disable".
func Open(dsn string, log *slog.Logger) (*DB, error) {
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))

	bdb := bun.NewDB(sqldb, pgdialect.New())
	if log != nil && log.Enabled(context.Background(), slog.LevelDebug) {
		bdb.AddQueryHook(bundebug.NewQueryHook(bundebug.WithVerbose(true)))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := bdb.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("postgres ping: %w", err)
	}

	d := &DB{DB: bdb, log: log}
	if err := d.migrate(ctx); err != nil {
		return nil, fmt.Errorf("postgres migrate: %w", err)
	}

	return d, nil
}

// migrate runs all schema migrations in order, tracking applied versions.
func (d *DB) migrate(ctx context.Context) error {
	if _, err := d.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return err
	}

	for _, m := range migrations {
		var exists int
		if err := d.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM schema_migrations WHERE version = ?", m.version).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			continue
		}

		if d.log != nil {
			d.log.Info("applying migration", "version", m.version, "name", m.name)
		}
		for _, stmt := range m.statements {
			if _, err := d.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("migration %d (%s): %w", m.version, m.name, err)
			}
		}
		if _, err := d.ExecContext(ctx,
			"INSERT INTO schema_migrations (version) VALUES (?)", m.version); err != nil {
			return err
		}
	}

	return nil
}

type migration struct {
	version    int
	name       string
	statements []string
}

// migrations holds the ordered schema definition. Each statement is executed
// individually because the pgdriver protocol does not allow multiple
// statements in a single Exec.
var migrations = []migration{
	{
		version: 1,
		name:    "initial_schema",
		statements: []string{
			`CREATE TABLE IF NOT EXISTS sessions (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL,
    broadcast_token TEXT UNIQUE
)`,
			`CREATE TABLE IF NOT EXISTS channels (
    id         TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    type       TEXT NOT NULL CHECK(type IN ('feed', 'broadcast')),
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE(session_id, name)
)`,
			`CREATE TABLE IF NOT EXISTS sources (
    id         TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    origin     TEXT NOT NULL CHECK(origin IN ('abc', 'translator', 'admin')),
    peer_id    TEXT,
    connected  BOOLEAN NOT NULL DEFAULT FALSE,
    first_seen TIMESTAMPTZ NOT NULL,
    last_seen  TIMESTAMPTZ NOT NULL,
    UNIQUE(session_id, name)
)`,
			`CREATE TABLE IF NOT EXISTS channel_mix (
    id         TEXT PRIMARY KEY,
    channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    source_id  TEXT NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    muted      BOOLEAN NOT NULL DEFAULT FALSE,
    level      DOUBLE PRECISION NOT NULL DEFAULT 1.0 CHECK(level >= 0.0 AND level <= 2.0),
    UNIQUE(channel_id, source_id)
)`,
			`CREATE TABLE IF NOT EXISTS abcs (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    token_hash   TEXT NOT NULL UNIQUE,
    session_id   TEXT REFERENCES sessions(id) ON DELETE SET NULL,
    connected    BOOLEAN NOT NULL DEFAULT FALSE,
    last_seen    TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL
)`,
			`CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL CHECK(role IN ('admin', 'translator')),
    created_at    TIMESTAMPTZ NOT NULL
)`,
			`CREATE TABLE IF NOT EXISTS translator_sessions (
    translator_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id    TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    PRIMARY KEY(translator_id, session_id)
)`,
			`CREATE TABLE IF NOT EXISTS refresh_tokens (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
)`,
			`CREATE TABLE IF NOT EXISTS recordings (
    id          TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    source_id   TEXT REFERENCES sources(id) ON DELETE SET NULL,
    channel_id  TEXT REFERENCES channels(id) ON DELETE SET NULL,
    file_path   TEXT NOT NULL,
    started_at  TIMESTAMPTZ NOT NULL,
    ended_at    TIMESTAMPTZ,
    size_bytes  BIGINT NOT NULL DEFAULT 0
)`,
		},
	},
}

func generateToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
