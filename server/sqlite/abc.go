package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	crosstalk "github.com/nicosql/crosstalk/server"
)

// ABCStore implements crosstalk.ABCService.
type ABCStore struct {
	db *DB
}

func NewABCStore(db *DB) *ABCStore {
	return &ABCStore{db: db}
}

func (s *ABCStore) List(ctx context.Context) ([]crosstalk.ABC, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, name, token_hash, session_id, connected, last_seen, created_at FROM abcs ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var abcs []crosstalk.ABC
	for rows.Next() {
		var abc crosstalk.ABC
		var sessionID sql.NullString
		var lastSeen sql.NullString
		var createdAt string
		if err := rows.Scan(&abc.ID, &abc.Name, &abc.TokenHash, &sessionID, &abc.Connected, &lastSeen, &createdAt); err != nil {
			return nil, err
		}
		if sessionID.Valid {
			abc.SessionID = &sessionID.String
		}
		if lastSeen.Valid {
			t, _ := time.Parse(time.RFC3339, lastSeen.String)
			abc.LastSeen = &t
		}
		abc.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		abcs = append(abcs, abc)
	}
	return abcs, rows.Err()
}

func (s *ABCStore) Get(ctx context.Context, id string) (*crosstalk.ABC, error) {
	var abc crosstalk.ABC
	var sessionID sql.NullString
	var lastSeen sql.NullString
	var createdAt string
	err := s.db.QueryRowContext(ctx,
		"SELECT id, name, token_hash, session_id, connected, last_seen, created_at FROM abcs WHERE id = ?", id).
		Scan(&abc.ID, &abc.Name, &abc.TokenHash, &sessionID, &abc.Connected, &lastSeen, &createdAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("abc not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	if sessionID.Valid {
		abc.SessionID = &sessionID.String
	}
	if lastSeen.Valid {
		t, _ := time.Parse(time.RFC3339, lastSeen.String)
		abc.LastSeen = &t
	}
	abc.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &abc, nil
}

func (s *ABCStore) Create(ctx context.Context, abc *crosstalk.ABC) error {
	if abc.CreatedAt.IsZero() {
		abc.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO abcs (id, name, token_hash, session_id, connected, last_seen, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		abc.ID, abc.Name, abc.TokenHash, abc.SessionID, abc.Connected, nil, abc.CreatedAt.Format(time.RFC3339))
	return err
}

func (s *ABCStore) Update(ctx context.Context, abc *crosstalk.ABC) error {
	var lastSeen *string
	if abc.LastSeen != nil {
		s := abc.LastSeen.Format(time.RFC3339)
		lastSeen = &s
	}
	result, err := s.db.ExecContext(ctx,
		"UPDATE abcs SET name = ?, session_id = ?, connected = ?, last_seen = ? WHERE id = ?",
		abc.Name, abc.SessionID, abc.Connected, lastSeen, abc.ID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("abc not found: %s", abc.ID)
	}
	return nil
}

func (s *ABCStore) Delete(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM abcs WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("abc not found: %s", id)
	}
	return nil
}

func (s *ABCStore) GetByTokenHash(ctx context.Context, tokenHash string) (*crosstalk.ABC, error) {
	var abc crosstalk.ABC
	var sessionID sql.NullString
	var lastSeen sql.NullString
	var createdAt string
	err := s.db.QueryRowContext(ctx,
		"SELECT id, name, token_hash, session_id, connected, last_seen, created_at FROM abcs WHERE token_hash = ?", tokenHash).
		Scan(&abc.ID, &abc.Name, &abc.TokenHash, &sessionID, &abc.Connected, &lastSeen, &createdAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("abc not found for token")
	}
	if err != nil {
		return nil, err
	}
	if sessionID.Valid {
		abc.SessionID = &sessionID.String
	}
	if lastSeen.Valid {
		t, _ := time.Parse(time.RFC3339, lastSeen.String)
		abc.LastSeen = &t
	}
	abc.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &abc, nil
}
