package sqlite

import (
	"database/sql"
	"fmt"

	crosstalk "github.com/aleksclark/crosstalk/server"
)

// UserService implements crosstalk.UserService backed by SQLite.
type UserService struct {
	DB *sql.DB
}

func (s *UserService) CreateUser(user *crosstalk.User) error {
	_, err := s.DB.Exec(
		`INSERT INTO users (id, username, password_hash, created_at) VALUES (?, ?, ?, ?)`,
		user.ID, user.Username, user.PasswordHash, user.CreatedAt.Format(timeFormat),
	)
	if err != nil {
		return fmt.Errorf("sqlite create user: %w", err)
	}
	return nil
}

func (s *UserService) FindUserByID(id string) (*crosstalk.User, error) {
	row := s.DB.QueryRow(
		`SELECT id, username, password_hash, created_at FROM users WHERE id = ?`, id,
	)
	return scanUser(row)
}

func (s *UserService) FindUserByUsername(username string) (*crosstalk.User, error) {
	row := s.DB.QueryRow(
		`SELECT id, username, password_hash, created_at FROM users WHERE username = ?`, username,
	)
	return scanUser(row)
}

func (s *UserService) ListUsers() ([]crosstalk.User, error) {
	rows, err := s.DB.Query(`SELECT id, username, password_hash, created_at FROM users ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("sqlite list users: %w", err)
	}
	defer rows.Close()
	var users []crosstalk.User
	for rows.Next() {
		var u crosstalk.User
		var createdAt string
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &createdAt); err != nil {
			return nil, fmt.Errorf("sqlite scan user: %w", err)
		}
		t, err := parseTime(createdAt)
		if err != nil {
			return nil, fmt.Errorf("sqlite parse user created_at: %w", err)
		}
		u.CreatedAt = t
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *UserService) UpdateUser(user *crosstalk.User) error {
	result, err := s.DB.Exec(
		`UPDATE users SET username = ? WHERE id = ?`,
		user.Username, user.ID,
	)
	if err != nil {
		return fmt.Errorf("sqlite update user: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite update user rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *UserService) DeleteUser(id string) error {
	result, err := s.DB.Exec(`DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("sqlite delete user: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite delete user rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func scanUser(row *sql.Row) (*crosstalk.User, error) {
	var u crosstalk.User
	var createdAt string
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &createdAt); err != nil {
		return nil, err
	}
	t, err := parseTime(createdAt)
	if err != nil {
		return nil, fmt.Errorf("sqlite parse user created_at: %w", err)
	}
	u.CreatedAt = t
	return &u, nil
}
