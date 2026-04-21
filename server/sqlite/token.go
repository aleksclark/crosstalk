package sqlite

import (
	"database/sql"
	"fmt"

	crosstalk "github.com/anthropics/crosstalk/server"
)

// TokenService implements crosstalk.TokenService backed by SQLite.
type TokenService struct {
	DB *sql.DB
}

func (s *TokenService) CreateToken(token *crosstalk.APIToken) error {
	_, err := s.DB.Exec(
		`INSERT INTO api_tokens (id, name, token_hash, user_id, created_at) VALUES (?, ?, ?, ?, ?)`,
		token.ID, token.Name, token.TokenHash, token.UserID, token.CreatedAt.Format(timeFormat),
	)
	if err != nil {
		return fmt.Errorf("sqlite create token: %w", err)
	}
	return nil
}

func (s *TokenService) FindTokenByHash(hash string) (*crosstalk.APIToken, error) {
	row := s.DB.QueryRow(
		`SELECT id, name, token_hash, user_id, created_at FROM api_tokens WHERE token_hash = ?`, hash,
	)
	return scanToken(row)
}

func (s *TokenService) DeleteToken(id string) error {
	result, err := s.DB.Exec(`DELETE FROM api_tokens WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("sqlite delete token: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite delete token rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *TokenService) ListTokens() ([]crosstalk.APIToken, error) {
	rows, err := s.DB.Query(
		`SELECT id, name, token_hash, user_id, created_at FROM api_tokens ORDER BY created_at`,
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite list tokens: %w", err)
	}
	defer rows.Close()

	var tokens []crosstalk.APIToken
	for rows.Next() {
		var t crosstalk.APIToken
		var createdAt string
		if err := rows.Scan(&t.ID, &t.Name, &t.TokenHash, &t.UserID, &createdAt); err != nil {
			return nil, fmt.Errorf("sqlite scan token: %w", err)
		}
		parsed, err := parseTime(createdAt)
		if err != nil {
			return nil, fmt.Errorf("sqlite parse token created_at: %w", err)
		}
		t.CreatedAt = parsed
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

func scanToken(row *sql.Row) (*crosstalk.APIToken, error) {
	var t crosstalk.APIToken
	var createdAt string
	if err := row.Scan(&t.ID, &t.Name, &t.TokenHash, &t.UserID, &createdAt); err != nil {
		return nil, err
	}
	parsed, err := parseTime(createdAt)
	if err != nil {
		return nil, fmt.Errorf("sqlite parse token created_at: %w", err)
	}
	t.CreatedAt = parsed
	return &t, nil
}
