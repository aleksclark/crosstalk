package postgres

import (
	"context"
	"database/sql"
	"errors"
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
	m := &refreshTokenModel{
		ID:        rt.ID,
		UserID:    rt.UserID,
		TokenHash: rt.TokenHash,
		ExpiresAt: rt.ExpiresAt,
		CreatedAt: rt.CreatedAt,
	}
	_, err := s.db.NewInsert().Model(m).Exec(ctx)
	return err
}

func (s *RefreshTokenStore) GetByHash(ctx context.Context, hash string) (*crosstalk.RefreshToken, error) {
	m := new(refreshTokenModel)
	err := s.db.NewSelect().Model(m).Where("token_hash = ?", hash).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("refresh token not found")
	}
	if err != nil {
		return nil, err
	}
	out := m.toDomain()
	return &out, nil
}

func (s *RefreshTokenStore) DeleteByHash(ctx context.Context, hash string) error {
	_, err := s.db.NewDelete().Model((*refreshTokenModel)(nil)).Where("token_hash = ?", hash).Exec(ctx)
	return err
}

func (s *RefreshTokenStore) DeleteByUserID(ctx context.Context, userID string) error {
	_, err := s.db.NewDelete().Model((*refreshTokenModel)(nil)).Where("user_id = ?", userID).Exec(ctx)
	return err
}
