# Persistence

[← Back to Index](../index.md) · [Server Overview](overview.md)

---

## Overview

SQLite for all durable state. [Goose](https://github.com/pressly/goose) for schema migrations.

## Schema (Conceptual)

### users
| Column | Type | Notes |
|--------|------|-------|
| id | TEXT (ULID) | Primary key |
| username | TEXT | Unique |
| password_hash | TEXT | bcrypt |
| created_at | DATETIME | |

### api_tokens
| Column | Type | Notes |
|--------|------|-------|
| id | TEXT (ULID) | Primary key |
| name | TEXT | Human-readable label |
| token_hash | TEXT | SHA-256 of plaintext token |
| user_id | TEXT | FK → users |
| created_at | DATETIME | |

### session_templates
| Column | Type | Notes |
|--------|------|-------|
| id | TEXT (ULID) | Primary key |
| name | TEXT | |
| roles | JSON | Array of role objects `[{name, multi_client}]` |
| mappings | JSON | Array of channel mapping objects |
| is_default | BOOLEAN | At most one template flagged as default |
| created_at | DATETIME | |
| updated_at | DATETIME | |

### sessions
| Column | Type | Notes |
|--------|------|-------|
| id | TEXT (ULID) | Primary key |
| template_id | TEXT | FK → session_templates |
| name | TEXT | |
| status | TEXT | `waiting`, `active`, `ended` |
| created_at | DATETIME | |
| ended_at | DATETIME | Nullable |

### session_clients
| Column | Type | Notes |
|--------|------|-------|
| id | TEXT (ULID) | Primary key |
| session_id | TEXT | FK → sessions |
| role | TEXT | Role name from template |
| client_identifier | TEXT | WebRTC connection identifier |
| status | TEXT | `connected`, `disconnected` |
| connected_at | DATETIME | |
| disconnected_at | DATETIME | Nullable |

## Migration Strategy

- Goose with SQL migration files in `server/sqlite/migrations/`
- Migrations run automatically on server startup
- All migrations are forward-only (no down migrations) to keep things simple
- SQLite WAL mode enabled for concurrent read access

## Notes

- ULIDs for primary keys (sortable, no coordination needed)
- JSON columns for flexible schema in templates (roles, mappings)
- Session client records are append-only — reconnections create new rows
- Indexes on `sessions.status` and `session_clients.session_id` for common queries
