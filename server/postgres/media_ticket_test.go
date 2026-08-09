package postgres_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	crosstalk "github.com/aleksclark/crosstalk/server"
	"github.com/aleksclark/crosstalk/server/pgtest"
	"github.com/aleksclark/crosstalk/server/postgres"
)

func TestMediaTicketStore_ConsumeReplayExpiryGeneration(t *testing.T) {
	db := pgtest.New(t)
	sessions := postgres.NewSessionStore(db)
	tickets := postgres.NewMediaTicketStore(db)
	ctx := context.Background()

	sess := &crosstalk.Session{ID: ulid.Make().String(), Name: "tickets"}
	require.NoError(t, sessions.Create(ctx, sess))
	_, err := db.ExecContext(ctx, `UPDATE sessions SET owner_generation = 5, owner_id = 'o1' WHERE id = ?`, sess.ID)
	require.NoError(t, err)

	nonce := "nonce-" + ulid.Make().String()
	ticket := &crosstalk.MediaTicket{
		SessionID:         sess.ID,
		OwnerID:           "o1",
		OwnerGeneration:   5,
		Subject:           "device-1",
		Role:              "translator",
		ProduceChannelIDs: []string{"ch-produce"},
		ListenChannelIDs:  []string{"ch-listen"},
		ExpiresAt:         time.Now().UTC().Add(30 * time.Second),
	}
	require.NoError(t, tickets.Issue(ctx, ticket, nonce))
	assert.Equal(t, postgres.HashNonce(nonce), ticket.NonceHash)

	// Wrong generation fails.
	_, err = tickets.Consume(ctx, nonce, 4)
	require.Error(t, err)
	assert.True(t, errors.Is(err, crosstalk.ErrStaleGeneration), "%v", err)

	// Happy path.
	got, err := tickets.Consume(ctx, nonce, 5)
	require.NoError(t, err)
	require.NotNil(t, got.ConsumedAt)
	assert.Equal(t, []string{"ch-produce"}, got.ProduceChannelIDs)

	// Replay fails.
	_, err = tickets.Consume(ctx, nonce, 5)
	require.Error(t, err)
	assert.True(t, errors.Is(err, crosstalk.ErrTicketConsumed), "%v", err)

	// Expired ticket fails.
	expNonce := "exp-" + ulid.Make().String()
	expTicket := &crosstalk.MediaTicket{
		SessionID:       sess.ID,
		OwnerID:         "o1",
		OwnerGeneration: 5,
		Subject:         "device-2",
		Role:            "translator",
		ExpiresAt:       time.Now().UTC().Add(-time.Second),
	}
	require.NoError(t, tickets.Issue(ctx, expTicket, expNonce))
	// Force expiry in DB in case clock skew.
	_, err = db.ExecContext(ctx, `UPDATE media_tickets SET expires_at = now() - interval '1 second' WHERE nonce_hash = ?`, postgres.HashNonce(expNonce))
	require.NoError(t, err)
	_, err = tickets.Consume(ctx, expNonce, 5)
	require.Error(t, err)
	assert.True(t, errors.Is(err, crosstalk.ErrTicketExpired), "%v", err)
}

func TestMediaTicketStore_ConcurrentConsumeOnce(t *testing.T) {
	db := pgtest.New(t)
	sessions := postgres.NewSessionStore(db)
	tickets := postgres.NewMediaTicketStore(db)
	ctx := context.Background()

	sess := &crosstalk.Session{ID: ulid.Make().String(), Name: "conc"}
	require.NoError(t, sessions.Create(ctx, sess))
	_, err := db.ExecContext(ctx, `UPDATE sessions SET owner_generation = 1, owner_id = 'o' WHERE id = ?`, sess.ID)
	require.NoError(t, err)

	nonce := "c-" + ulid.Make().String()
	require.NoError(t, tickets.Issue(ctx, &crosstalk.MediaTicket{
		SessionID:       sess.ID,
		OwnerID:         "o",
		OwnerGeneration: 1,
		Subject:         "s",
		Role:            "admin",
		ExpiresAt:       time.Now().UTC().Add(time.Minute),
	}, nonce))

	var success atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := tickets.Consume(ctx, nonce, 1); err == nil {
				success.Add(1)
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, int32(1), success.Load())
}
