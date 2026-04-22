package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"

	crosstalk "github.com/anthropics/crosstalk/server"
)

// SessionTemplateService implements crosstalk.SessionTemplateService backed by SQLite.
type SessionTemplateService struct {
	DB *sql.DB
}

func (s *SessionTemplateService) CreateTemplate(tmpl *crosstalk.SessionTemplate) error {
	roles, err := json.Marshal(tmpl.Roles)
	if err != nil {
		return fmt.Errorf("sqlite marshal roles: %w", err)
	}
	mappings, err := json.Marshal(tmpl.Mappings)
	if err != nil {
		return fmt.Errorf("sqlite marshal mappings: %w", err)
	}

	_, err = s.DB.Exec(
		`INSERT INTO session_templates (id, name, is_default, roles, mappings, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		tmpl.ID, tmpl.Name, boolToInt(tmpl.IsDefault), string(roles), string(mappings),
		tmpl.CreatedAt.Format(timeFormat), tmpl.UpdatedAt.Format(timeFormat),
	)
	if err != nil {
		return fmt.Errorf("sqlite create template: %w", err)
	}
	return nil
}

func (s *SessionTemplateService) FindTemplateByID(id string) (*crosstalk.SessionTemplate, error) {
	row := s.DB.QueryRow(
		`SELECT id, name, is_default, roles, mappings, created_at, updated_at
		 FROM session_templates WHERE id = ?`, id,
	)
	return scanTemplate(row)
}

func (s *SessionTemplateService) ListTemplates() ([]crosstalk.SessionTemplate, error) {
	rows, err := s.DB.Query(
		`SELECT id, name, is_default, roles, mappings, created_at, updated_at
		 FROM session_templates ORDER BY created_at`,
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite list templates: %w", err)
	}
	defer rows.Close()

	var templates []crosstalk.SessionTemplate
	for rows.Next() {
		var t crosstalk.SessionTemplate
		var isDefault int
		var rolesJSON, mappingsJSON, createdAt, updatedAt string
		if err := rows.Scan(&t.ID, &t.Name, &isDefault, &rolesJSON, &mappingsJSON, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("sqlite scan template: %w", err)
		}
		t.IsDefault = isDefault != 0
		if err := json.Unmarshal([]byte(rolesJSON), &t.Roles); err != nil {
			return nil, fmt.Errorf("sqlite unmarshal roles: %w", err)
		}
		if err := json.Unmarshal([]byte(mappingsJSON), &t.Mappings); err != nil {
			return nil, fmt.Errorf("sqlite unmarshal mappings: %w", err)
		}
		parsed, err := parseTime(createdAt)
		if err != nil {
			return nil, fmt.Errorf("sqlite parse template created_at: %w", err)
		}
		t.CreatedAt = parsed
		parsed, err = parseTime(updatedAt)
		if err != nil {
			return nil, fmt.Errorf("sqlite parse template updated_at: %w", err)
		}
		t.UpdatedAt = parsed
		templates = append(templates, t)
	}
	return templates, rows.Err()
}

func (s *SessionTemplateService) UpdateTemplate(tmpl *crosstalk.SessionTemplate) error {
	roles, err := json.Marshal(tmpl.Roles)
	if err != nil {
		return fmt.Errorf("sqlite marshal roles: %w", err)
	}
	mappings, err := json.Marshal(tmpl.Mappings)
	if err != nil {
		return fmt.Errorf("sqlite marshal mappings: %w", err)
	}

	result, err := s.DB.Exec(
		`UPDATE session_templates SET name = ?, is_default = ?, roles = ?, mappings = ?, updated_at = ?
		 WHERE id = ?`,
		tmpl.Name, boolToInt(tmpl.IsDefault), string(roles), string(mappings),
		tmpl.UpdatedAt.Format(timeFormat), tmpl.ID,
	)
	if err != nil {
		return fmt.Errorf("sqlite update template: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite update template rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *SessionTemplateService) DeleteTemplate(id string) error {
	result, err := s.DB.Exec(`DELETE FROM session_templates WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("sqlite delete template: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite delete template rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *SessionTemplateService) FindDefaultTemplate() (*crosstalk.SessionTemplate, error) {
	row := s.DB.QueryRow(
		`SELECT id, name, is_default, roles, mappings, created_at, updated_at
		 FROM session_templates WHERE is_default = 1 LIMIT 1`,
	)
	return scanTemplate(row)
}

func scanTemplate(row *sql.Row) (*crosstalk.SessionTemplate, error) {
	var t crosstalk.SessionTemplate
	var isDefault int
	var rolesJSON, mappingsJSON, createdAt, updatedAt string
	if err := row.Scan(&t.ID, &t.Name, &isDefault, &rolesJSON, &mappingsJSON, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	t.IsDefault = isDefault != 0
	if err := json.Unmarshal([]byte(rolesJSON), &t.Roles); err != nil {
		return nil, fmt.Errorf("sqlite unmarshal roles: %w", err)
	}
	if err := json.Unmarshal([]byte(mappingsJSON), &t.Mappings); err != nil {
		return nil, fmt.Errorf("sqlite unmarshal mappings: %w", err)
	}
	parsed, err := parseTime(createdAt)
	if err != nil {
		return nil, fmt.Errorf("sqlite parse template created_at: %w", err)
	}
	t.CreatedAt = parsed
	parsed, err = parseTime(updatedAt)
	if err != nil {
		return nil, fmt.Errorf("sqlite parse template updated_at: %w", err)
	}
	t.UpdatedAt = parsed
	return &t, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
