package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	crosstalk "github.com/aleksclark/crosstalk/server"
)

// SessionStore implements crosstalk.SessionService.
type SessionStore struct {
	db *DB
}

func NewSessionStore(db *DB) *SessionStore {
	return &SessionStore{db: db}
}

func (s *SessionStore) List(ctx context.Context) ([]crosstalk.Session, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, name, description, broadcast_token, created_at, updated_at FROM sessions ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []crosstalk.Session
	for rows.Next() {
		var sess crosstalk.Session
		var broadcastToken sql.NullString
		var createdAt, updatedAt string
		if err := rows.Scan(&sess.ID, &sess.Name, &sess.Description, &broadcastToken, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		sess.BroadcastToken = broadcastToken.String
		sess.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		sess.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		sessions = append(sessions, sess)
	}
	return sessions, rows.Err()
}

func (s *SessionStore) Get(ctx context.Context, id string) (*crosstalk.Session, error) {
	var sess crosstalk.Session
	var broadcastToken sql.NullString
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(ctx,
		"SELECT id, name, description, broadcast_token, created_at, updated_at FROM sessions WHERE id = ?", id).
		Scan(&sess.ID, &sess.Name, &sess.Description, &broadcastToken, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	sess.BroadcastToken = broadcastToken.String
	sess.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	sess.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &sess, nil
}

func (s *SessionStore) Create(ctx context.Context, sess *crosstalk.Session) error {
	if sess.BroadcastToken == "" {
		sess.BroadcastToken = generateToken()
	}
	now := time.Now().UTC()
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = now
	}
	if sess.UpdatedAt.IsZero() {
		sess.UpdatedAt = now
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO sessions (id, name, description, broadcast_token, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		sess.ID, sess.Name, sess.Description, sess.BroadcastToken,
		sess.CreatedAt.Format(time.RFC3339), sess.UpdatedAt.Format(time.RFC3339))
	return err
}

func (s *SessionStore) Update(ctx context.Context, sess *crosstalk.Session) error {
	sess.UpdatedAt = time.Now().UTC()
	result, err := s.db.ExecContext(ctx,
		"UPDATE sessions SET name = ?, description = ?, updated_at = ? WHERE id = ?",
		sess.Name, sess.Description, sess.UpdatedAt.Format(time.RFC3339), sess.ID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("session not found: %s", sess.ID)
	}
	return nil
}

func (s *SessionStore) Delete(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("session not found: %s", id)
	}
	return nil
}

func (s *SessionStore) GetByBroadcastToken(ctx context.Context, token string) (*crosstalk.Session, error) {
	var sess crosstalk.Session
	var broadcastToken sql.NullString
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(ctx,
		"SELECT id, name, description, broadcast_token, created_at, updated_at FROM sessions WHERE broadcast_token = ?", token).
		Scan(&sess.ID, &sess.Name, &sess.Description, &broadcastToken, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found for broadcast token")
	}
	if err != nil {
		return nil, err
	}
	sess.BroadcastToken = broadcastToken.String
	sess.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	sess.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return &sess, nil
}

func (s *SessionStore) RegenerateBroadcastToken(ctx context.Context, id string) (string, error) {
	token := generateToken()
	result, err := s.db.ExecContext(ctx,
		"UPDATE sessions SET broadcast_token = ?, updated_at = ? WHERE id = ?",
		token, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return "", err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return "", fmt.Errorf("session not found: %s", id)
	}
	return token, nil
}

func generateToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
