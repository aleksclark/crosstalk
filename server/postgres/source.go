package postgres

import (
	"context"
	"database/sql"
	"errors"
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
	var models []sourceModel
	err := s.db.NewSelect().Model(&models).
		Where("session_id = ?", sessionID).
		Order("first_seen").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]crosstalk.Source, 0, len(models))
	for i := range models {
		out = append(out, models[i].toDomain())
	}
	return out, nil
}

func (s *SourceStore) Get(ctx context.Context, id string) (*crosstalk.Source, error) {
	m := new(sourceModel)
	err := s.db.NewSelect().Model(m).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("source not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	out := m.toDomain()
	return &out, nil
}

func (s *SourceStore) Create(ctx context.Context, src *crosstalk.Source) error {
	now := time.Now().UTC()
	if src.FirstSeen.IsZero() {
		src.FirstSeen = now
	}
	if src.LastSeen.IsZero() {
		src.LastSeen = now
	}
	m := &sourceModel{
		ID:        src.ID,
		SessionID: src.SessionID,
		Name:      src.Name,
		Origin:    string(src.Origin),
		PeerID:    src.PeerID,
		Connected: src.Connected,
		FirstSeen: src.FirstSeen,
		LastSeen:  src.LastSeen,
	}
	_, err := s.db.NewInsert().Model(m).Exec(ctx)
	return err
}

func (s *SourceStore) Update(ctx context.Context, src *crosstalk.Source) error {
	src.LastSeen = time.Now().UTC()
	res, err := s.db.NewUpdate().Model((*sourceModel)(nil)).
		Set("name = ?", src.Name).
		Set("peer_id = ?", src.PeerID).
		Set("connected = ?", src.Connected).
		Set("last_seen = ?", src.LastSeen).
		Where("id = ?", src.ID).
		Exec(ctx)
	if err != nil {
		return err
	}
	return requireAffected(res, "source", src.ID)
}

func (s *SourceStore) Delete(ctx context.Context, id string) error {
	res, err := s.db.NewDelete().Model((*sourceModel)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return err
	}
	return requireAffected(res, "source", id)
}
