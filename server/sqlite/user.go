package sqlite

import (
	"context"
	"database/sql"
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
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, username, password_hash, role, created_at FROM users ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []crosstalk.User
	for rows.Next() {
		var u crosstalk.User
		var createdAt string
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &createdAt); err != nil {
			return nil, err
		}
		u.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *UserStore) Get(ctx context.Context, id string) (*crosstalk.User, error) {
	var u crosstalk.User
	var createdAt string
	err := s.db.QueryRowContext(ctx,
		"SELECT id, username, password_hash, role, created_at FROM users WHERE id = ?", id).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &createdAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	u.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &u, nil
}

func (s *UserStore) GetByUsername(ctx context.Context, username string) (*crosstalk.User, error) {
	var u crosstalk.User
	var createdAt string
	err := s.db.QueryRowContext(ctx,
		"SELECT id, username, password_hash, role, created_at FROM users WHERE username = ?", username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &createdAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found: %s", username)
	}
	if err != nil {
		return nil, err
	}
	u.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &u, nil
}

func (s *UserStore) Create(ctx context.Context, u *crosstalk.User) error {
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO users (id, username, password_hash, role, created_at) VALUES (?, ?, ?, ?, ?)",
		u.ID, u.Username, u.PasswordHash, u.Role, u.CreatedAt.Format(time.RFC3339))
	return err
}

func (s *UserStore) Update(ctx context.Context, u *crosstalk.User) error {
	result, err := s.db.ExecContext(ctx,
		"UPDATE users SET username = ?, password_hash = ?, role = ? WHERE id = ?",
		u.Username, u.PasswordHash, u.Role, u.ID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("user not found: %s", u.ID)
	}
	return nil
}

func (s *UserStore) Delete(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM users WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("user not found: %s", id)
	}
	return nil
}

func (s *UserStore) ListByRole(ctx context.Context, role string) ([]crosstalk.User, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, username, password_hash, role, created_at FROM users WHERE role = ? ORDER BY created_at DESC", role)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []crosstalk.User
	for rows.Next() {
		var u crosstalk.User
		var createdAt string
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &createdAt); err != nil {
			return nil, err
		}
		u.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *UserStore) AssignSessions(ctx context.Context, translatorID string, sessionIDs []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Clear existing assignments
	if _, err := tx.ExecContext(ctx, "DELETE FROM translator_sessions WHERE translator_id = ?", translatorID); err != nil {
		return err
	}

	// Insert new assignments
	for _, sid := range sessionIDs {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO translator_sessions (translator_id, session_id) VALUES (?, ?)",
			translatorID, sid); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *UserStore) GetAssignedSessions(ctx context.Context, translatorID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT session_id FROM translator_sessions WHERE translator_id = ?", translatorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
