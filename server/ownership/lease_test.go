package ownership_test

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
	"github.com/aleksclark/crosstalk/server/ownership"
	"github.com/aleksclark/crosstalk/server/pgtest"
	"github.com/aleksclark/crosstalk/server/postgres"
)

func createSession(t *testing.T, db *postgres.DB) string {
	t.Helper()
	id := ulid.Make().String()
	require.NoError(t, postgres.NewSessionStore(db).Create(context.Background(), &crosstalk.Session{
		ID:   id,
		Name: "lease-session",
	}))
	return id
}

func TestLease_AcquireIncrementsGenerationAndBlocksSecondOwner(t *testing.T) {
	db := pgtest.New(t)
	svc := ownership.NewStore(db.DB)
	ctx := context.Background()
	sid := createSession(t, db)

	l1, err := svc.Acquire(ctx, sid, "owner-a", 30*time.Second)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), l1.Generation)
	assert.Equal(t, "owner-a", l1.OwnerID)
	assert.True(t, l1.ExpiresAt.After(time.Now()))

	_, err = svc.Acquire(ctx, sid, "owner-b", 30*time.Second)
	require.Error(t, err)
	assert.True(t, errors.Is(err, crosstalk.ErrLeaseHeld), "%v", err)

	cur, err := svc.Current(ctx, sid)
	require.NoError(t, err)
	assert.Equal(t, "owner-a", cur.OwnerID)
	assert.Equal(t, uint64(1), cur.Generation)
}

func TestLease_ExpiredOrReleasedReacquiredWithLargerGeneration(t *testing.T) {
	db := pgtest.New(t)
	svc := ownership.NewStore(db.DB)
	ctx := context.Background()
	sid := createSession(t, db)

	l1, err := svc.Acquire(ctx, sid, "owner-a", 30*time.Second)
	require.NoError(t, err)
	require.NoError(t, svc.Release(ctx, l1))

	l2, err := svc.Acquire(ctx, sid, "owner-b", 30*time.Second)
	require.NoError(t, err)
	assert.Equal(t, "owner-b", l2.OwnerID)
	assert.Greater(t, l2.Generation, l1.Generation)

	// Force expiry.
	_, err = db.ExecContext(ctx, `UPDATE sessions SET lease_until = now() - interval '1 second' WHERE id = ?`, sid)
	require.NoError(t, err)

	l3, err := svc.Acquire(ctx, sid, "owner-c", 30*time.Second)
	require.NoError(t, err)
	assert.Equal(t, "owner-c", l3.OwnerID)
	assert.Greater(t, l3.Generation, l2.Generation)
}

func TestLease_StaleGenerationCannotRenewOrRelease(t *testing.T) {
	db := pgtest.New(t)
	svc := ownership.NewStore(db.DB)
	ctx := context.Background()
	sid := createSession(t, db)

	l1, err := svc.Acquire(ctx, sid, "owner-a", time.Second)
	require.NoError(t, err)

	// Expire and re-acquire as B so generation advances.
	_, err = db.ExecContext(ctx, `UPDATE sessions SET lease_until = now() - interval '1 second' WHERE id = ?`, sid)
	require.NoError(t, err)
	l2, err := svc.Acquire(ctx, sid, "owner-b", 30*time.Second)
	require.NoError(t, err)
	assert.Greater(t, l2.Generation, l1.Generation)

	_, err = svc.Renew(ctx, l1, 30*time.Second)
	require.Error(t, err)
	assert.True(t, errors.Is(err, crosstalk.ErrStaleGeneration) || errors.Is(err, crosstalk.ErrLeaseNotHeld), "%v", err)

	err = svc.Release(ctx, l1)
	require.Error(t, err)
	assert.True(t, errors.Is(err, crosstalk.ErrStaleGeneration) || errors.Is(err, crosstalk.ErrLeaseNotHeld), "%v", err)

	// Current owner can renew/release.
	renewed, err := svc.Renew(ctx, l2, 45*time.Second)
	require.NoError(t, err)
	assert.Equal(t, l2.Generation, renewed.Generation)
	require.NoError(t, svc.Release(ctx, renewed))
}

func TestLease_ConcurrentAcquireOneWinner(t *testing.T) {
	db := pgtest.New(t)
	svc := ownership.NewStore(db.DB)
	ctx := context.Background()
	sid := createSession(t, db)

	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			owner := ulid.Make().String()
			if _, err := svc.Acquire(ctx, sid, owner, 30*time.Second); err == nil {
				wins.Add(1)
			}
		}(i)
	}
	wg.Wait()
	assert.Equal(t, int32(1), wins.Load())
}
