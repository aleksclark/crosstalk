// Package crosstalk defines the core domain types for the CrossTalk server.
// This package has zero external dependencies.
package crosstalk

import (
	"errors"
	"fmt"
	"time"
)

// ChannelType is the type of a channel within a session.
type ChannelType string

const (
	ChannelFeed      ChannelType = "feed"
	ChannelBroadcast ChannelType = "broadcast"
)

// SourceOrigin indicates how a source connected to the session.
type SourceOrigin string

const (
	OriginABC        SourceOrigin = "abc"
	OriginTranslator SourceOrigin = "translator"
	OriginAdmin      SourceOrigin = "admin"
)

// SessionState is the durable lifecycle state of a session.
type SessionState string

const (
	SessionWaiting  SessionState = "waiting"
	SessionActive   SessionState = "active"
	SessionDraining SessionState = "draining"
	SessionEnded    SessionState = "ended"
	SessionArchived SessionState = "archived"
	SessionFailed   SessionState = "failed"
)

// Domain errors for control-plane lifecycle, leases, and media tickets.
var (
	ErrInvalidSessionTransition = errors.New("invalid session state transition")
	ErrLeaseNotHeld             = errors.New("session lease not held")
	ErrLeaseHeld                = errors.New("session lease held by another owner")
	ErrStaleGeneration          = errors.New("stale owner generation")
	ErrTicketNotFound           = errors.New("media ticket not found")
	ErrTicketExpired            = errors.New("media ticket expired")
	ErrTicketConsumed           = errors.New("media ticket already consumed")
	ErrTicketInvalid            = errors.New("media ticket invalid")
)

// Session represents a long-lived session (e.g., "10am Sunday service").
type Session struct {
	ID              string
	Name            string
	Description     string
	BroadcastToken  string
	State           SessionState
	OwnerID         string
	OwnerGeneration uint64
	LeaseUntil      *time.Time
	StartedAt       *time.Time
	EndedAt         *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Channel represents an audio stream within a session.
type Channel struct {
	ID        string
	SessionID string
	Name      string
	Type      ChannelType
	CreatedAt time.Time
}

// Source represents any audio-producing connection.
type Source struct {
	ID        string
	SessionID string
	Name      string
	Origin    SourceOrigin
	PeerID    *string
	Connected bool
	FirstSeen time.Time
	LastSeen  time.Time
}

// MixEntry holds per-channel mix state for each source.
type MixEntry struct {
	ID        string
	ChannelID string
	SourceID  string
	Muted     bool
	Level     float64 // 0.0 - 2.0, default 1.0
}

// ABC represents an Audio Broadcast Client device.
type ABC struct {
	ID        string
	Name      string
	TokenHash string
	SessionID *string
	// MonitorChannelID is the channel the ABC listens to for booth return
	// audio. When nil the ABC falls back to monitoring all broadcast channels.
	MonitorChannelID *string
	Connected        bool
	LastSeen         *time.Time
	CreatedAt        time.Time
}

// User represents an admin or translator account.
type User struct {
	ID           string
	Username     string
	PasswordHash string
	Role         string // "admin" or "translator"
	CreatedAt    time.Time
}

// RefreshToken represents a stored refresh token for session management.
type RefreshToken struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	CreatedAt time.Time
}

// Recording represents a recorded audio file.
type Recording struct {
	ID        string
	SessionID string
	SourceID  *string
	ChannelID *string
	FilePath  string
	StartedAt time.Time
	EndedAt   *time.Time
	SizeBytes int64
}

// MediaTicket is a one-time, session-scoped media admission credential.
// Only the hash of the nonce/JTI is persisted; the plaintext nonce is returned
// to the issuer once and never stored.
type MediaTicket struct {
	ID                string
	NonceHash         string
	SessionID         string
	OwnerID           string
	OwnerGeneration   uint64
	Subject           string
	Role              string
	ProduceChannelIDs []string
	ListenChannelIDs  []string
	ExpiresAt         time.Time
	ConsumedAt        *time.Time
	CreatedAt         time.Time
}

// CanTransitionSession reports whether from→to is a legal lifecycle move.
// Same-state transitions are idempotent and always allowed.
func CanTransitionSession(from, to SessionState) bool {
	if from == to {
		return true
	}
	switch from {
	case SessionWaiting:
		return to == SessionActive
	case SessionActive:
		return to == SessionDraining || to == SessionFailed
	case SessionDraining:
		return to == SessionEnded || to == SessionFailed
	case SessionEnded:
		return to == SessionArchived
	default:
		return false
	}
}

// ValidateSessionTransition returns nil when from→to is legal.
func ValidateSessionTransition(from, to SessionState) error {
	if CanTransitionSession(from, to) {
		return nil
	}
	return fmt.Errorf("%w: %s -> %s", ErrInvalidSessionTransition, from, to)
}

// IsTerminalSessionState reports whether state is a terminal lifecycle state.
func IsTerminalSessionState(state SessionState) bool {
	return state == SessionEnded || state == SessionArchived || state == SessionFailed
}
