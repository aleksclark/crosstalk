package sqlite

import (
	"database/sql"
	"fmt"
	"time"

	crosstalk "github.com/aleksclark/crosstalk/server"
)

// SessionService implements crosstalk.SessionService backed by SQLite.
type SessionService struct {
	DB *sql.DB
}

func (s *SessionService) CreateSession(session *crosstalk.Session) error {
	var endedAt *string
	if session.EndedAt != nil {
		v := session.EndedAt.Format(timeFormat)
		endedAt = &v
	}

	_, err := s.DB.Exec(
		`INSERT INTO sessions (id, template_id, name, status, created_at, ended_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		session.ID, session.TemplateID, session.Name, string(session.Status),
		session.CreatedAt.Format(timeFormat), endedAt,
	)
	if err != nil {
		return fmt.Errorf("sqlite create session: %w", err)
	}
	return nil
}

func (s *SessionService) FindSessionByID(id string) (*crosstalk.Session, error) {
	row := s.DB.QueryRow(
		`SELECT id, template_id, name, status, created_at, ended_at
		 FROM sessions WHERE id = ?`, id,
	)
	return scanSession(row)
}

func (s *SessionService) ListSessions() ([]crosstalk.Session, error) {
	rows, err := s.DB.Query(
		`SELECT id, template_id, name, status, created_at, ended_at
		 FROM sessions ORDER BY created_at`,
	)
	if err != nil {
		return nil, fmt.Errorf("sqlite list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []crosstalk.Session
	for rows.Next() {
		var sess crosstalk.Session
		var status, createdAt string
		var endedAt *string
		if err := rows.Scan(&sess.ID, &sess.TemplateID, &sess.Name, &status, &createdAt, &endedAt); err != nil {
			return nil, fmt.Errorf("sqlite scan session: %w", err)
		}
		sess.Status = crosstalk.SessionStatus(status)
		parsed, err := parseTime(createdAt)
		if err != nil {
			return nil, fmt.Errorf("sqlite parse session created_at: %w", err)
		}
		sess.CreatedAt = parsed
		if endedAt != nil {
			t, err := parseTime(*endedAt)
			if err != nil {
				return nil, fmt.Errorf("sqlite parse session ended_at: %w", err)
			}
			sess.EndedAt = &t
		}
		sessions = append(sessions, sess)
	}
	return sessions, rows.Err()
}

func (s *SessionService) UpdateSessionStatus(id string, status crosstalk.SessionStatus) error {
	result, err := s.DB.Exec(
		`UPDATE sessions SET status = ? WHERE id = ?`,
		string(status), id,
	)
	if err != nil {
		return fmt.Errorf("sqlite update session status: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite update session status rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *SessionService) EndSession(id string) error {
	now := time.Now().UTC().Format(timeFormat)
	result, err := s.DB.Exec(
		`UPDATE sessions SET status = ?, ended_at = ? WHERE id = ?`,
		string(crosstalk.SessionEnded), now, id,
	)
	if err != nil {
		return fmt.Errorf("sqlite end session: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite end session rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func scanSession(row *sql.Row) (*crosstalk.Session, error) {
	var sess crosstalk.Session
	var status, createdAt string
	var endedAt *string
	if err := row.Scan(&sess.ID, &sess.TemplateID, &sess.Name, &status, &createdAt, &endedAt); err != nil {
		return nil, err
	}
	sess.Status = crosstalk.SessionStatus(status)
	parsed, err := parseTime(createdAt)
	if err != nil {
		return nil, fmt.Errorf("sqlite parse session created_at: %w", err)
	}
	sess.CreatedAt = parsed
	if endedAt != nil {
		t, err := parseTime(*endedAt)
		if err != nil {
			return nil, fmt.Errorf("sqlite parse session ended_at: %w", err)
		}
		sess.EndedAt = &t
	}
	return &sess, nil
}
