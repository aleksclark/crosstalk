package sqlite_test

import (
	"database/sql"
	"testing"
	"time"

	crosstalk "github.com/anthropics/crosstalk/server"
	"github.com/anthropics/crosstalk/server/sqlite"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenService_CreateAndFindByHash(t *testing.T) {
	db := openTestDB(t)
	userSvc := &sqlite.UserService{DB: db.DB}
	tokenSvc := &sqlite.TokenService{DB: db.DB}

	now := time.Now().UTC().Truncate(time.Second)
	user := &crosstalk.User{
		ID:           ulid.Make().String(),
		Username:     "alice",
		PasswordHash: "hash123",
		CreatedAt:    now,
	}
	require.NoError(t, userSvc.CreateUser(user))

	token := &crosstalk.APIToken{
		ID:        ulid.Make().String(),
		Name:      "my-token",
		TokenHash: "tokenhash123",
		UserID:    user.ID,
		CreatedAt: now,
	}
	require.NoError(t, tokenSvc.CreateToken(token))

	got, err := tokenSvc.FindTokenByHash("tokenhash123")
	require.NoError(t, err)
	assert.Equal(t, token.ID, got.ID)
	assert.Equal(t, token.Name, got.Name)
	assert.Equal(t, token.TokenHash, got.TokenHash)
	assert.Equal(t, token.UserID, got.UserID)
	assert.Equal(t, now, got.CreatedAt)
}

func TestTokenService_FindByHash_NotFound(t *testing.T) {
	db := openTestDB(t)
	tokenSvc := &sqlite.TokenService{DB: db.DB}

	_, err := tokenSvc.FindTokenByHash("nonexistent")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestTokenService_Delete(t *testing.T) {
	db := openTestDB(t)
	userSvc := &sqlite.UserService{DB: db.DB}
	tokenSvc := &sqlite.TokenService{DB: db.DB}

	now := time.Now().UTC().Truncate(time.Second)
	user := &crosstalk.User{
		ID:           ulid.Make().String(),
		Username:     "bob",
		PasswordHash: "hash456",
		CreatedAt:    now,
	}
	require.NoError(t, userSvc.CreateUser(user))

	token := &crosstalk.APIToken{
		ID:        ulid.Make().String(),
		Name:      "deleteme",
		TokenHash: "tokenhash456",
		UserID:    user.ID,
		CreatedAt: now,
	}
	require.NoError(t, tokenSvc.CreateToken(token))
	require.NoError(t, tokenSvc.DeleteToken(token.ID))

	_, err := tokenSvc.FindTokenByHash("tokenhash456")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestTokenService_Delete_NotFound(t *testing.T) {
	db := openTestDB(t)
	tokenSvc := &sqlite.TokenService{DB: db.DB}

	err := tokenSvc.DeleteToken("nonexistent")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestTokenService_ListTokens(t *testing.T) {
	db := openTestDB(t)
	userSvc := &sqlite.UserService{DB: db.DB}
	tokenSvc := &sqlite.TokenService{DB: db.DB}

	now := time.Now().UTC().Truncate(time.Second)
	user := &crosstalk.User{
		ID:           ulid.Make().String(),
		Username:     "charlie",
		PasswordHash: "hash789",
		CreatedAt:    now,
	}
	require.NoError(t, userSvc.CreateUser(user))

	t1 := &crosstalk.APIToken{
		ID:        ulid.Make().String(),
		Name:      "token-1",
		TokenHash: "hash-a",
		UserID:    user.ID,
		CreatedAt: now,
	}
	t2 := &crosstalk.APIToken{
		ID:        ulid.Make().String(),
		Name:      "token-2",
		TokenHash: "hash-b",
		UserID:    user.ID,
		CreatedAt: now.Add(time.Second),
	}
	require.NoError(t, tokenSvc.CreateToken(t1))
	require.NoError(t, tokenSvc.CreateToken(t2))

	tokens, err := tokenSvc.ListTokens()
	require.NoError(t, err)
	require.Len(t, tokens, 2)
	assert.Equal(t, t1.ID, tokens[0].ID)
	assert.Equal(t, t2.ID, tokens[1].ID)
}

func TestTokenService_ListTokens_Empty(t *testing.T) {
	db := openTestDB(t)
	tokenSvc := &sqlite.TokenService{DB: db.DB}

	tokens, err := tokenSvc.ListTokens()
	require.NoError(t, err)
	assert.Empty(t, tokens)
}
