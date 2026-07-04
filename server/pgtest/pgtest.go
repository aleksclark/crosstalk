// Package pgtest provides ephemeral PostgreSQL databases for tests.
//
// Each call to New creates a uniquely named database, runs the CrossTalk
// migrations against it via the postgres package, and drops it on cleanup.
// This keeps every test fully isolated while exercising the real Postgres +
// Bun stack (no mocks, no in-memory fakes).
package pgtest

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/uptrace/bun/driver/pgdriver"

	"github.com/aleksclark/crosstalk/server/postgres"
)

// AdminDSN returns the maintenance DSN used to create/drop test databases.
// Override with CT_TEST_DATABASE_URL.
func AdminDSN() string {
	if v := os.Getenv("CT_TEST_DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
}

func dsnForDB(admin, dbName string) string {
	// Replace the path segment (database name) in the admin DSN.
	if i := strings.LastIndex(admin, "/"); i >= 0 {
		rest := admin[i+1:]
		q := ""
		if j := strings.Index(rest, "?"); j >= 0 {
			q = rest[j:]
		}
		return admin[:i+1] + dbName + q
	}
	return admin
}

// New provisions a fresh database and returns an opened *postgres.DB.
func New(t testing.TB) *postgres.DB {
	t.Helper()

	admin := AdminDSN()
	dbName := "ct_test_" + strings.ToLower(ulid.Make().String())

	adminDB := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(admin)))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := adminDB.ExecContext(ctx, fmt.Sprintf(`CREATE DATABASE %q`, dbName)); err != nil {
		_ = adminDB.Close()
		t.Fatalf("pgtest: create database %s: %v (is Postgres running at %s?)", dbName, err, admin)
	}
	_ = adminDB.Close()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	db, err := postgres.Open(dsnForDB(admin, dbName), log)
	if err != nil {
		dropDatabase(admin, dbName)
		t.Fatalf("pgtest: open %s: %v", dbName, err)
	}

	t.Cleanup(func() {
		_ = db.Close()
		dropDatabase(admin, dbName)
	})

	return db
}

func dropDatabase(admin, dbName string) {
	adminDB := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(admin)))
	defer func() { _ = adminDB.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _ = adminDB.ExecContext(ctx, fmt.Sprintf(`DROP DATABASE IF EXISTS %q WITH (FORCE)`, dbName))
}
