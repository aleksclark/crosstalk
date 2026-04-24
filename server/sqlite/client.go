package sqlite

import (
	"database/sql"
	"fmt"

	crosstalk "github.com/aleksclark/crosstalk/server"
)

// ClientService implements crosstalk.ClientService backed by SQLite.
type ClientService struct {
	DB *sql.DB
}

func (s *ClientService) CreateClient(c *crosstalk.Client) error {
	_, err := s.DB.Exec(
		`INSERT INTO clients (id, name, owner_id, source_name, sink_name, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.Name, c.OwnerID, c.SourceName, c.SinkName,
		c.CreatedAt.Format(timeFormat), c.UpdatedAt.Format(timeFormat),
	)
	if err != nil {
		return fmt.Errorf("sqlite create client: %w", err)
	}
	return nil
}

func (s *ClientService) FindClientByID(id string) (*crosstalk.Client, error) {
	row := s.DB.QueryRow(
		`SELECT id, name, owner_id, source_name, sink_name, created_at, updated_at
		 FROM clients WHERE id = ?`, id,
	)
	return scanClient(row)
}

func (s *ClientService) ListClients() ([]crosstalk.Client, error) {
	return s.queryClients(`SELECT id, name, owner_id, source_name, sink_name, created_at, updated_at
		FROM clients ORDER BY created_at`)
}

func (s *ClientService) ListClientsByOwner(ownerID string) ([]crosstalk.Client, error) {
	return s.queryClients(`SELECT id, name, owner_id, source_name, sink_name, created_at, updated_at
		FROM clients WHERE owner_id = ? ORDER BY created_at`, ownerID)
}

func (s *ClientService) UpdateClient(c *crosstalk.Client) error {
	result, err := s.DB.Exec(
		`UPDATE clients SET name = ?, source_name = ?, sink_name = ?, updated_at = ? WHERE id = ?`,
		c.Name, c.SourceName, c.SinkName, c.UpdatedAt.Format(timeFormat), c.ID,
	)
	if err != nil {
		return fmt.Errorf("sqlite update client: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *ClientService) DeleteClient(id string) error {
	result, err := s.DB.Exec(`DELETE FROM clients WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("sqlite delete client: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *ClientService) queryClients(query string, args ...any) ([]crosstalk.Client, error) {
	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite query clients: %w", err)
	}
	defer rows.Close()

	var clients []crosstalk.Client
	for rows.Next() {
		var c crosstalk.Client
		var createdAt, updatedAt string
		if err := rows.Scan(&c.ID, &c.Name, &c.OwnerID, &c.SourceName, &c.SinkName, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("sqlite scan client: %w", err)
		}
		c.CreatedAt, _ = parseTime(createdAt)
		c.UpdatedAt, _ = parseTime(updatedAt)
		clients = append(clients, c)
	}
	return clients, rows.Err()
}

func scanClient(row *sql.Row) (*crosstalk.Client, error) {
	var c crosstalk.Client
	var createdAt, updatedAt string
	if err := row.Scan(&c.ID, &c.Name, &c.OwnerID, &c.SourceName, &c.SinkName, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	c.CreatedAt, _ = parseTime(createdAt)
	c.UpdatedAt, _ = parseTime(updatedAt)
	return &c, nil
}
