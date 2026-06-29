package sqlite_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	crosstalk "github.com/nicosql/crosstalk/server"
	"github.com/nicosql/crosstalk/server/sqlite"
)

func testDB(t *testing.T) *sqlite.DB {
	t.Helper()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	db, err := sqlite.Open(":memory:", log)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestSessionStore_CRUD(t *testing.T) {
	db := testDB(t)
	store := sqlite.NewSessionStore(db)
	ctx := context.Background()

	// Create
	sess := &crosstalk.Session{
		ID:          ulid.Make().String(),
		Name:        "Test Session",
		Description: "A test session",
	}
	err := store.Create(ctx, sess)
	require.NoError(t, err)
	assert.NotEmpty(t, sess.BroadcastToken)
	assert.False(t, sess.CreatedAt.IsZero())

	// Get
	got, err := store.Get(ctx, sess.ID)
	require.NoError(t, err)
	assert.Equal(t, sess.Name, got.Name)
	assert.Equal(t, sess.Description, got.Description)

	// List
	list, err := store.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	// Update
	sess.Name = "Updated Session"
	err = store.Update(ctx, sess)
	require.NoError(t, err)
	got, err = store.Get(ctx, sess.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Session", got.Name)

	// GetByBroadcastToken
	got, err = store.GetByBroadcastToken(ctx, sess.BroadcastToken)
	require.NoError(t, err)
	assert.Equal(t, sess.ID, got.ID)

	// RegenerateBroadcastToken
	newToken, err := store.RegenerateBroadcastToken(ctx, sess.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, newToken)
	assert.NotEqual(t, sess.BroadcastToken, newToken)

	// Delete
	err = store.Delete(ctx, sess.ID)
	require.NoError(t, err)
	_, err = store.Get(ctx, sess.ID)
	assert.Error(t, err)
}

func TestChannelStore_CRUD(t *testing.T) {
	db := testDB(t)
	sessionStore := sqlite.NewSessionStore(db)
	store := sqlite.NewChannelStore(db)
	ctx := context.Background()

	// Create session first
	sess := &crosstalk.Session{ID: ulid.Make().String(), Name: "Test"}
	require.NoError(t, sessionStore.Create(ctx, sess))

	// Create channel
	ch := &crosstalk.Channel{
		ID:        ulid.Make().String(),
		SessionID: sess.ID,
		Name:      "Feed Channel",
		Type:      crosstalk.ChannelFeed,
	}
	err := store.Create(ctx, ch)
	require.NoError(t, err)

	// Get
	got, err := store.Get(ctx, ch.ID)
	require.NoError(t, err)
	assert.Equal(t, ch.Name, got.Name)
	assert.Equal(t, crosstalk.ChannelFeed, got.Type)

	// List
	list, err := store.List(ctx, sess.ID)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	// Update
	ch.Name = "Updated Feed"
	err = store.Update(ctx, ch)
	require.NoError(t, err)
	got, err = store.Get(ctx, ch.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Feed", got.Name)

	// Delete
	err = store.Delete(ctx, ch.ID)
	require.NoError(t, err)
	_, err = store.Get(ctx, ch.ID)
	assert.Error(t, err)
}

func TestSourceStore_CRUD(t *testing.T) {
	db := testDB(t)
	sessionStore := sqlite.NewSessionStore(db)
	store := sqlite.NewSourceStore(db)
	ctx := context.Background()

	sess := &crosstalk.Session{ID: ulid.Make().String(), Name: "Test"}
	require.NoError(t, sessionStore.Create(ctx, sess))

	src := &crosstalk.Source{
		ID:        ulid.Make().String(),
		SessionID: sess.ID,
		Name:      "Booth A mic",
		Origin:    crosstalk.OriginABC,
		Connected: true,
	}
	err := store.Create(ctx, src)
	require.NoError(t, err)

	got, err := store.Get(ctx, src.ID)
	require.NoError(t, err)
	assert.Equal(t, src.Name, got.Name)
	assert.Equal(t, crosstalk.OriginABC, got.Origin)
	assert.True(t, got.Connected)

	// List
	list, err := store.List(ctx, sess.ID)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	// Update (disconnect)
	src.Connected = false
	peerID := "peer-123"
	src.PeerID = &peerID
	err = store.Update(ctx, src)
	require.NoError(t, err)
	got, err = store.Get(ctx, src.ID)
	require.NoError(t, err)
	assert.False(t, got.Connected)
	assert.NotNil(t, got.PeerID)
	assert.Equal(t, "peer-123", *got.PeerID)

	// Delete
	err = store.Delete(ctx, src.ID)
	require.NoError(t, err)
}

func TestMixStore(t *testing.T) {
	db := testDB(t)
	sessionStore := sqlite.NewSessionStore(db)
	channelStore := sqlite.NewChannelStore(db)
	sourceStore := sqlite.NewSourceStore(db)
	store := sqlite.NewMixStore(db)
	ctx := context.Background()

	sess := &crosstalk.Session{ID: ulid.Make().String(), Name: "Test"}
	require.NoError(t, sessionStore.Create(ctx, sess))

	ch := &crosstalk.Channel{ID: ulid.Make().String(), SessionID: sess.ID, Name: "Feed", Type: crosstalk.ChannelFeed}
	require.NoError(t, channelStore.Create(ctx, ch))

	src := &crosstalk.Source{ID: ulid.Make().String(), SessionID: sess.ID, Name: "Mic 1", Origin: crosstalk.OriginABC}
	require.NoError(t, sourceStore.Create(ctx, src))

	// Set mix
	entries := []crosstalk.MixEntry{
		{ChannelID: ch.ID, SourceID: src.ID, Muted: false, Level: 0.8},
	}
	err := store.SetMix(ctx, ch.ID, entries)
	require.NoError(t, err)

	// Get mix
	got, err := store.GetMix(ctx, ch.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, src.ID, got[0].SourceID)
	assert.Equal(t, 0.8, got[0].Level)
	assert.False(t, got[0].Muted)

	// Update mix (mute)
	entries[0].Muted = true
	entries[0].Level = 1.5
	err = store.SetMix(ctx, ch.ID, entries)
	require.NoError(t, err)

	got, err = store.GetMix(ctx, ch.ID)
	require.NoError(t, err)
	assert.True(t, got[0].Muted)
	assert.Equal(t, 1.5, got[0].Level)
}

func TestABCStore_CRUD(t *testing.T) {
	db := testDB(t)
	store := sqlite.NewABCStore(db)
	ctx := context.Background()

	abc := &crosstalk.ABC{
		ID:        ulid.Make().String(),
		Name:      "Booth A",
		TokenHash: "abc123hash",
	}
	err := store.Create(ctx, abc)
	require.NoError(t, err)

	// Get
	got, err := store.Get(ctx, abc.ID)
	require.NoError(t, err)
	assert.Equal(t, "Booth A", got.Name)

	// GetByTokenHash
	got, err = store.GetByTokenHash(ctx, "abc123hash")
	require.NoError(t, err)
	assert.Equal(t, abc.ID, got.ID)

	// List
	list, err := store.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	// Update
	abc.Name = "Booth B"
	err = store.Update(ctx, abc)
	require.NoError(t, err)
	got, err = store.Get(ctx, abc.ID)
	require.NoError(t, err)
	assert.Equal(t, "Booth B", got.Name)

	// Delete
	err = store.Delete(ctx, abc.ID)
	require.NoError(t, err)
	_, err = store.Get(ctx, abc.ID)
	assert.Error(t, err)
}

func TestUserStore_CRUD(t *testing.T) {
	db := testDB(t)
	store := sqlite.NewUserStore(db)
	sessionStore := sqlite.NewSessionStore(db)
	ctx := context.Background()

	user := &crosstalk.User{
		ID:           ulid.Make().String(),
		Username:     "admin",
		PasswordHash: "$2a$10$test",
		Role:         "admin",
	}
	err := store.Create(ctx, user)
	require.NoError(t, err)

	// Get
	got, err := store.Get(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, "admin", got.Username)

	// GetByUsername
	got, err = store.GetByUsername(ctx, "admin")
	require.NoError(t, err)
	assert.Equal(t, user.ID, got.ID)

	// List
	list, err := store.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	// ListByRole
	list, err = store.ListByRole(ctx, "admin")
	require.NoError(t, err)
	assert.Len(t, list, 1)
	list, err = store.ListByRole(ctx, "translator")
	require.NoError(t, err)
	assert.Len(t, list, 0)

	// AssignSessions
	sess := &crosstalk.Session{ID: ulid.Make().String(), Name: "Test"}
	require.NoError(t, sessionStore.Create(ctx, sess))

	translator := &crosstalk.User{
		ID:           ulid.Make().String(),
		Username:     "translator1",
		PasswordHash: "$2a$10$test",
		Role:         "translator",
	}
	require.NoError(t, store.Create(ctx, translator))

	err = store.AssignSessions(ctx, translator.ID, []string{sess.ID})
	require.NoError(t, err)

	sessions, err := store.GetAssignedSessions(ctx, translator.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{sess.ID}, sessions)

	// Update
	user.Username = "superadmin"
	err = store.Update(ctx, user)
	require.NoError(t, err)

	// Delete
	err = store.Delete(ctx, user.ID)
	require.NoError(t, err)
}

func TestRefreshTokenStore(t *testing.T) {
	db := testDB(t)
	userStore := sqlite.NewUserStore(db)
	store := sqlite.NewRefreshTokenStore(db)
	ctx := context.Background()

	// Create a user first
	user := &crosstalk.User{
		ID:           ulid.Make().String(),
		Username:     "testuser",
		PasswordHash: "$2a$10$test",
		Role:         "admin",
	}
	require.NoError(t, userStore.Create(ctx, user))

	rt := &crosstalk.RefreshToken{
		ID:        ulid.Make().String(),
		UserID:    user.ID,
		TokenHash: "somehash123",
	}
	err := store.Create(ctx, rt)
	require.NoError(t, err)

	// GetByHash
	got, err := store.GetByHash(ctx, "somehash123")
	require.NoError(t, err)
	assert.Equal(t, rt.ID, got.ID)
	assert.Equal(t, user.ID, got.UserID)

	// DeleteByHash
	err = store.DeleteByHash(ctx, "somehash123")
	require.NoError(t, err)
	_, err = store.GetByHash(ctx, "somehash123")
	assert.Error(t, err)

	// DeleteByUserID
	rt2 := &crosstalk.RefreshToken{
		ID:        ulid.Make().String(),
		UserID:    user.ID,
		TokenHash: "anotherhash",
	}
	require.NoError(t, store.Create(ctx, rt2))
	err = store.DeleteByUserID(ctx, user.ID)
	require.NoError(t, err)
	_, err = store.GetByHash(ctx, "anotherhash")
	assert.Error(t, err)
}
