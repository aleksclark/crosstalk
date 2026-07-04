package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	crosstalk "github.com/aleksclark/crosstalk/server"
)

// UserStore implements crosstalk.UserService.
type UserStore struct {
	db *DB
}

func NewUserStore(db *DB) *UserStore {
	return &UserStore{db: db}
}

func (s *UserStore) List(ctx context.Context) ([]crosstalk.User, error) {
	var models []userModel
	err := s.db.NewSelect().Model(&models).Order("created_at DESC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return usersToDomain(models), nil
}

func (s *UserStore) Get(ctx context.Context, id string) (*crosstalk.User, error) {
	m := new(userModel)
	err := s.db.NewSelect().Model(m).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("user not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	out := m.toDomain()
	return &out, nil
}

func (s *UserStore) GetByUsername(ctx context.Context, username string) (*crosstalk.User, error) {
	m := new(userModel)
	err := s.db.NewSelect().Model(m).Where("username = ?", username).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("user not found: %s", username)
	}
	if err != nil {
		return nil, err
	}
	out := m.toDomain()
	return &out, nil
}

func (s *UserStore) Create(ctx context.Context, u *crosstalk.User) error {
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now().UTC()
	}
	m := &userModel{
		ID:           u.ID,
		Username:     u.Username,
		PasswordHash: u.PasswordHash,
		Role:         u.Role,
		CreatedAt:    u.CreatedAt,
	}
	_, err := s.db.NewInsert().Model(m).Exec(ctx)
	return err
}

func (s *UserStore) Update(ctx context.Context, u *crosstalk.User) error {
	res, err := s.db.NewUpdate().Model((*userModel)(nil)).
		Set("username = ?", u.Username).
		Set("password_hash = ?", u.PasswordHash).
		Set("role = ?", u.Role).
		Where("id = ?", u.ID).
		Exec(ctx)
	if err != nil {
		return err
	}
	return requireAffected(res, "user", u.ID)
}

func (s *UserStore) Delete(ctx context.Context, id string) error {
	res, err := s.db.NewDelete().Model((*userModel)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return err
	}
	return requireAffected(res, "user", id)
}

func (s *UserStore) ListByRole(ctx context.Context, role string) ([]crosstalk.User, error) {
	var models []userModel
	err := s.db.NewSelect().Model(&models).
		Where("role = ?", role).
		Order("created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return usersToDomain(models), nil
}

func (s *UserStore) AssignSessions(ctx context.Context, translatorID string, sessionIDs []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.NewDelete().Model((*translatorSessionModel)(nil)).
		Where("translator_id = ?", translatorID).Exec(ctx); err != nil {
		return err
	}

	for _, sid := range sessionIDs {
		m := &translatorSessionModel{TranslatorID: translatorID, SessionID: sid}
		if _, err := tx.NewInsert().Model(m).Exec(ctx); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *UserStore) GetAssignedSessions(ctx context.Context, translatorID string) ([]string, error) {
	var ids []string
	err := s.db.NewSelect().Model((*translatorSessionModel)(nil)).
		Column("session_id").
		Where("translator_id = ?", translatorID).
		Scan(ctx, &ids)
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func usersToDomain(models []userModel) []crosstalk.User {
	out := make([]crosstalk.User, 0, len(models))
	for i := range models {
		out = append(out, models[i].toDomain())
	}
	return out
}
