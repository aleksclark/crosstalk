package postgres

import (
	"time"

	"github.com/uptrace/bun"

	crosstalk "github.com/aleksclark/crosstalk/server"
)

// sessionModel maps the sessions table.
type sessionModel struct {
	bun.BaseModel `bun:"table:sessions,alias:sess"`

	ID             string    `bun:"id,pk"`
	Name           string    `bun:"name,notnull"`
	Description    string    `bun:"description,notnull"`
	BroadcastToken *string   `bun:"broadcast_token"`
	CreatedAt      time.Time `bun:"created_at,notnull"`
	UpdatedAt      time.Time `bun:"updated_at,notnull"`
}

func (m *sessionModel) toDomain() crosstalk.Session {
	s := crosstalk.Session{
		ID:          m.ID,
		Name:        m.Name,
		Description: m.Description,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
	if m.BroadcastToken != nil {
		s.BroadcastToken = *m.BroadcastToken
	}
	return s
}

func sessionFromDomain(s *crosstalk.Session) *sessionModel {
	m := &sessionModel{
		ID:          s.ID,
		Name:        s.Name,
		Description: s.Description,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
	}
	if s.BroadcastToken != "" {
		t := s.BroadcastToken
		m.BroadcastToken = &t
	}
	return m
}

// channelModel maps the channels table.
type channelModel struct {
	bun.BaseModel `bun:"table:channels,alias:ch"`

	ID        string    `bun:"id,pk"`
	SessionID string    `bun:"session_id,notnull"`
	Name      string    `bun:"name,notnull"`
	Type      string    `bun:"type,notnull"`
	CreatedAt time.Time `bun:"created_at,notnull"`
}

func (m *channelModel) toDomain() crosstalk.Channel {
	return crosstalk.Channel{
		ID:        m.ID,
		SessionID: m.SessionID,
		Name:      m.Name,
		Type:      crosstalk.ChannelType(m.Type),
		CreatedAt: m.CreatedAt,
	}
}

// sourceModel maps the sources table.
type sourceModel struct {
	bun.BaseModel `bun:"table:sources,alias:src"`

	ID        string    `bun:"id,pk"`
	SessionID string    `bun:"session_id,notnull"`
	Name      string    `bun:"name,notnull"`
	Origin    string    `bun:"origin,notnull"`
	PeerID    *string   `bun:"peer_id"`
	Connected bool      `bun:"connected,notnull"`
	FirstSeen time.Time `bun:"first_seen,notnull"`
	LastSeen  time.Time `bun:"last_seen,notnull"`
}

func (m *sourceModel) toDomain() crosstalk.Source {
	return crosstalk.Source{
		ID:        m.ID,
		SessionID: m.SessionID,
		Name:      m.Name,
		Origin:    crosstalk.SourceOrigin(m.Origin),
		PeerID:    m.PeerID,
		Connected: m.Connected,
		FirstSeen: m.FirstSeen,
		LastSeen:  m.LastSeen,
	}
}

// mixModel maps the channel_mix table.
type mixModel struct {
	bun.BaseModel `bun:"table:channel_mix,alias:mix"`

	ID        string  `bun:"id,pk"`
	ChannelID string  `bun:"channel_id,notnull"`
	SourceID  string  `bun:"source_id,notnull"`
	Muted     bool    `bun:"muted,notnull"`
	Level     float64 `bun:"level,notnull"`
}

func (m *mixModel) toDomain() crosstalk.MixEntry {
	return crosstalk.MixEntry{
		ID:        m.ID,
		ChannelID: m.ChannelID,
		SourceID:  m.SourceID,
		Muted:     m.Muted,
		Level:     m.Level,
	}
}

// abcModel maps the abcs table.
type abcModel struct {
	bun.BaseModel `bun:"table:abcs,alias:abc"`

	ID        string     `bun:"id,pk"`
	Name      string     `bun:"name,notnull"`
	TokenHash string     `bun:"token_hash,notnull"`
	SessionID *string    `bun:"session_id"`
	Connected bool       `bun:"connected,notnull"`
	LastSeen  *time.Time `bun:"last_seen"`
	CreatedAt time.Time  `bun:"created_at,notnull"`
}

func (m *abcModel) toDomain() crosstalk.ABC {
	return crosstalk.ABC{
		ID:        m.ID,
		Name:      m.Name,
		TokenHash: m.TokenHash,
		SessionID: m.SessionID,
		Connected: m.Connected,
		LastSeen:  m.LastSeen,
		CreatedAt: m.CreatedAt,
	}
}

// userModel maps the users table.
type userModel struct {
	bun.BaseModel `bun:"table:users,alias:usr"`

	ID           string    `bun:"id,pk"`
	Username     string    `bun:"username,notnull"`
	PasswordHash string    `bun:"password_hash,notnull"`
	Role         string    `bun:"role,notnull"`
	CreatedAt    time.Time `bun:"created_at,notnull"`
}

func (m *userModel) toDomain() crosstalk.User {
	return crosstalk.User{
		ID:           m.ID,
		Username:     m.Username,
		PasswordHash: m.PasswordHash,
		Role:         m.Role,
		CreatedAt:    m.CreatedAt,
	}
}

// translatorSessionModel maps the translator_sessions join table.
type translatorSessionModel struct {
	bun.BaseModel `bun:"table:translator_sessions,alias:ts"`

	TranslatorID string `bun:"translator_id,pk"`
	SessionID    string `bun:"session_id,pk"`
}

// refreshTokenModel maps the refresh_tokens table.
type refreshTokenModel struct {
	bun.BaseModel `bun:"table:refresh_tokens,alias:rt"`

	ID        string    `bun:"id,pk"`
	UserID    string    `bun:"user_id,notnull"`
	TokenHash string    `bun:"token_hash,notnull"`
	ExpiresAt time.Time `bun:"expires_at,notnull"`
	CreatedAt time.Time `bun:"created_at,notnull"`
}

func (m *refreshTokenModel) toDomain() crosstalk.RefreshToken {
	return crosstalk.RefreshToken{
		ID:        m.ID,
		UserID:    m.UserID,
		TokenHash: m.TokenHash,
		ExpiresAt: m.ExpiresAt,
		CreatedAt: m.CreatedAt,
	}
}

// recordingModel maps the recordings table.
type recordingModel struct {
	bun.BaseModel `bun:"table:recordings,alias:rec"`

	ID        string     `bun:"id,pk"`
	SessionID string     `bun:"session_id,notnull"`
	SourceID  *string    `bun:"source_id"`
	ChannelID *string    `bun:"channel_id"`
	FilePath  string     `bun:"file_path,notnull"`
	StartedAt time.Time  `bun:"started_at,notnull"`
	EndedAt   *time.Time `bun:"ended_at"`
	SizeBytes int64      `bun:"size_bytes,notnull"`
}

func (m *recordingModel) toDomain() crosstalk.Recording {
	return crosstalk.Recording{
		ID:        m.ID,
		SessionID: m.SessionID,
		SourceID:  m.SourceID,
		ChannelID: m.ChannelID,
		FilePath:  m.FilePath,
		StartedAt: m.StartedAt,
		EndedAt:   m.EndedAt,
		SizeBytes: m.SizeBytes,
	}
}
