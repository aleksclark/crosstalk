package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"

	crosstalk "github.com/aleksclark/crosstalk/server"
)

// MediaTicketStore implements crosstalk.MediaTicketService.
type MediaTicketStore struct {
	db *DB
}

// NewMediaTicketStore constructs a media ticket store.
func NewMediaTicketStore(db *DB) *MediaTicketStore {
	return &MediaTicketStore{db: db}
}

// HashNonce returns the SHA-256 hex digest of a media-ticket nonce.
func HashNonce(nonce string) string {
	sum := sha256.Sum256([]byte(nonce))
	return hex.EncodeToString(sum[:])
}

// Issue persists a media ticket, storing only the hash of the nonce.
func (s *MediaTicketStore) Issue(ctx context.Context, ticket *crosstalk.MediaTicket, nonce string) error {
	if ticket == nil {
		return fmt.Errorf("%w: nil ticket", crosstalk.ErrTicketInvalid)
	}
	if nonce == "" {
		return fmt.Errorf("%w: empty nonce", crosstalk.ErrTicketInvalid)
	}
	if ticket.SessionID == "" || ticket.Subject == "" || ticket.Role == "" {
		return fmt.Errorf("%w: session, subject, and role are required", crosstalk.ErrTicketInvalid)
	}
	if ticket.ExpiresAt.IsZero() {
		return fmt.Errorf("%w: expiry required", crosstalk.ErrTicketInvalid)
	}
	if ticket.ID == "" {
		ticket.ID = ulid.Make().String()
	}
	if ticket.CreatedAt.IsZero() {
		ticket.CreatedAt = time.Now().UTC()
	}
	ticket.NonceHash = HashNonce(nonce)
	produce := ticket.ProduceChannelIDs
	if produce == nil {
		produce = []string{}
	}
	listen := ticket.ListenChannelIDs
	if listen == nil {
		listen = []string{}
	}
	ticket.ProduceChannelIDs = produce
	ticket.ListenChannelIDs = listen

	m := &mediaTicketModel{
		ID:                ticket.ID,
		NonceHash:         ticket.NonceHash,
		SessionID:         ticket.SessionID,
		OwnerID:           ticket.OwnerID,
		OwnerGeneration:   int64(ticket.OwnerGeneration),
		Subject:           ticket.Subject,
		Role:              ticket.Role,
		ProduceChannelIDs: produce,
		ListenChannelIDs:  listen,
		ExpiresAt:         ticket.ExpiresAt.UTC(),
		ConsumedAt:        nil,
		CreatedAt:         ticket.CreatedAt.UTC(),
	}
	// Empty slices must not become SQL NULL for NOT NULL array columns.
	// Use explicit DEFAULT when empty via two-path insert.
	if len(produce) == 0 && len(listen) == 0 {
		_, err := s.db.NewRaw(`
			INSERT INTO media_tickets (
				id, nonce_hash, session_id, owner_id, owner_generation,
				subject, role, produce_channel_ids, listen_channel_ids,
				expires_at, consumed_at, created_at
			) VALUES (
				?, ?, ?, ?, ?,
				?, ?, '{}', '{}',
				?, NULL, ?
			)
		`,
			m.ID, m.NonceHash, m.SessionID, m.OwnerID, m.OwnerGeneration,
			m.Subject, m.Role,
			m.ExpiresAt, m.CreatedAt,
		).Exec(ctx)
		return err
	}
	_, err := s.db.NewInsert().Model(m).Exec(ctx)
	return err
}

// Consume atomically marks an unconsumed, unexpired ticket as used when:
//  1. the ticket's stored owner_generation matches the provided fence, and
//  2. the session's current owner_generation still matches (stale tickets fail
//     closed after a lease takeover).
func (s *MediaTicketStore) Consume(ctx context.Context, nonce string, ownerGeneration uint64) (*crosstalk.MediaTicket, error) {
	if nonce == "" {
		return nil, fmt.Errorf("%w: empty nonce", crosstalk.ErrTicketInvalid)
	}
	hash := HashNonce(nonce)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	m := new(mediaTicketModel)
	err = tx.NewSelect().Model(m).
		Where("nonce_hash = ?", hash).
		For("UPDATE").
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, crosstalk.ErrTicketNotFound
	}
	if err != nil {
		return nil, err
	}

	if m.ConsumedAt != nil {
		return nil, crosstalk.ErrTicketConsumed
	}

	var expired bool
	if err := tx.QueryRowContext(ctx, `SELECT expires_at <= now() FROM media_tickets WHERE id = ?`, m.ID).Scan(&expired); err != nil {
		return nil, err
	}
	if expired {
		return nil, crosstalk.ErrTicketExpired
	}

	if uint64(m.OwnerGeneration) != ownerGeneration {
		return nil, fmt.Errorf("%w: ticket generation %d != %d", crosstalk.ErrStaleGeneration, m.OwnerGeneration, ownerGeneration)
	}

	// Fence against session takeover: live session generation must still match.
	var sessionGen int64
	err = tx.QueryRowContext(ctx, `
		SELECT owner_generation FROM sessions WHERE id = ? FOR SHARE
	`, m.SessionID).Scan(&sessionGen)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("session not found: %s", m.SessionID)
	}
	if err != nil {
		return nil, err
	}
	if uint64(sessionGen) != ownerGeneration {
		return nil, fmt.Errorf("%w: session generation %d != %d", crosstalk.ErrStaleGeneration, sessionGen, ownerGeneration)
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE media_tickets
		SET consumed_at = now()
		WHERE id = ?
		  AND consumed_at IS NULL
		  AND expires_at > now()
		  AND owner_generation = ?
	`, m.ID, int64(ownerGeneration))
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, crosstalk.ErrTicketConsumed
	}

	m2 := new(mediaTicketModel)
	if err := tx.NewSelect().Model(m2).Where("id = ?", m.ID).Scan(ctx); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	out := m2.toDomain()
	return &out, nil
}

// GetByNonceHash looks up a ticket by hash without consuming it.
func (s *MediaTicketStore) GetByNonceHash(ctx context.Context, nonceHash string) (*crosstalk.MediaTicket, error) {
	m := new(mediaTicketModel)
	err := s.db.NewSelect().Model(m).Where("nonce_hash = ?", nonceHash).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, crosstalk.ErrTicketNotFound
	}
	if err != nil {
		return nil, err
	}
	out := m.toDomain()
	return &out, nil
}
