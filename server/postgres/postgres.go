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

// migrationAdvisoryLockID is a stable 64-bit key used with pg_advisory_lock so
// concurrent Open() callers serialize schema changes.
const migrationAdvisoryLockID int64 = 0x43544d49475201 // "CTMIGR\x01"

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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := bdb.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("postgres ping: %w", err)
	}

	d := &DB{DB: bdb, log: log}
	if err := d.migrate(ctx); err != nil {
		_ = bdb.Close()
		return nil, fmt.Errorf("postgres migrate: %w", err)
	}

	return d, nil
}

// migrate runs all schema migrations under a session advisory lock.
// Each migration's statements and version insert run in ONE transaction so a
// statement failure rolls back both schema changes and the version row.
func (d *DB) migrate(ctx context.Context) error {
	conn, err := d.Conn(ctx)
	if err != nil {
		return fmt.Errorf("migration conn: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock(?)`, migrationAdvisoryLockID); err != nil {
		return fmt.Errorf("advisory lock: %w", err)
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock(?)`, migrationAdvisoryLockID)
	}()

	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return err
	}

	for _, m := range migrations {
		var exists int
		if err := conn.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM schema_migrations WHERE version = ?", m.version).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			continue
		}

		if d.log != nil {
			d.log.Info("applying migration", "version", m.version, "name", m.name)
		}

		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("migration %d begin: %w", m.version, err)
		}

		for _, stmt := range m.statements {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("migration %d (%s): %w", m.version, m.name, err)
			}
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO schema_migrations (version) VALUES (?)", m.version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %d version insert: %w", m.version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("migration %d commit: %w", m.version, err)
		}
	}

	return nil
}

// MigrateForTest exposes migrate for package tests (same package via export
// test helpers live in postgres_test which is external — use Open instead).
// This is used by white-box tests in package postgres.
func (d *DB) MigrateForTest(ctx context.Context) error {
	return d.migrate(ctx)
}

// WithMigrationsForTest temporarily replaces the migration set for tests.
// The returned restore function must be called.
func WithMigrationsForTest(ms []Migration) (restore func()) {
	prev := migrations
	converted := make([]migration, len(ms))
	for i, m := range ms {
		converted[i] = migration{version: m.Version, name: m.Name, statements: m.Statements}
	}
	migrations = converted
	return func() { migrations = prev }
}

// Migration is the exported migration descriptor used by tests.
type Migration struct {
	Version    int
	Name       string
	Statements []string
}

// Migrations returns a copy of the built-in migration list for inspection.
func Migrations() []Migration {
	out := make([]Migration, len(migrations))
	for i, m := range migrations {
		out[i] = Migration{Version: m.version, Name: m.name, Statements: append([]string(nil), m.statements...)}
	}
	return out
}

// OpenWithoutMigrate opens a DB without running migrations (test helper).
func OpenWithoutMigrate(dsn string, log *slog.Logger) (*DB, error) {
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	bdb := bun.NewDB(sqldb, pgdialect.New())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := bdb.PingContext(ctx); err != nil {
		_ = bdb.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}
	return &DB{DB: bdb, log: log}, nil
}

type migration struct {
	version    int
	name       string
	statements []string
}

// migrations holds the ordered schema definition. Each statement is executed
// individually because the pgdriver protocol does not allow multiple
// statements in a single Exec. Statements for one version run inside a single
// transaction together with the version insert.
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
	{
		version: 2,
		name:    "abc_monitor_channel",
		statements: []string{
			`ALTER TABLE abcs ADD COLUMN IF NOT EXISTS monitor_channel_id TEXT REFERENCES channels(id) ON DELETE SET NULL`,
		},
	},
	{
		version: 3,
		name:    "session_lifecycle_and_media_tickets",
		statements: []string{
			`ALTER TABLE sessions ADD COLUMN IF NOT EXISTS state TEXT NOT NULL DEFAULT 'waiting'`,
			`ALTER TABLE sessions ADD COLUMN IF NOT EXISTS owner_id TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE sessions ADD COLUMN IF NOT EXISTS owner_generation BIGINT NOT NULL DEFAULT 0`,
			`ALTER TABLE sessions ADD COLUMN IF NOT EXISTS lease_until TIMESTAMPTZ`,
			`ALTER TABLE sessions ADD COLUMN IF NOT EXISTS started_at TIMESTAMPTZ`,
			`ALTER TABLE sessions ADD COLUMN IF NOT EXISTS ended_at TIMESTAMPTZ`,
			// CHECK constraint for legal states (additive; existing rows get waiting).
			`DO $$ BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'sessions_state_check'
  ) THEN
    ALTER TABLE sessions ADD CONSTRAINT sessions_state_check
      CHECK (state IN ('waiting','active','draining','ended','archived','failed'));
  END IF;
END $$`,
			`CREATE TABLE IF NOT EXISTS media_tickets (
    id                   TEXT PRIMARY KEY,
    nonce_hash           TEXT NOT NULL UNIQUE,
    session_id           TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    owner_id             TEXT NOT NULL,
    owner_generation     BIGINT NOT NULL,
    subject              TEXT NOT NULL,
    role                 TEXT NOT NULL,
    produce_channel_ids  TEXT[] NOT NULL DEFAULT '{}',
    listen_channel_ids   TEXT[] NOT NULL DEFAULT '{}',
    expires_at           TIMESTAMPTZ NOT NULL,
    consumed_at          TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
)`,
			`CREATE INDEX IF NOT EXISTS media_tickets_session_id_idx ON media_tickets (session_id)`,
			`CREATE INDEX IF NOT EXISTS media_tickets_expires_at_idx ON media_tickets (expires_at)`,
		},
	},
	{
		version: 4,
		name:    "abc_audio_control",
		statements: []string{
			`CREATE TABLE abc_audio_settings (
    abc_id TEXT PRIMARY KEY REFERENCES abcs(id) ON DELETE CASCADE,
    desired_revision BIGINT NOT NULL DEFAULT 0 CHECK (desired_revision >= 0),
    desired_output_device_uid TEXT,
    desired_output_volume_percent SMALLINT CHECK (desired_output_volume_percent BETWEEN 0 AND 100),
    desired_output_muted BOOLEAN,
    desired_input_device_uid TEXT,
    desired_input_gain_percent SMALLINT CHECK (desired_input_gain_percent BETWEEN 0 AND 100),
    command_id TEXT,
    reported_revision BIGINT NOT NULL DEFAULT 0 CHECK (reported_revision >= 0),
    reported_command_id TEXT,
    reported_output_device_uid TEXT,
    observed_output_volume_percent SMALLINT CHECK (observed_output_volume_percent BETWEEN 0 AND 100),
    observed_output_muted BOOLEAN,
    reported_input_device_uid TEXT,
    observed_input_gain_percent SMALLINT CHECK (observed_input_gain_percent BETWEEN 0 AND 100),
    output_volume_state TEXT NOT NULL DEFAULT 'unknown'
      CHECK (output_volume_state IN ('unknown','pending','applied','unsupported','error','device_mismatch')),
    output_mute_state TEXT NOT NULL DEFAULT 'unknown'
      CHECK (output_mute_state IN ('unknown','pending','applied','unsupported','error','device_mismatch')),
    input_gain_state TEXT NOT NULL DEFAULT 'unknown'
      CHECK (input_gain_state IN ('unknown','pending','applied','unsupported','error','device_mismatch')),
    error_code TEXT NOT NULL DEFAULT '',
    error_detail TEXT NOT NULL DEFAULT '',
    capabilities JSONB NOT NULL DEFAULT '{}'::jsonb,
    reported_at TIMESTAMPTZ,
    desired_updated_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((desired_revision = 0) = (desired_output_device_uid IS NULL)),
    CHECK ((desired_revision = 0) = (desired_input_device_uid IS NULL)),
    CHECK ((desired_revision = 0) = (desired_output_volume_percent IS NULL)),
    CHECK ((desired_revision = 0) = (desired_output_muted IS NULL)),
    CHECK ((desired_revision = 0) = (desired_input_gain_percent IS NULL))
)`,
			`CREATE TABLE abc_audio_audit_events (
    id TEXT PRIMARY KEY,
    abc_id TEXT NOT NULL REFERENCES abcs(id) ON DELETE CASCADE,
    request_id TEXT NOT NULL,
    actor_user_id TEXT NOT NULL,
    actor_role TEXT NOT NULL,
    desired_revision BIGINT NOT NULL,
    previous_desired JSONB NOT NULL,
    new_desired JSONB NOT NULL,
    outcome TEXT NOT NULL CHECK (outcome IN ('accepted','no_op')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (abc_id, request_id)
)`,
			`CREATE INDEX abc_audio_audit_events_abc_created_idx
    ON abc_audio_audit_events (abc_id, created_at DESC)`,
		},
	},
}

func generateToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
