package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	crosstalk "github.com/aleksclark/crosstalk/server"
)

// ChannelStore implements crosstalk.ChannelService.
type ChannelStore struct {
	db *DB
}

func NewChannelStore(db *DB) *ChannelStore {
	return &ChannelStore{db: db}
}

func (s *ChannelStore) List(ctx context.Context, sessionID string) ([]crosstalk.Channel, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, session_id, name, type, created_at FROM channels WHERE session_id = ? ORDER BY created_at", sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []crosstalk.Channel
	for rows.Next() {
		var ch crosstalk.Channel
		var createdAt string
		if err := rows.Scan(&ch.ID, &ch.SessionID, &ch.Name, &ch.Type, &createdAt); err != nil {
			return nil, err
		}
		ch.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		channels = append(channels, ch)
	}
	return channels, rows.Err()
}

func (s *ChannelStore) Get(ctx context.Context, id string) (*crosstalk.Channel, error) {
	var ch crosstalk.Channel
	var createdAt string
	err := s.db.QueryRowContext(ctx,
		"SELECT id, session_id, name, type, created_at FROM channels WHERE id = ?", id).
		Scan(&ch.ID, &ch.SessionID, &ch.Name, &ch.Type, &createdAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("channel not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	ch.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &ch, nil
}

func (s *ChannelStore) Create(ctx context.Context, ch *crosstalk.Channel) error {
	if ch.CreatedAt.IsZero() {
		ch.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO channels (id, session_id, name, type, created_at) VALUES (?, ?, ?, ?, ?)",
		ch.ID, ch.SessionID, ch.Name, ch.Type, ch.CreatedAt.Format(time.RFC3339))
	return err
}

func (s *ChannelStore) Update(ctx context.Context, ch *crosstalk.Channel) error {
	result, err := s.db.ExecContext(ctx,
		"UPDATE channels SET name = ?, type = ? WHERE id = ?",
		ch.Name, ch.Type, ch.ID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("channel not found: %s", ch.ID)
	}
	return nil
}

func (s *ChannelStore) Delete(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM channels WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("channel not found: %s", id)
	}
	return nil
}
