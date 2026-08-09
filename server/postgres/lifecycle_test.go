package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun/driver/pgdriver"

	crosstalk "github.com/aleksclark/crosstalk/server"
	"github.com/aleksclark/crosstalk/server/pgtest"
	"github.com/aleksclark/crosstalk/server/postgres"
)

func TestMigration_ConcurrentAppliesOnce(t *testing.T) {
	admin := pgtest.AdminDSN()
	dbName := "ct_mig_" + strings.ToLower(ulid.Make().String())
	adminDB := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(admin)))
	ctx := context.Background()
	_, err := adminDB.ExecContext(ctx, fmt.Sprintf(`CREATE DATABASE %q`, dbName))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = adminDB.ExecContext(context.Background(), fmt.Sprintf(`DROP DATABASE IF EXISTS %q WITH (FORCE)`, dbName))
		_ = adminDB.Close()
	})

	dsn := rewriteDB(admin, dbName)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			db, err := postgres.Open(dsn, log)
			if err != nil {
				errs[i] = err
				return
			}
			_ = db.Close()
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		require.NoError(t, err, "worker %d", i)
	}

	// Exactly one row per migration version.
	check := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	defer func() { _ = check.Close() }()
	var count int
	require.NoError(t, check.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count))
	assert.Equal(t, len(postgres.Migrations()), count)

	// Columns from lifecycle migration exist once.
	var colCount int
	require.NoError(t, check.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_name = 'sessions' AND column_name = 'owner_generation'
	`).Scan(&colCount))
	assert.Equal(t, 1, colCount)

	var ticketTable int
	require.NoError(t, check.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_name = 'media_tickets'
	`).Scan(&ticketTable))
	assert.Equal(t, 1, ticketTable)
}

func TestMigration_FailureRollsBackSchemaAndVersion(t *testing.T) {
	admin := pgtest.AdminDSN()
	dbName := "ct_migfail_" + strings.ToLower(ulid.Make().String())
	adminDB := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(admin)))
	ctx := context.Background()
	_, err := adminDB.ExecContext(ctx, fmt.Sprintf(`CREATE DATABASE %q`, dbName))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = adminDB.ExecContext(context.Background(), fmt.Sprintf(`DROP DATABASE IF EXISTS %q WITH (FORCE)`, dbName))
		_ = adminDB.Close()
	})
	dsn := rewriteDB(admin, dbName)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Apply only a stub first version, then a failing second version.
	restore := postgres.WithMigrationsForTest([]postgres.Migration{
		{
			Version: 1,
			Name:    "ok_table",
			Statements: []string{
				`CREATE TABLE IF NOT EXISTS ok_table (id TEXT PRIMARY KEY)`,
			},
		},
		{
			Version: 2,
			Name:    "boom",
			Statements: []string{
				`CREATE TABLE IF NOT EXISTS boom_table (id TEXT PRIMARY KEY)`,
				`ALTER TABLE boom_table ADD COLUMN nope TEXT REFERENCES missing_table(id)`,
			},
		},
	})
	defer restore()

	db, err := postgres.OpenWithoutMigrate(dsn, log)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	err = db.MigrateForTest(ctx)
	require.Error(t, err)

	check := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	defer func() { _ = check.Close() }()

	var v2 int
	require.NoError(t, check.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = 2`).Scan(&v2))
	assert.Equal(t, 0, v2, "failed migration must not insert version")

	var boom int
	require.NoError(t, check.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.tables WHERE table_name = 'boom_table'
	`).Scan(&boom))
	assert.Equal(t, 0, boom, "failed migration must roll back schema")

	var v1 int
	require.NoError(t, check.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = 1`).Scan(&v1))
	assert.Equal(t, 1, v1)
}

func TestSessionLifecycle_InvalidTransitionFails(t *testing.T) {
	db := pgtest.New(t)
	store := postgres.NewSessionStore(db)
	ctx := context.Background()

	sess := &crosstalk.Session{ID: ulid.Make().String(), Name: "lifecycle"}
	require.NoError(t, store.Create(ctx, sess))

	got, err := store.Get(ctx, sess.ID)
	require.NoError(t, err)
	assert.Equal(t, crosstalk.SessionWaiting, got.State)
	assert.Equal(t, uint64(0), got.OwnerGeneration)

	// waiting -> ended is illegal
	err = store.TransitionState(ctx, sess.ID, crosstalk.SessionEnded, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, crosstalk.ErrInvalidSessionTransition))

	// legal path
	require.NoError(t, store.TransitionState(ctx, sess.ID, crosstalk.SessionActive, nil))
	require.NoError(t, store.TransitionState(ctx, sess.ID, crosstalk.SessionActive, nil)) // idempotent
	require.NoError(t, store.TransitionState(ctx, sess.ID, crosstalk.SessionDraining, nil))
	require.NoError(t, store.TransitionState(ctx, sess.ID, crosstalk.SessionEnded, nil))
	require.NoError(t, store.TransitionState(ctx, sess.ID, crosstalk.SessionArchived, nil))

	got, err = store.Get(ctx, sess.ID)
	require.NoError(t, err)
	assert.Equal(t, crosstalk.SessionArchived, got.State)
	assert.NotNil(t, got.StartedAt)
	assert.NotNil(t, got.EndedAt)
}

func TestSessionLifecycle_StaleGenerationCannotPublishTerminal(t *testing.T) {
	db := pgtest.New(t)
	store := postgres.NewSessionStore(db)
	ctx := context.Background()

	sess := &crosstalk.Session{ID: ulid.Make().String(), Name: "fence"}
	require.NoError(t, store.Create(ctx, sess))
	require.NoError(t, store.TransitionState(ctx, sess.ID, crosstalk.SessionActive, nil))

	// Simulate owner generation bump (as lease acquire would).
	_, err := db.ExecContext(ctx, `UPDATE sessions SET owner_generation = 3, owner_id = 'owner-a' WHERE id = ?`, sess.ID)
	require.NoError(t, err)

	stale := uint64(1)
	err = store.TransitionState(ctx, sess.ID, crosstalk.SessionDraining, &stale)
	require.Error(t, err)
	assert.True(t, errors.Is(err, crosstalk.ErrStaleGeneration), "%v", err)

	current := uint64(3)
	require.NoError(t, store.TransitionState(ctx, sess.ID, crosstalk.SessionDraining, &current))
	require.NoError(t, store.TransitionState(ctx, sess.ID, crosstalk.SessionFailed, &current))

	got, err := store.Get(ctx, sess.ID)
	require.NoError(t, err)
	assert.Equal(t, crosstalk.SessionFailed, got.State)
}

func rewriteDB(admin, dbName string) string {
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
