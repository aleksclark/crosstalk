package auth_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	crosstalk "github.com/nicosql/crosstalk/server"
	"github.com/nicosql/crosstalk/server/auth"
	"github.com/nicosql/crosstalk/server/sqlite"
)

func setupAuth(t *testing.T) (*auth.Service, crosstalk.UserService) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	db, err := sqlite.Open(":memory:", log)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	users := sqlite.NewUserStore(db)
	refreshTokens := sqlite.NewRefreshTokenStore(db)

	cfg := auth.Config{
		Secret:          "test-secret",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
	}

	svc := auth.NewService(cfg, users, refreshTokens)
	return svc, users
}

func TestLogin(t *testing.T) {
	svc, users := setupAuth(t)
	ctx := context.Background()

	// Create user
	hash, err := auth.HashPassword("password123")
	require.NoError(t, err)

	user := &crosstalk.User{
		ID:           ulid.Make().String(),
		Username:     "admin",
		PasswordHash: hash,
		Role:         "admin",
	}
	require.NoError(t, users.Create(ctx, user))

	// Login success
	pair, err := svc.Login(ctx, "admin", "password123")
	require.NoError(t, err)
	assert.NotEmpty(t, pair.AccessToken)
	assert.NotEmpty(t, pair.RefreshToken)

	// Validate access token
	claims, err := svc.ValidateAccessToken(pair.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, user.ID, claims.Subject)
	assert.Equal(t, "admin", claims.Role)

	// Login failure - wrong password
	_, err = svc.Login(ctx, "admin", "wrong")
	assert.Error(t, err)

	// Login failure - wrong username
	_, err = svc.Login(ctx, "nobody", "password123")
	assert.Error(t, err)
}

func TestRefresh(t *testing.T) {
	svc, users := setupAuth(t)
	ctx := context.Background()

	hash, err := auth.HashPassword("password123")
	require.NoError(t, err)

	user := &crosstalk.User{
		ID:           ulid.Make().String(),
		Username:     "admin",
		PasswordHash: hash,
		Role:         "admin",
	}
	require.NoError(t, users.Create(ctx, user))

	// Login
	pair, err := svc.Login(ctx, "admin", "password123")
	require.NoError(t, err)

	// Refresh
	newPair, err := svc.Refresh(ctx, pair.RefreshToken)
	require.NoError(t, err)
	assert.NotEmpty(t, newPair.AccessToken)
	assert.NotEmpty(t, newPair.RefreshToken)
	// Refresh token must always differ (ULID-based)
	assert.NotEqual(t, pair.RefreshToken, newPair.RefreshToken)

	// Old refresh token is invalid (rotation)
	_, err = svc.Refresh(ctx, pair.RefreshToken)
	assert.Error(t, err)
}

func TestLogout(t *testing.T) {
	svc, users := setupAuth(t)
	ctx := context.Background()

	hash, err := auth.HashPassword("password123")
	require.NoError(t, err)

	user := &crosstalk.User{
		ID:           ulid.Make().String(),
		Username:     "admin",
		PasswordHash: hash,
		Role:         "admin",
	}
	require.NoError(t, users.Create(ctx, user))

	pair, err := svc.Login(ctx, "admin", "password123")
	require.NoError(t, err)

	// Logout
	err = svc.Logout(ctx, pair.RefreshToken)
	require.NoError(t, err)

	// Refresh should fail
	_, err = svc.Refresh(ctx, pair.RefreshToken)
	assert.Error(t, err)
}

func TestValidateAccessToken_Invalid(t *testing.T) {
	svc, _ := setupAuth(t)

	_, err := svc.ValidateAccessToken("not-a-valid-token")
	assert.Error(t, err)

	_, err = svc.ValidateAccessToken("")
	assert.Error(t, err)
}

func TestHashToken(t *testing.T) {
	hash1 := auth.HashToken("test-token")
	hash2 := auth.HashToken("test-token")
	assert.Equal(t, hash1, hash2)

	hash3 := auth.HashToken("different-token")
	assert.NotEqual(t, hash1, hash3)
}
