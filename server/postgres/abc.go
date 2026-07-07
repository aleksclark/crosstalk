package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	crosstalk "github.com/aleksclark/crosstalk/server"
)

// ABCStore implements crosstalk.ABCService.
type ABCStore struct {
	db *DB
}

func NewABCStore(db *DB) *ABCStore {
	return &ABCStore{db: db}
}

func (s *ABCStore) List(ctx context.Context) ([]crosstalk.ABC, error) {
	var models []abcModel
	err := s.db.NewSelect().Model(&models).Order("created_at DESC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]crosstalk.ABC, 0, len(models))
	for i := range models {
		out = append(out, models[i].toDomain())
	}
	return out, nil
}

func (s *ABCStore) Get(ctx context.Context, id string) (*crosstalk.ABC, error) {
	m := new(abcModel)
	err := s.db.NewSelect().Model(m).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("abc not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	out := m.toDomain()
	return &out, nil
}

func (s *ABCStore) Create(ctx context.Context, abc *crosstalk.ABC) error {
	if abc.CreatedAt.IsZero() {
		abc.CreatedAt = time.Now().UTC()
	}
	m := &abcModel{
		ID:               abc.ID,
		Name:             abc.Name,
		TokenHash:        abc.TokenHash,
		SessionID:        abc.SessionID,
		MonitorChannelID: abc.MonitorChannelID,
		Connected:        abc.Connected,
		LastSeen:         abc.LastSeen,
		CreatedAt:        abc.CreatedAt,
	}
	_, err := s.db.NewInsert().Model(m).Exec(ctx)
	return err
}

func (s *ABCStore) Update(ctx context.Context, abc *crosstalk.ABC) error {
	res, err := s.db.NewUpdate().Model((*abcModel)(nil)).
		Set("name = ?", abc.Name).
		Set("session_id = ?", abc.SessionID).
		Set("monitor_channel_id = ?", abc.MonitorChannelID).
		Set("connected = ?", abc.Connected).
		Set("last_seen = ?", abc.LastSeen).
		Where("id = ?", abc.ID).
		Exec(ctx)
	if err != nil {
		return err
	}
	return requireAffected(res, "abc", abc.ID)
}

func (s *ABCStore) Delete(ctx context.Context, id string) error {
	res, err := s.db.NewDelete().Model((*abcModel)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return err
	}
	return requireAffected(res, "abc", id)
}

func (s *ABCStore) GetByTokenHash(ctx context.Context, tokenHash string) (*crosstalk.ABC, error) {
	m := new(abcModel)
	err := s.db.NewSelect().Model(m).Where("token_hash = ?", tokenHash).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("abc not found for token")
	}
	if err != nil {
		return nil, err
	}
	out := m.toDomain()
	return &out, nil
}
