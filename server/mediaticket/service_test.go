package mediaticket_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	crosstalk "github.com/aleksclark/crosstalk/server"
	"github.com/aleksclark/crosstalk/server/mediaticket"
	"github.com/aleksclark/crosstalk/server/ownership"
	"github.com/aleksclark/crosstalk/server/pgtest"
	"github.com/aleksclark/crosstalk/server/postgres"
)

func TestMediaTicket_IssueAndConsume(t *testing.T) {
	db := pgtest.New(t)
	ctx := context.Background()
	sessions := postgres.NewSessionStore(db)
	sid := ulid.Make().String()
	require.NoError(t, sessions.Create(ctx, &crosstalk.Session{ID: sid, Name: "mt"}))

	leases := ownership.NewStore(db.DB)
	lease, err := leases.Acquire(ctx, sid, "instance-1", time.Minute)
	require.NoError(t, err)

	svc := mediaticket.NewService(postgres.NewMediaTicketStore(db), []byte("test-secret"))
	issued, err := svc.Issue(ctx, mediaticket.IssueRequest{
		SessionID:         sid,
		OwnerID:           lease.OwnerID,
		OwnerGeneration:   lease.Generation,
		Subject:           "translator-1",
		Role:              "translator",
		ProduceChannelIDs: []string{"prod-1"},
		ListenChannelIDs:  []string{"listen-1", "listen-2"},
		TTL:               30 * time.Second,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, issued.Token)
	assert.NotEmpty(t, issued.Nonce)
	assert.Equal(t, mediaticket.Audience, string(issued.Claims.Audience[0]))
	assert.Equal(t, lease.Generation, issued.Claims.OwnerGeneration)
	assert.LessOrEqual(t, issued.Claims.ExpiresAt.Sub(issued.Claims.IssuedAt.Time), mediaticket.MaxTTL)

	claims, err := svc.ParseToken(issued.Token)
	require.NoError(t, err)
	assert.Equal(t, "translator-1", claims.Subject)
	assert.Equal(t, []string{"prod-1"}, claims.ProduceChannelIDs)

	got, err := svc.Consume(ctx, mediaticket.ConsumeRequest{
		Token:           issued.Token,
		OwnerGeneration: lease.Generation,
	})
	require.NoError(t, err)
	require.NotNil(t, got.ConsumedAt)
	assert.Equal(t, "translator", got.Role)

	_, err = svc.Consume(ctx, mediaticket.ConsumeRequest{
		Token:           issued.Token,
		OwnerGeneration: lease.Generation,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, crosstalk.ErrTicketConsumed), "%v", err)
}

func TestMediaTicket_WrongGenerationAndExpiry(t *testing.T) {
	db := pgtest.New(t)
	ctx := context.Background()
	sessions := postgres.NewSessionStore(db)
	sid := ulid.Make().String()
	require.NoError(t, sessions.Create(ctx, &crosstalk.Session{ID: sid, Name: "mt2"}))

	leases := ownership.NewStore(db.DB)
	lease, err := leases.Acquire(ctx, sid, "instance-1", time.Minute)
	require.NoError(t, err)

	store := postgres.NewMediaTicketStore(db)
	svc := mediaticket.NewService(store, nil)

	issued, err := svc.Issue(ctx, mediaticket.IssueRequest{
		SessionID:       sid,
		OwnerID:         lease.OwnerID,
		OwnerGeneration: lease.Generation,
		Subject:         "abc-1",
		Role:            "abc",
		TTL:             20 * time.Second,
	})
	require.NoError(t, err)

	_, err = svc.Consume(ctx, mediaticket.ConsumeRequest{
		Nonce:           issued.Nonce,
		OwnerGeneration: lease.Generation + 1,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, crosstalk.ErrStaleGeneration), "%v", err)

	_, err = db.ExecContext(ctx, `UPDATE media_tickets SET expires_at = now() - interval '2 seconds' WHERE nonce_hash = ?`,
		mediaticket.HashNonce(issued.Nonce))
	require.NoError(t, err)
	_, err = svc.Consume(ctx, mediaticket.ConsumeRequest{
		Nonce:           issued.Nonce,
		OwnerGeneration: lease.Generation,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, crosstalk.ErrTicketExpired), "%v", err)
}

func TestMediaTicket_TTLCapped(t *testing.T) {
	db := pgtest.New(t)
	ctx := context.Background()
	sessions := postgres.NewSessionStore(db)
	sid := ulid.Make().String()
	require.NoError(t, sessions.Create(ctx, &crosstalk.Session{ID: sid, Name: "ttl"}))

	svc := mediaticket.NewService(postgres.NewMediaTicketStore(db), nil)
	_, err := svc.Issue(ctx, mediaticket.IssueRequest{
		SessionID:       sid,
		OwnerID:         "o",
		OwnerGeneration: 1,
		Subject:         "s",
		Role:            "admin",
		TTL:             2 * time.Minute,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, crosstalk.ErrTicketInvalid), "%v", err)
}

func TestMediaTicket_StaleOwnerCannotConsumeAfterLeaseTakeover(t *testing.T) {
	db := pgtest.New(t)
	ctx := context.Background()
	sessions := postgres.NewSessionStore(db)
	sid := ulid.Make().String()
	require.NoError(t, sessions.Create(ctx, &crosstalk.Session{ID: sid, Name: "takeover"}))

	leases := ownership.NewStore(db.DB)
	l1, err := leases.Acquire(ctx, sid, "owner-a", time.Minute)
	require.NoError(t, err)

	svc := mediaticket.NewService(postgres.NewMediaTicketStore(db), []byte("k"))
	issued, err := svc.Issue(ctx, mediaticket.IssueRequest{
		SessionID:       sid,
		OwnerID:         l1.OwnerID,
		OwnerGeneration: l1.Generation,
		Subject:         "dev",
		Role:            "translator",
		TTL:             30 * time.Second,
	})
	require.NoError(t, err)

	// Force expiry and takeover — generation advances.
	_, err = db.ExecContext(ctx, `UPDATE sessions SET lease_until = now() - interval '1 second' WHERE id = ?`, sid)
	require.NoError(t, err)
	l2, err := leases.Acquire(ctx, sid, "owner-b", time.Minute)
	require.NoError(t, err)
	assert.Greater(t, l2.Generation, l1.Generation)

	// Live fence (new gen) rejects ticket minted under old gen.
	_, err = svc.Consume(ctx, mediaticket.ConsumeRequest{
		Token:           issued.Token,
		OwnerGeneration: l2.Generation,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, crosstalk.ErrStaleGeneration), "%v", err)

	// Stale fence (old gen) also fails because session generation advanced.
	_, err = svc.Consume(ctx, mediaticket.ConsumeRequest{
		Token:           issued.Token,
		OwnerGeneration: l1.Generation,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, crosstalk.ErrStaleGeneration), "%v", err)
}
