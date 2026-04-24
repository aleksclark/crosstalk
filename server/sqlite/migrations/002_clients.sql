-- +goose Up
CREATE TABLE clients (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    owner_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_name TEXT NOT NULL DEFAULT '',
    sink_name   TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE INDEX idx_clients_owner ON clients(owner_id);

ALTER TABLE api_tokens ADD COLUMN client_id TEXT REFERENCES clients(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE api_tokens DROP COLUMN client_id;
DROP TABLE clients;
