# Management

[← Back to Index](../index.md) · [Admin Web Overview](overview.md)

---

## API Tokens (`/tokens`)

| Action | Description |
|--------|-------------|
| List | Table of all tokens (name, created date, last used) |
| Create | Name input → generates token → shows plaintext ONCE |
| Revoke | Delete button with confirmation |

Tokens cannot be edited after creation (only created or revoked).

## Users (`/users`)

| Action | Description |
|--------|-------------|
| List | Table of users (username, created date) |
| Create | Username + password form |
| Edit | Change password |
| Delete | With confirmation, prevents deleting last admin |

## Session Templates (`/templates`)

### List View

Table of templates with name, role count, mapping count, default flag.

### Editor View (`/templates/:id`)

- **Name** — text input
- **Default** — toggle (at most one template can be default)
- **Roles** — editable list of role definitions (add/remove):
  - Name (text input)
  - Multi-client toggle (boolean — whether multiple clients can fill this role)
- **Mappings** — visual editor for channel mappings:
  ```
  [role dropdown]:[channel name] → [role dropdown]:[channel name]
                                    or → record
                                    or → broadcast
  ```
- Add/remove mapping rows
- Validation:
  - All roles referenced in mappings must exist in the roles list
  - Multi-client roles cannot appear as mapping sources (error shown inline)

### Example: "Translation" Template

```
Roles:
  translator  (single-client)
  studio      (single-client)

Mappings:
  translator:mic     → studio:output
  studio:input       → translator:speakers
  translator:mic     → broadcast
  translator:mic     → record
  studio:input       → record
```

## Sessions (`/sessions`)

### List View

| Column | Description |
|--------|-------------|
| Name | Session name |
| Template | Template used |
| Status | waiting / active / ended |
| Clients | Connected client count / total roles |
| Created | Timestamp |
| Actions | Connect, End |

### Detail View (`/sessions/:id`)

- Session metadata (template, status, created/ended times)
- Connected clients per role (with status indicators)
- Channel binding status (which mappings are active)
- "Connect" button → navigates to session connect view
- "End Session" button with confirmation
