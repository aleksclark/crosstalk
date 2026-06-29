package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	crosstalk "github.com/aleksclark/crosstalk/server"
)

// RefreshTokenStore implements crosstalk.RefreshTokenService.
type RefreshTokenStore struct {
	db *DB
}

func NewRefreshTokenStore(db *DB) *RefreshTokenStore {
	return &RefreshTokenStore{db: db}
}

func (s *RefreshTokenStore) Create(ctx context.Context, rt *crosstalk.RefreshToken) error {
	if rt.CreatedAt.IsZero() {
		rt.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, created_at) VALUES (?, ?, ?, ?, ?)",
		rt.ID, rt.UserID, rt.TokenHash, rt.ExpiresAt.Format(time.RFC3339), rt.CreatedAt.Format(time.RFC3339))
	return err
}

func (s *RefreshTokenStore) GetByHash(ctx context.Context, hash string) (*crosstalk.RefreshToken, error) {
	var rt crosstalk.RefreshToken
	var expiresAt, createdAt string
	err := s.db.QueryRowContext(ctx,
		"SELECT id, user_id, token_hash, expires_at, created_at FROM refresh_tokens WHERE token_hash = ?", hash).
		Scan(&rt.ID, &rt.UserID, &rt.TokenHash, &expiresAt, &createdAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("refresh token not found")
	}
	if err != nil {
		return nil, err
	}
	rt.ExpiresAt, _ = time.Parse(time.RFC3339, expiresAt)
	rt.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &rt, nil
}

func (s *RefreshTokenStore) DeleteByHash(ctx context.Context, hash string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM refresh_tokens WHERE token_hash = ?", hash)
	return err
}

func (s *RefreshTokenStore) DeleteByUserID(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM refresh_tokens WHERE user_id = ?", userID)
	return err
}
