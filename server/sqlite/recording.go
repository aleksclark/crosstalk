package sqlite

import (
	"context"
	"database/sql"
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
	var endedAt *string
	if r.EndedAt != nil {
		v := r.EndedAt.Format(time.RFC3339)
		endedAt = &v
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO recordings (id, session_id, source_id, channel_id, file_path, started_at, ended_at, size_bytes)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.SessionID, r.SourceID, r.ChannelID, r.FilePath,
		r.StartedAt.Format(time.RFC3339), endedAt, r.SizeBytes)
	return err
}

func (s *RecordingStore) FindBySession(ctx context.Context, sessionID string) ([]crosstalk.Recording, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, source_id, channel_id, file_path, started_at, ended_at, size_bytes
		 FROM recordings WHERE session_id = ? ORDER BY started_at DESC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recordings []crosstalk.Recording
	for rows.Next() {
		r, err := scanRecording(rows)
		if err != nil {
			return nil, err
		}
		recordings = append(recordings, r)
	}
	return recordings, rows.Err()
}

func (s *RecordingStore) FindByID(ctx context.Context, id string) (*crosstalk.Recording, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, session_id, source_id, channel_id, file_path, started_at, ended_at, size_bytes
		 FROM recordings WHERE id = ?`, id)

	var r crosstalk.Recording
	var sourceID, channelID, endedAt sql.NullString
	var startedAt string
	err := row.Scan(&r.ID, &r.SessionID, &sourceID, &channelID, &r.FilePath, &startedAt, &endedAt, &r.SizeBytes)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("recording not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	if sourceID.Valid {
		r.SourceID = &sourceID.String
	}
	if channelID.Valid {
		r.ChannelID = &channelID.String
	}
	r.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
	if endedAt.Valid {
		t, _ := time.Parse(time.RFC3339, endedAt.String)
		r.EndedAt = &t
	}
	return &r, nil
}

func (s *RecordingStore) Update(ctx context.Context, r *crosstalk.Recording) error {
	var endedAt *string
	if r.EndedAt != nil {
		v := r.EndedAt.Format(time.RFC3339)
		endedAt = &v
	}
	result, err := s.db.ExecContext(ctx,
		`UPDATE recordings SET ended_at = ?, size_bytes = ? WHERE id = ?`,
		endedAt, r.SizeBytes, r.ID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("recording not found: %s", r.ID)
	}
	return nil
}

func (s *RecordingStore) List(ctx context.Context) ([]crosstalk.Recording, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, source_id, channel_id, file_path, started_at, ended_at, size_bytes
		 FROM recordings ORDER BY started_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recordings []crosstalk.Recording
	for rows.Next() {
		r, err := scanRecording(rows)
		if err != nil {
			return nil, err
		}
		recordings = append(recordings, r)
	}
	return recordings, rows.Err()
}

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanRecording(row scanner) (crosstalk.Recording, error) {
	var r crosstalk.Recording
	var sourceID, channelID, endedAt sql.NullString
	var startedAt string
	err := row.Scan(&r.ID, &r.SessionID, &sourceID, &channelID, &r.FilePath, &startedAt, &endedAt, &r.SizeBytes)
	if err != nil {
		return r, err
	}
	if sourceID.Valid {
		r.SourceID = &sourceID.String
	}
	if channelID.Valid {
		r.ChannelID = &channelID.String
	}
	r.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
	if endedAt.Valid {
		t, _ := time.Parse(time.RFC3339, endedAt.String)
		r.EndedAt = &t
	}
	return r, nil
}
