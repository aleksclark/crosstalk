// Package crosstalk defines the core domain types for the CrossTalk server.
// This package has zero external dependencies.
package crosstalk

import "time"

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

// Session represents a long-lived session (e.g., "10am Sunday service").
type Session struct {
	ID             string
	Name           string
	Description    string
	BroadcastToken string
	CreatedAt      time.Time
	UpdatedAt      time.Time
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
	Connected bool
	LastSeen  *time.Time
	CreatedAt time.Time
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
