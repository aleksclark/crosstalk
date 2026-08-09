// Package ownership provides fenced one-owner session leases backed by
// PostgreSQL. Lease decisions use SQL now() so wall-clock skew on callers
// cannot bypass fencing.
package ownership

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	crosstalk "github.com/aleksclark/crosstalk/server"
)

// Lease is a fenced session ownership grant.
type Lease struct {
	SessionID  string
	OwnerID    string
	Generation uint64
	ExpiresAt  time.Time
}

// Service manages exclusive session owner leases.
type Service interface {
	Acquire(ctx context.Context, sessionID, ownerID string, ttl time.Duration) (Lease, error)
	Renew(ctx context.Context, lease Lease, ttl time.Duration) (Lease, error)
	Release(ctx context.Context, lease Lease) error
	Current(ctx context.Context, sessionID string) (Lease, error)
}

// Store implements Service against the sessions table via Bun.
type Store struct {
	db *bun.DB
}

// NewStore constructs a lease service. Pass the *bun.DB from postgres.DB
// (embedded field) so this package does not import the postgres subpackage.
func NewStore(db *bun.DB) *Store {
	return &Store{db: db}
}

// Acquire takes ownership of a session when no unexpired lease is held.
// On success owner_generation is incremented and owner_id/lease_until set.
func (s *Store) Acquire(ctx context.Context, sessionID, ownerID string, ttl time.Duration) (Lease, error) {
	if sessionID == "" || ownerID == "" {
		return Lease{}, fmt.Errorf("sessionID and ownerID are required")
	}
	if ttl <= 0 {
		return Lease{}, fmt.Errorf("ttl must be positive")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Lease{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var (
		curOwner   string
		curGen     int64
		leaseUntil sql.NullTime
	)
	err = tx.QueryRowContext(ctx, `
		SELECT owner_id, owner_generation, lease_until
		FROM sessions
		WHERE id = ?
		FOR UPDATE
	`, sessionID).Scan(&curOwner, &curGen, &leaseUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return Lease{}, fmt.Errorf("session not found: %s", sessionID)
	}
	if err != nil {
		return Lease{}, err
	}

	// Block when another owner holds an unexpired lease.
	var held bool
	err = tx.QueryRowContext(ctx, `
		SELECT CASE
			WHEN lease_until IS NOT NULL AND lease_until > now()
				AND owner_id <> '' AND owner_id <> ?
			THEN TRUE ELSE FALSE
		END
		FROM sessions WHERE id = ?
	`, ownerID, sessionID).Scan(&held)
	if err != nil {
		return Lease{}, err
	}
	if held {
		return Lease{}, crosstalk.ErrLeaseHeld
	}

	// Same owner re-acquire of an unexpired lease is a renew (no gen bump).
	var sameOwnerUnexpired bool
	err = tx.QueryRowContext(ctx, `
		SELECT CASE
			WHEN owner_id = ? AND lease_until IS NOT NULL AND lease_until > now()
			THEN TRUE ELSE FALSE
		END
		FROM sessions WHERE id = ?
	`, ownerID, sessionID).Scan(&sameOwnerUnexpired)
	if err != nil {
		return Lease{}, err
	}

	var newGen int64
	var expiresAt time.Time
	if sameOwnerUnexpired {
		err = tx.QueryRowContext(ctx, `
			UPDATE sessions
			SET lease_until = now() + (?::text || ' seconds')::interval,
			    updated_at = now()
			WHERE id = ? AND owner_id = ? AND owner_generation = ?
			RETURNING owner_generation, lease_until
		`, fmt.Sprintf("%f", ttl.Seconds()), sessionID, ownerID, curGen).Scan(&newGen, &expiresAt)
		if err != nil {
			return Lease{}, err
		}
	} else {
		// Expired, released, or empty: bump generation and assign.
		err = tx.QueryRowContext(ctx, `
			UPDATE sessions
			SET owner_id = ?,
			    owner_generation = owner_generation + 1,
			    lease_until = now() + (?::text || ' seconds')::interval,
			    updated_at = now()
			WHERE id = ?
			  AND (
			    lease_until IS NULL
			    OR lease_until <= now()
			    OR owner_id = ''
			    OR owner_id = ?
			  )
			RETURNING owner_generation, lease_until
		`, ownerID, fmt.Sprintf("%f", ttl.Seconds()), sessionID, ownerID).Scan(&newGen, &expiresAt)
		if errors.Is(err, sql.ErrNoRows) {
			return Lease{}, crosstalk.ErrLeaseHeld
		}
		if err != nil {
			return Lease{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return Lease{}, err
	}
	return Lease{
		SessionID:  sessionID,
		OwnerID:    ownerID,
		Generation: uint64(newGen),
		ExpiresAt:  expiresAt.UTC(),
	}, nil
}

// Renew extends an unexpired lease held by the exact owner+generation fence.
func (s *Store) Renew(ctx context.Context, lease Lease, ttl time.Duration) (Lease, error) {
	if ttl <= 0 {
		return Lease{}, fmt.Errorf("ttl must be positive")
	}
	var (
		newGen    int64
		expiresAt time.Time
	)
	err := s.db.QueryRowContext(ctx, `
		UPDATE sessions
		SET lease_until = now() + (?::text || ' seconds')::interval,
		    updated_at = now()
		WHERE id = ?
		  AND owner_id = ?
		  AND owner_generation = ?
		  AND lease_until IS NOT NULL
		  AND lease_until > now()
		RETURNING owner_generation, lease_until
	`, fmt.Sprintf("%f", ttl.Seconds()), lease.SessionID, lease.OwnerID, int64(lease.Generation)).
		Scan(&newGen, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		cur, cerr := s.Current(ctx, lease.SessionID)
		if cerr == nil && cur.Generation != 0 && cur.Generation != lease.Generation {
			return Lease{}, crosstalk.ErrStaleGeneration
		}
		return Lease{}, crosstalk.ErrLeaseNotHeld
	}
	if err != nil {
		return Lease{}, err
	}
	return Lease{
		SessionID:  lease.SessionID,
		OwnerID:    lease.OwnerID,
		Generation: uint64(newGen),
		ExpiresAt:  expiresAt.UTC(),
	}, nil
}

// Release clears an unexpired lease held by the exact owner+generation fence.
// A stale owner cannot release a newer lease.
func (s *Store) Release(ctx context.Context, lease Lease) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE sessions
		SET owner_id = '',
		    lease_until = NULL,
		    updated_at = now()
		WHERE id = ?
		  AND owner_id = ?
		  AND owner_generation = ?
	`, lease.SessionID, lease.OwnerID, int64(lease.Generation))
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		cur, cerr := s.Current(ctx, lease.SessionID)
		if cerr == nil && cur.Generation != 0 && cur.Generation != lease.Generation {
			return crosstalk.ErrStaleGeneration
		}
		return crosstalk.ErrLeaseNotHeld
	}
	return nil
}

// Current returns the current lease snapshot. When no owner is set, Generation
// may still be non-zero (last generation) with empty OwnerID and zero ExpiresAt.
func (s *Store) Current(ctx context.Context, sessionID string) (Lease, error) {
	var (
		ownerID    string
		gen        int64
		leaseUntil sql.NullTime
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT owner_id, owner_generation, lease_until
		FROM sessions WHERE id = ?
	`, sessionID).Scan(&ownerID, &gen, &leaseUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return Lease{}, fmt.Errorf("session not found: %s", sessionID)
	}
	if err != nil {
		return Lease{}, err
	}
	l := Lease{
		SessionID:  sessionID,
		OwnerID:    ownerID,
		Generation: uint64(gen),
	}
	if leaseUntil.Valid {
		var active bool
		_ = s.db.QueryRowContext(ctx, `SELECT lease_until > now() FROM sessions WHERE id = ?`, sessionID).Scan(&active)
		if active {
			l.ExpiresAt = leaseUntil.Time.UTC()
		}
	}
	return l, nil
}
