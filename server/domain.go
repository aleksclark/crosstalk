// Package crosstalk defines the core domain types and service interfaces for
// the CrossTalk server. This package contains no external dependencies.
//
// Implementations live in subpackages grouped by dependency:
//   - sqlite/   — SQLite-backed persistence
//   - http/     — REST API handlers (wraps net/http)
//   - ws/       — WebSocket signaling (wraps nhooyr.io/websocket)
//   - pion/     — WebRTC media forwarding (wraps github.com/pion/webrtc)
//   - mock/     — In-memory mock implementations for testing
package crosstalk

import (
	"strings"
	"time"
)

// User represents an authenticated admin user.
type User struct {
	ID           string
	Username     string
	PasswordHash string
	CreatedAt    time.Time
}

// APIToken represents a long-lived API authentication token.
type APIToken struct {
	ID        string
	Name      string
	TokenHash string
	UserID    string
	CreatedAt time.Time
}

// Role defines a named role in a session template with cardinality settings.
type Role struct {
	Name        string `json:"name"`
	MultiClient bool   `json:"multi_client"`
}

// Mapping defines a channel routing rule within a session template.
type Mapping struct {
	Source string `json:"source"` // "role:channel_name"
	Sink   string `json:"sink"`   // "role:channel_name", "record", or "broadcast"
}

// SessionTemplate is a reusable blueprint for sessions.
type SessionTemplate struct {
	ID        string
	Name      string
	IsDefault bool
	Roles     []Role
	Mappings  []Mapping
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Session is a runtime instance of a SessionTemplate.
type Session struct {
	ID         string
	TemplateID string
	Name       string
	Status     SessionStatus
	CreatedAt  time.Time
	EndedAt    *time.Time
}

// SessionStatus represents the lifecycle state of a session.
type SessionStatus string

const (
	SessionWaiting SessionStatus = "waiting"
	SessionActive  SessionStatus = "active"
	SessionEnded   SessionStatus = "ended"
)

// SessionClient tracks a client's participation in a session.
type SessionClient struct {
	ID             string
	SessionID      string
	Role           string
	ClientID       string
	Status         ClientConnectionStatus
	ConnectedAt    time.Time
	DisconnectedAt *time.Time
}

// ClientConnectionStatus represents a client's connection state in a session.
type ClientConnectionStatus string

const (
	ClientConnected    ClientConnectionStatus = "connected"
	ClientDisconnected ClientConnectionStatus = "disconnected"
)

// UserService defines operations on users.
type UserService interface {
	FindUserByID(id string) (*User, error)
	FindUserByUsername(username string) (*User, error)
	ListUsers() ([]User, error)
	CreateUser(user *User) error
	UpdateUser(user *User) error
	DeleteUser(id string) error
}

// TokenService defines operations on API tokens.
type TokenService interface {
	FindTokenByHash(hash string) (*APIToken, error)
	CreateToken(token *APIToken) error
	DeleteToken(id string) error
	ListTokens() ([]APIToken, error)
}

// SessionTemplateService defines operations on session templates.
type SessionTemplateService interface {
	FindTemplateByID(id string) (*SessionTemplate, error)
	ListTemplates() ([]SessionTemplate, error)
	CreateTemplate(tmpl *SessionTemplate) error
	UpdateTemplate(tmpl *SessionTemplate) error
	DeleteTemplate(id string) error
	FindDefaultTemplate() (*SessionTemplate, error)
}

// SessionService defines operations on sessions.
type SessionService interface {
	FindSessionByID(id string) (*Session, error)
	ListSessions() ([]Session, error)
	CreateSession(session *Session) error
	UpdateSessionStatus(id string, status SessionStatus) error
	EndSession(id string) error
}

// SessionOrchestrator defines the subset of orchestrator operations needed by
// the HTTP layer. The full implementation lives in pion.Orchestrator.
type SessionOrchestrator interface {
	EndSession(sessionID string)
}

// Validate checks that a SessionTemplate's mappings are consistent with its roles.
// Multi-client roles must not appear as mapping sources.
func (t *SessionTemplate) Validate() error {
	roleSet := make(map[string]Role, len(t.Roles))
	for _, r := range t.Roles {
		roleSet[r.Name] = r
	}

	for _, m := range t.Mappings {
		srcRole := sourceRole(m.Source)
		if srcRole == "" {
			continue
		}
		if r, ok := roleSet[srcRole]; ok && r.MultiClient {
			return &ValidationError{
				Field:   "mappings",
				Message: "multi-client role " + srcRole + " cannot be a mapping source",
			}
		}
	}
	return nil
}

// ValidationError represents a domain validation failure.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

// sourceRole extracts the role name from a "role:channel" mapping source.
func sourceRole(source string) string {
	for i, c := range source {
		if c == ':' {
			return source[:i]
		}
	}
	return ""
}

// ActiveBinding represents a resolved template mapping where both sides are available.
type ActiveBinding struct {
	Mapping       Mapping // The original template mapping
	SourceRole    string  // Role name of the source
	SourceChannel string  // Channel name on the source
	SinkRole      string  // Role name of the sink (empty for "record"/"broadcast")
	SinkChannel   string  // Channel name on the sink (empty for "record"/"broadcast")
	SinkType      string  // "role", "record", or "broadcast"
}

// SplitRoleChannel splits "role:channel" into (role, channel).
// Returns ("", "") if the format is invalid.
func SplitRoleChannel(s string) (string, string) {
	i := strings.IndexByte(s, ':')
	if i < 0 {
		return "", ""
	}
	return s[:i], s[i+1:]
}

// ResolveBindings computes which template mappings can activate given the set of
// connected role names. A mapping activates when:
//   - Its source role is in connectedRoles
//   - Its sink is "record" or "broadcast" (always available when source is present)
//   - OR its sink role is also in connectedRoles
func ResolveBindings(tmpl *SessionTemplate, connectedRoles map[string]bool) []ActiveBinding {
	var bindings []ActiveBinding
	for _, m := range tmpl.Mappings {
		srcRole, srcChannel := SplitRoleChannel(m.Source)
		if srcRole == "" || !connectedRoles[srcRole] {
			continue
		}

		switch m.Sink {
		case "record":
			bindings = append(bindings, ActiveBinding{
				Mapping:       m,
				SourceRole:    srcRole,
				SourceChannel: srcChannel,
				SinkType:      "record",
			})
		case "broadcast":
			bindings = append(bindings, ActiveBinding{
				Mapping:       m,
				SourceRole:    srcRole,
				SourceChannel: srcChannel,
				SinkType:      "broadcast",
			})
		default:
			sinkRole, sinkChannel := SplitRoleChannel(m.Sink)
			if sinkRole == "" || !connectedRoles[sinkRole] {
				continue
			}
			bindings = append(bindings, ActiveBinding{
				Mapping:       m,
				SourceRole:    srcRole,
				SourceChannel: srcChannel,
				SinkRole:      sinkRole,
				SinkChannel:   sinkChannel,
				SinkType:      "role",
			})
		}
	}
	return bindings
}
