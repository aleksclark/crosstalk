package postgres

import (
	"context"
	"fmt"

	"github.com/oklog/ulid/v2"
	"github.com/uptrace/bun"

	crosstalk "github.com/aleksclark/crosstalk/server"
)

// MixStore implements crosstalk.MixService.
type MixStore struct {
	db *DB
}

func NewMixStore(db *DB) *MixStore {
	return &MixStore{db: db}
}

func (s *MixStore) GetMix(ctx context.Context, channelID string) ([]crosstalk.MixEntry, error) {
	var models []mixModel
	err := s.db.NewSelect().Model(&models).Where("channel_id = ?", channelID).Scan(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]crosstalk.MixEntry, 0, len(models))
	for i := range models {
		out = append(out, models[i].toDomain())
	}
	return out, nil
}

func (s *MixStore) SetMix(ctx context.Context, channelID string, entries []crosstalk.MixEntry) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// SetMix replaces the channel's full mix. Delete entries for this channel
	// that are no longer present so removing a source actually persists; when
	// the new set is empty, all entries for the channel are removed.
	keep := make([]string, 0, len(entries))
	for _, e := range entries {
		keep = append(keep, e.SourceID)
	}
	del := tx.NewDelete().Model((*mixModel)(nil)).Where("channel_id = ?", channelID)
	if len(keep) > 0 {
		del = del.Where("source_id NOT IN (?)", bun.In(keep))
	}
	if _, err := del.Exec(ctx); err != nil {
		return err
	}

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
		m := &mixModel{
			ID:        e.ID,
			ChannelID: e.ChannelID,
			SourceID:  e.SourceID,
			Muted:     e.Muted,
			Level:     e.Level,
		}
		_, err := tx.NewInsert().Model(m).
			On("CONFLICT (channel_id, source_id) DO UPDATE").
			Set("muted = EXCLUDED.muted").
			Set("level = EXCLUDED.level").
			Exec(ctx)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}
