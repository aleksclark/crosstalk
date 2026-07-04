package postgres

import (
	"context"
	"database/sql"
	"errors"
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
	var models []sessionModel
	err := s.db.NewSelect().Model(&models).Order("created_at DESC").Scan(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]crosstalk.Session, 0, len(models))
	for i := range models {
		out = append(out, models[i].toDomain())
	}
	return out, nil
}

func (s *SessionStore) Get(ctx context.Context, id string) (*crosstalk.Session, error) {
	m := new(sessionModel)
	err := s.db.NewSelect().Model(m).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("session not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	out := m.toDomain()
	return &out, nil
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
	_, err := s.db.NewInsert().Model(sessionFromDomain(sess)).Exec(ctx)
	return err
}

func (s *SessionStore) Update(ctx context.Context, sess *crosstalk.Session) error {
	sess.UpdatedAt = time.Now().UTC()
	res, err := s.db.NewUpdate().Model((*sessionModel)(nil)).
		Set("name = ?", sess.Name).
		Set("description = ?", sess.Description).
		Set("updated_at = ?", sess.UpdatedAt).
		Where("id = ?", sess.ID).
		Exec(ctx)
	if err != nil {
		return err
	}
	return requireAffected(res, "session", sess.ID)
}

func (s *SessionStore) Delete(ctx context.Context, id string) error {
	res, err := s.db.NewDelete().Model((*sessionModel)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return err
	}
	return requireAffected(res, "session", id)
}

func (s *SessionStore) GetByBroadcastToken(ctx context.Context, token string) (*crosstalk.Session, error) {
	m := new(sessionModel)
	err := s.db.NewSelect().Model(m).Where("broadcast_token = ?", token).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("session not found for broadcast token")
	}
	if err != nil {
		return nil, err
	}
	out := m.toDomain()
	return &out, nil
}

func (s *SessionStore) RegenerateBroadcastToken(ctx context.Context, id string) (string, error) {
	token := generateToken()
	res, err := s.db.NewUpdate().Model((*sessionModel)(nil)).
		Set("broadcast_token = ?", token).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return "", err
	}
	if err := requireAffected(res, "session", id); err != nil {
		return "", err
	}
	return token, nil
}

func requireAffected(res sql.Result, entity, id string) error {
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%s not found: %s", entity, id)
	}
	return nil
}
