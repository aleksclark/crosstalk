package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	crosstalk "github.com/aleksclark/crosstalk/server"
)

// SourceStore implements crosstalk.SourceService.
type SourceStore struct {
	db *DB
}

func NewSourceStore(db *DB) *SourceStore {
	return &SourceStore{db: db}
}

func (s *SourceStore) List(ctx context.Context, sessionID string) ([]crosstalk.Source, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, session_id, name, origin, peer_id, connected, first_seen, last_seen FROM sources WHERE session_id = ? ORDER BY first_seen", sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sources []crosstalk.Source
	for rows.Next() {
		var src crosstalk.Source
		var peerID sql.NullString
		var firstSeen, lastSeen string
		if err := rows.Scan(&src.ID, &src.SessionID, &src.Name, &src.Origin, &peerID, &src.Connected, &firstSeen, &lastSeen); err != nil {
			return nil, err
		}
		if peerID.Valid {
			src.PeerID = &peerID.String
		}
		src.FirstSeen, _ = time.Parse(time.RFC3339, firstSeen)
		src.LastSeen, _ = time.Parse(time.RFC3339, lastSeen)
		sources = append(sources, src)
	}
	return sources, rows.Err()
}

func (s *SourceStore) Get(ctx context.Context, id string) (*crosstalk.Source, error) {
	var src crosstalk.Source
	var peerID sql.NullString
	var firstSeen, lastSeen string
	err := s.db.QueryRowContext(ctx,
		"SELECT id, session_id, name, origin, peer_id, connected, first_seen, last_seen FROM sources WHERE id = ?", id).
		Scan(&src.ID, &src.SessionID, &src.Name, &src.Origin, &peerID, &src.Connected, &firstSeen, &lastSeen)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("source not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	if peerID.Valid {
		src.PeerID = &peerID.String
	}
	src.FirstSeen, _ = time.Parse(time.RFC3339, firstSeen)
	src.LastSeen, _ = time.Parse(time.RFC3339, lastSeen)
	return &src, nil
}

func (s *SourceStore) Create(ctx context.Context, src *crosstalk.Source) error {
	now := time.Now().UTC()
	if src.FirstSeen.IsZero() {
		src.FirstSeen = now
	}
	if src.LastSeen.IsZero() {
		src.LastSeen = now
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO sources (id, session_id, name, origin, peer_id, connected, first_seen, last_seen) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		src.ID, src.SessionID, src.Name, src.Origin, src.PeerID, src.Connected,
		src.FirstSeen.Format(time.RFC3339), src.LastSeen.Format(time.RFC3339))
	return err
}

func (s *SourceStore) Update(ctx context.Context, src *crosstalk.Source) error {
	src.LastSeen = time.Now().UTC()
	result, err := s.db.ExecContext(ctx,
		"UPDATE sources SET name = ?, peer_id = ?, connected = ?, last_seen = ? WHERE id = ?",
		src.Name, src.PeerID, src.Connected, src.LastSeen.Format(time.RFC3339), src.ID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("source not found: %s", src.ID)
	}
	return nil
}

func (s *SourceStore) Delete(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM sources WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("source not found: %s", id)
	}
	return nil
}
