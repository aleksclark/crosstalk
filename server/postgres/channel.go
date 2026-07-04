package postgres

import (
	"context"
	"database/sql"
	"errors"
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
	var models []channelModel
	err := s.db.NewSelect().Model(&models).
		Where("session_id = ?", sessionID).
		Order("created_at").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]crosstalk.Channel, 0, len(models))
	for i := range models {
		out = append(out, models[i].toDomain())
	}
	return out, nil
}

func (s *ChannelStore) Get(ctx context.Context, id string) (*crosstalk.Channel, error) {
	m := new(channelModel)
	err := s.db.NewSelect().Model(m).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("channel not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	out := m.toDomain()
	return &out, nil
}

func (s *ChannelStore) Create(ctx context.Context, ch *crosstalk.Channel) error {
	if ch.CreatedAt.IsZero() {
		ch.CreatedAt = time.Now().UTC()
	}
	m := &channelModel{
		ID:        ch.ID,
		SessionID: ch.SessionID,
		Name:      ch.Name,
		Type:      string(ch.Type),
		CreatedAt: ch.CreatedAt,
	}
	_, err := s.db.NewInsert().Model(m).Exec(ctx)
	return err
}

func (s *ChannelStore) Update(ctx context.Context, ch *crosstalk.Channel) error {
	res, err := s.db.NewUpdate().Model((*channelModel)(nil)).
		Set("name = ?", ch.Name).
		Set("type = ?", string(ch.Type)).
		Where("id = ?", ch.ID).
		Exec(ctx)
	if err != nil {
		return err
	}
	return requireAffected(res, "channel", ch.ID)
}

func (s *ChannelStore) Delete(ctx context.Context, id string) error {
	res, err := s.db.NewDelete().Model((*channelModel)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return err
	}
	return requireAffected(res, "channel", id)
}
