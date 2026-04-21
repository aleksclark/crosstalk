// Package sqlite implements domain service interfaces backed by SQLite.
// This package wraps the database/sql and goose dependencies.
package sqlite

import (
	"database/sql"
	"embed"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrations embed.FS

// DB wraps a *sql.DB connection to a SQLite database.
type DB struct {
	*sql.DB
}

// Open opens a SQLite database at the given path, enables WAL mode,
// enables foreign keys, and runs all pending goose migrations.
func Open(dbPath string) (*DB, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}

	goose.SetBaseFS(migrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite set dialect: %w", err)
	}
	if err := goose.Up(db, "migrations"); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite migrate: %w", err)
	}

	return &DB{DB: db}, nil
}

// Close closes the underlying database connection.
func (db *DB) Close() error {
	return db.DB.Close()
}
