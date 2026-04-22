package sqlite_test

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	crosstalk "github.com/anthropics/crosstalk/server"
	"github.com/anthropics/crosstalk/server/sqlite"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestDB(t *testing.T) *sqlite.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sqlite.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestUserService_CreateAndFindByID(t *testing.T) {
	db := openTestDB(t)
	svc := &sqlite.UserService{DB: db.DB}

	now := time.Now().UTC().Truncate(time.Second)
	user := &crosstalk.User{
		ID:           ulid.Make().String(),
		Username:     "alice",
		PasswordHash: "hash123",
		CreatedAt:    now,
	}

	require.NoError(t, svc.CreateUser(user))

	got, err := svc.FindUserByID(user.ID)
	require.NoError(t, err)
	assert.Equal(t, user.ID, got.ID)
	assert.Equal(t, user.Username, got.Username)
	assert.Equal(t, user.PasswordHash, got.PasswordHash)
	assert.Equal(t, now, got.CreatedAt)
}

func TestUserService_FindByUsername(t *testing.T) {
	db := openTestDB(t)
	svc := &sqlite.UserService{DB: db.DB}

	now := time.Now().UTC().Truncate(time.Second)
	user := &crosstalk.User{
		ID:           ulid.Make().String(),
		Username:     "bob",
		PasswordHash: "hash456",
		CreatedAt:    now,
	}

	require.NoError(t, svc.CreateUser(user))

	got, err := svc.FindUserByUsername("bob")
	require.NoError(t, err)
	assert.Equal(t, user.ID, got.ID)
	assert.Equal(t, "bob", got.Username)
}

func TestUserService_FindByUsername_NotFound(t *testing.T) {
	db := openTestDB(t)
	svc := &sqlite.UserService{DB: db.DB}

	_, err := svc.FindUserByUsername("nonexistent")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestUserService_FindByID_NotFound(t *testing.T) {
	db := openTestDB(t)
	svc := &sqlite.UserService{DB: db.DB}

	_, err := svc.FindUserByID("nonexistent")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestUserService_Delete(t *testing.T) {
	db := openTestDB(t)
	svc := &sqlite.UserService{DB: db.DB}

	now := time.Now().UTC().Truncate(time.Second)
	user := &crosstalk.User{
		ID:           ulid.Make().String(),
		Username:     "charlie",
		PasswordHash: "hash789",
		CreatedAt:    now,
	}

	require.NoError(t, svc.CreateUser(user))
	require.NoError(t, svc.DeleteUser(user.ID))

	_, err := svc.FindUserByID(user.ID)
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestUserService_Delete_NotFound(t *testing.T) {
	db := openTestDB(t)
	svc := &sqlite.UserService{DB: db.DB}

	err := svc.DeleteUser("nonexistent")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}
