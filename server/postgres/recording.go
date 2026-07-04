package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	crosstalk "github.com/aleksclark/crosstalk/server"
)

// RecordingStore implements crosstalk.RecordingService.
type RecordingStore struct {
	db *DB
}

func NewRecordingStore(db *DB) *RecordingStore {
	return &RecordingStore{db: db}
}

func (s *RecordingStore) Create(ctx context.Context, r *crosstalk.Recording) error {
	if r.StartedAt.IsZero() {
		r.StartedAt = time.Now().UTC()
	}
	m := &recordingModel{
		ID:        r.ID,
		SessionID: r.SessionID,
		SourceID:  r.SourceID,
		ChannelID: r.ChannelID,
		FilePath:  r.FilePath,
		StartedAt: r.StartedAt,
		EndedAt:   r.EndedAt,
		SizeBytes: r.SizeBytes,
	}
	_, err := s.db.NewInsert().Model(m).Exec(ctx)
	return err
}

func (s *RecordingStore) FindBySession(ctx context.Context, sessionID string) ([]crosstalk.Recording, error) {
	var models []recordingModel
	err := s.db.NewSelect().Model(&models).
		Where("session_id = ?", sessionID).
		Order("started_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return recordingsToDomain(models), nil
}

func (s *RecordingStore) FindByID(ctx context.Context, id string) (*crosstalk.Recording, error) {
	m := new(recordingModel)
	err := s.db.NewSelect().Model(m).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("recording not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	out := m.toDomain()
	return &out, nil
}

func (s *RecordingStore) Update(ctx context.Context, r *crosstalk.Recording) error {
	res, err := s.db.NewUpdate().Model((*recordingModel)(nil)).
		Set("ended_at = ?", r.EndedAt).
		Set("size_bytes = ?", r.SizeBytes).
		Where("id = ?", r.ID).
		Exec(ctx)
	if err != nil {
		return err
	}
	return requireAffected(res, "recording", r.ID)
}

func (s *RecordingStore) List(ctx context.Context) ([]crosstalk.Recording, error) {
	var models []recordingModel
	err := s.db.NewSelect().Model(&models).Order("started_at DESC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return recordingsToDomain(models), nil
}

func recordingsToDomain(models []recordingModel) []crosstalk.Recording {
	out := make([]crosstalk.Recording, 0, len(models))
	for i := range models {
		out = append(out, models[i].toDomain())
	}
	return out
}
