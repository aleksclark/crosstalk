package sqlite

import (
	"context"
	"fmt"

	crosstalk "github.com/nicosql/crosstalk/server"
	"github.com/oklog/ulid/v2"
)

// MixStore implements crosstalk.MixService.
type MixStore struct {
	db *DB
}

func NewMixStore(db *DB) *MixStore {
	return &MixStore{db: db}
}

func (s *MixStore) GetMix(ctx context.Context, channelID string) ([]crosstalk.MixEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, channel_id, source_id, muted, level FROM channel_mix WHERE channel_id = ?", channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []crosstalk.MixEntry
	for rows.Next() {
		var e crosstalk.MixEntry
		if err := rows.Scan(&e.ID, &e.ChannelID, &e.SourceID, &e.Muted, &e.Level); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (s *MixStore) SetMix(ctx context.Context, channelID string, entries []crosstalk.MixEntry) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Upsert each entry
	for _, e := range entries {
		if e.ID == "" {
			e.ID = ulid.Make().String()
		}
		if e.ChannelID == "" {
			e.ChannelID = channelID
		}
		if e.ChannelID != channelID {
			return fmt.Errorf("mix entry channel_id mismatch: got %s, want %s", e.ChannelID, channelID)
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO channel_mix (id, channel_id, source_id, muted, level) VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT(channel_id, source_id) DO UPDATE SET muted = excluded.muted, level = excluded.level`,
			e.ID, e.ChannelID, e.SourceID, e.Muted, e.Level)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}
