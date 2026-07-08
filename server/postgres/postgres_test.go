package postgres_test

import (
	"context"
	"testing"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	crosstalk "github.com/aleksclark/crosstalk/server"
	"github.com/aleksclark/crosstalk/server/pgtest"
	"github.com/aleksclark/crosstalk/server/postgres"
)

func TestSessionStore_CRUD(t *testing.T) {
	db := pgtest.New(t)
	store := postgres.NewSessionStore(db)
	ctx := context.Background()

	sess := &crosstalk.Session{
		ID:          ulid.Make().String(),
		Name:        "Test Session",
		Description: "A test session",
	}
	err := store.Create(ctx, sess)
	require.NoError(t, err)
	assert.NotEmpty(t, sess.BroadcastToken)
	assert.False(t, sess.CreatedAt.IsZero())

	got, err := store.Get(ctx, sess.ID)
	require.NoError(t, err)
	assert.Equal(t, sess.Name, got.Name)
	assert.Equal(t, sess.Description, got.Description)

	list, err := store.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	sess.Name = "Updated Session"
	err = store.Update(ctx, sess)
	require.NoError(t, err)
	got, err = store.Get(ctx, sess.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Session", got.Name)

	got, err = store.GetByBroadcastToken(ctx, sess.BroadcastToken)
	require.NoError(t, err)
	assert.Equal(t, sess.ID, got.ID)

	newToken, err := store.RegenerateBroadcastToken(ctx, sess.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, newToken)
	assert.NotEqual(t, sess.BroadcastToken, newToken)

	err = store.Delete(ctx, sess.ID)
	require.NoError(t, err)
	_, err = store.Get(ctx, sess.ID)
	assert.Error(t, err)
}

func TestChannelStore_CRUD(t *testing.T) {
	db := pgtest.New(t)
	sessionStore := postgres.NewSessionStore(db)
	store := postgres.NewChannelStore(db)
	ctx := context.Background()

	sess := &crosstalk.Session{ID: ulid.Make().String(), Name: "Test"}
	require.NoError(t, sessionStore.Create(ctx, sess))

	ch := &crosstalk.Channel{
		ID:        ulid.Make().String(),
		SessionID: sess.ID,
		Name:      "Feed Channel",
		Type:      crosstalk.ChannelFeed,
	}
	err := store.Create(ctx, ch)
	require.NoError(t, err)

	got, err := store.Get(ctx, ch.ID)
	require.NoError(t, err)
	assert.Equal(t, ch.Name, got.Name)
	assert.Equal(t, crosstalk.ChannelFeed, got.Type)

	list, err := store.List(ctx, sess.ID)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	ch.Name = "Updated Feed"
	err = store.Update(ctx, ch)
	require.NoError(t, err)
	got, err = store.Get(ctx, ch.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Feed", got.Name)

	err = store.Delete(ctx, ch.ID)
	require.NoError(t, err)
	_, err = store.Get(ctx, ch.ID)
	assert.Error(t, err)
}

func TestSourceStore_CRUD(t *testing.T) {
	db := pgtest.New(t)
	sessionStore := postgres.NewSessionStore(db)
	store := postgres.NewSourceStore(db)
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

	list, err := store.List(ctx, sess.ID)
	require.NoError(t, err)
	assert.Len(t, list, 1)

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

	err = store.Delete(ctx, src.ID)
	require.NoError(t, err)
}

func TestMixStore(t *testing.T) {
	db := pgtest.New(t)
	sessionStore := postgres.NewSessionStore(db)
	channelStore := postgres.NewChannelStore(db)
	sourceStore := postgres.NewSourceStore(db)
	store := postgres.NewMixStore(db)
	ctx := context.Background()

	sess := &crosstalk.Session{ID: ulid.Make().String(), Name: "Test"}
	require.NoError(t, sessionStore.Create(ctx, sess))

	ch := &crosstalk.Channel{ID: ulid.Make().String(), SessionID: sess.ID, Name: "Feed", Type: crosstalk.ChannelFeed}
	require.NoError(t, channelStore.Create(ctx, ch))

	src := &crosstalk.Source{ID: ulid.Make().String(), SessionID: sess.ID, Name: "Mic 1", Origin: crosstalk.OriginABC}
	require.NoError(t, sourceStore.Create(ctx, src))

	entries := []crosstalk.MixEntry{
		{ChannelID: ch.ID, SourceID: src.ID, Muted: false, Level: 0.8},
	}
	err := store.SetMix(ctx, ch.ID, entries)
	require.NoError(t, err)

	got, err := store.GetMix(ctx, ch.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, src.ID, got[0].SourceID)
	assert.Equal(t, 0.8, got[0].Level)
	assert.False(t, got[0].Muted)

	entries[0].Muted = true
	entries[0].Level = 1.5
	err = store.SetMix(ctx, ch.ID, entries)
	require.NoError(t, err)

	got, err = store.GetMix(ctx, ch.ID)
	require.NoError(t, err)
	assert.True(t, got[0].Muted)
	assert.Equal(t, 1.5, got[0].Level)
}

// TestMixStore_SetMixRemovesAbsentEntries verifies SetMix deletes rows not in
// the new set — the fix for "removing a source reappears after refresh".
func TestMixStore_SetMixRemovesAbsentEntries(t *testing.T) {
	db := pgtest.New(t)
	sessionStore := postgres.NewSessionStore(db)
	channelStore := postgres.NewChannelStore(db)
	sourceStore := postgres.NewSourceStore(db)
	store := postgres.NewMixStore(db)
	ctx := context.Background()

	sess := &crosstalk.Session{ID: ulid.Make().String(), Name: "Test"}
	require.NoError(t, sessionStore.Create(ctx, sess))
	ch := &crosstalk.Channel{ID: ulid.Make().String(), SessionID: sess.ID, Name: "Feed", Type: crosstalk.ChannelFeed}
	require.NoError(t, channelStore.Create(ctx, ch))

	src1 := &crosstalk.Source{ID: ulid.Make().String(), SessionID: sess.ID, Name: "Mic 1", Origin: crosstalk.OriginABC}
	src2 := &crosstalk.Source{ID: ulid.Make().String(), SessionID: sess.ID, Name: "Mic 2", Origin: crosstalk.OriginABC}
	require.NoError(t, sourceStore.Create(ctx, src1))
	require.NoError(t, sourceStore.Create(ctx, src2))

	// Two sources assigned.
	require.NoError(t, store.SetMix(ctx, ch.ID, []crosstalk.MixEntry{
		{ChannelID: ch.ID, SourceID: src1.ID, Level: 1},
		{ChannelID: ch.ID, SourceID: src2.ID, Level: 1},
	}))
	got, err := store.GetMix(ctx, ch.ID)
	require.NoError(t, err)
	require.Len(t, got, 2)

	// Remove src2 by omitting it — it must not persist.
	require.NoError(t, store.SetMix(ctx, ch.ID, []crosstalk.MixEntry{
		{ChannelID: ch.ID, SourceID: src1.ID, Level: 1},
	}))
	got, err = store.GetMix(ctx, ch.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, src1.ID, got[0].SourceID)

	// Empty set clears the channel entirely.
	require.NoError(t, store.SetMix(ctx, ch.ID, nil))
	got, err = store.GetMix(ctx, ch.ID)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestABCStore_CRUD(t *testing.T) {
	db := pgtest.New(t)
	store := postgres.NewABCStore(db)
	ctx := context.Background()

	abc := &crosstalk.ABC{
		ID:        ulid.Make().String(),
		Name:      "Booth A",
		TokenHash: "abc123hash",
	}
	err := store.Create(ctx, abc)
	require.NoError(t, err)

	got, err := store.Get(ctx, abc.ID)
	require.NoError(t, err)
	assert.Equal(t, "Booth A", got.Name)

	got, err = store.GetByTokenHash(ctx, "abc123hash")
	require.NoError(t, err)
	assert.Equal(t, abc.ID, got.ID)

	list, err := store.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	abc.Name = "Booth B"
	err = store.Update(ctx, abc)
	require.NoError(t, err)
	got, err = store.Get(ctx, abc.ID)
	require.NoError(t, err)
	assert.Equal(t, "Booth B", got.Name)

	err = store.Delete(ctx, abc.ID)
	require.NoError(t, err)
	_, err = store.Get(ctx, abc.ID)
	assert.Error(t, err)
}

func TestUserStore_CRUD(t *testing.T) {
	db := pgtest.New(t)
	store := postgres.NewUserStore(db)
	sessionStore := postgres.NewSessionStore(db)
	ctx := context.Background()

	user := &crosstalk.User{
		ID:           ulid.Make().String(),
		Username:     "admin",
		PasswordHash: "$2a$10$test",
		Role:         "admin",
	}
	err := store.Create(ctx, user)
	require.NoError(t, err)

	got, err := store.Get(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, "admin", got.Username)

	got, err = store.GetByUsername(ctx, "admin")
	require.NoError(t, err)
	assert.Equal(t, user.ID, got.ID)

	list, err := store.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	list, err = store.ListByRole(ctx, "admin")
	require.NoError(t, err)
	assert.Len(t, list, 1)
	list, err = store.ListByRole(ctx, "translator")
	require.NoError(t, err)
	assert.Len(t, list, 0)

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

	user.Username = "superadmin"
	err = store.Update(ctx, user)
	require.NoError(t, err)

	err = store.Delete(ctx, user.ID)
	require.NoError(t, err)
}

func TestRefreshTokenStore(t *testing.T) {
	db := pgtest.New(t)
	userStore := postgres.NewUserStore(db)
	store := postgres.NewRefreshTokenStore(db)
	ctx := context.Background()

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

	got, err := store.GetByHash(ctx, "somehash123")
	require.NoError(t, err)
	assert.Equal(t, rt.ID, got.ID)
	assert.Equal(t, user.ID, got.UserID)

	err = store.DeleteByHash(ctx, "somehash123")
	require.NoError(t, err)
	_, err = store.GetByHash(ctx, "somehash123")
	assert.Error(t, err)

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
