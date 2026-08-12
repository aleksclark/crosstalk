package postgres

import (
	"encoding/json"
	"time"

	"github.com/uptrace/bun"

	crosstalk "github.com/aleksclark/crosstalk/server"
)

// sessionModel maps the sessions table.
type sessionModel struct {
	bun.BaseModel `bun:"table:sessions,alias:sess"`

	ID              string     `bun:"id,pk"`
	Name            string     `bun:"name,notnull"`
	Description     string     `bun:"description,notnull"`
	BroadcastToken  *string    `bun:"broadcast_token"`
	State           string     `bun:"state,notnull"`
	OwnerID         string     `bun:"owner_id,notnull"`
	OwnerGeneration int64      `bun:"owner_generation,notnull"`
	LeaseUntil      *time.Time `bun:"lease_until"`
	StartedAt       *time.Time `bun:"started_at"`
	EndedAt         *time.Time `bun:"ended_at"`
	CreatedAt       time.Time  `bun:"created_at,notnull"`
	UpdatedAt       time.Time  `bun:"updated_at,notnull"`
}

func (m *sessionModel) toDomain() crosstalk.Session {
	s := crosstalk.Session{
		ID:              m.ID,
		Name:            m.Name,
		Description:     m.Description,
		State:           crosstalk.SessionState(m.State),
		OwnerID:         m.OwnerID,
		OwnerGeneration: uint64(m.OwnerGeneration),
		LeaseUntil:      m.LeaseUntil,
		StartedAt:       m.StartedAt,
		EndedAt:         m.EndedAt,
		CreatedAt:       m.CreatedAt,
		UpdatedAt:       m.UpdatedAt,
	}
	if m.BroadcastToken != nil {
		s.BroadcastToken = *m.BroadcastToken
	}
	if s.State == "" {
		s.State = crosstalk.SessionWaiting
	}
	return s
}

func sessionFromDomain(s *crosstalk.Session) *sessionModel {
	state := string(s.State)
	if state == "" {
		state = string(crosstalk.SessionWaiting)
	}
	m := &sessionModel{
		ID:              s.ID,
		Name:            s.Name,
		Description:     s.Description,
		State:           state,
		OwnerID:         s.OwnerID,
		OwnerGeneration: int64(s.OwnerGeneration),
		LeaseUntil:      s.LeaseUntil,
		StartedAt:       s.StartedAt,
		EndedAt:         s.EndedAt,
		CreatedAt:       s.CreatedAt,
		UpdatedAt:       s.UpdatedAt,
	}
	if s.BroadcastToken != "" {
		t := s.BroadcastToken
		m.BroadcastToken = &t
	}
	return m
}

// mediaTicketModel maps the media_tickets table.
type mediaTicketModel struct {
	bun.BaseModel `bun:"table:media_tickets,alias:mt"`

	ID                string     `bun:"id,pk"`
	NonceHash         string     `bun:"nonce_hash,notnull"`
	SessionID         string     `bun:"session_id,notnull"`
	OwnerID           string     `bun:"owner_id,notnull"`
	OwnerGeneration   int64      `bun:"owner_generation,notnull"`
	Subject           string     `bun:"subject,notnull"`
	Role              string     `bun:"role,notnull"`
	ProduceChannelIDs []string   `bun:"produce_channel_ids,array,notnull"`
	ListenChannelIDs  []string   `bun:"listen_channel_ids,array,notnull"`
	ExpiresAt         time.Time  `bun:"expires_at,notnull"`
	ConsumedAt        *time.Time `bun:"consumed_at"`
	CreatedAt         time.Time  `bun:"created_at,notnull"`
}

func (m *mediaTicketModel) toDomain() crosstalk.MediaTicket {
	produce := append([]string(nil), m.ProduceChannelIDs...)
	listen := append([]string(nil), m.ListenChannelIDs...)
	return crosstalk.MediaTicket{
		ID:                m.ID,
		NonceHash:         m.NonceHash,
		SessionID:         m.SessionID,
		OwnerID:           m.OwnerID,
		OwnerGeneration:   uint64(m.OwnerGeneration),
		Subject:           m.Subject,
		Role:              m.Role,
		ProduceChannelIDs: produce,
		ListenChannelIDs:  listen,
		ExpiresAt:         m.ExpiresAt,
		ConsumedAt:        m.ConsumedAt,
		CreatedAt:         m.CreatedAt,
	}
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

	ID               string     `bun:"id,pk"`
	Name             string     `bun:"name,notnull"`
	TokenHash        string     `bun:"token_hash,notnull"`
	SessionID        *string    `bun:"session_id"`
	MonitorChannelID *string    `bun:"monitor_channel_id"`
	Connected        bool       `bun:"connected,notnull"`
	LastSeen         *time.Time `bun:"last_seen"`
	CreatedAt        time.Time  `bun:"created_at,notnull"`
}

func (m *abcModel) toDomain() crosstalk.ABC {
	return crosstalk.ABC{
		ID:               m.ID,
		Name:             m.Name,
		TokenHash:        m.TokenHash,
		SessionID:        m.SessionID,
		MonitorChannelID: m.MonitorChannelID,
		Connected:        m.Connected,
		LastSeen:         m.LastSeen,
		CreatedAt:        m.CreatedAt,
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

// abcAudioSettingsModel maps abc_audio_settings.
type abcAudioSettingsModel struct {
	bun.BaseModel `bun:"table:abc_audio_settings,alias:aas"`

	ABCID                       string     `bun:"abc_id,pk"`
	DesiredRevision             int64      `bun:"desired_revision,notnull"`
	DesiredOutputDeviceUID      *string    `bun:"desired_output_device_uid"`
	DesiredOutputVolumePercent  *int16     `bun:"desired_output_volume_percent"`
	DesiredOutputMuted          *bool      `bun:"desired_output_muted"`
	DesiredInputDeviceUID       *string    `bun:"desired_input_device_uid"`
	DesiredInputGainPercent     *int16     `bun:"desired_input_gain_percent"`
	CommandID                   *string    `bun:"command_id"`
	ReportedRevision            int64      `bun:"reported_revision,notnull"`
	ReportedCommandID           *string    `bun:"reported_command_id"`
	ReportedOutputDeviceUID     *string    `bun:"reported_output_device_uid"`
	ObservedOutputVolumePercent *int16     `bun:"observed_output_volume_percent"`
	ObservedOutputMuted         *bool      `bun:"observed_output_muted"`
	ReportedInputDeviceUID      *string    `bun:"reported_input_device_uid"`
	ObservedInputGainPercent    *int16     `bun:"observed_input_gain_percent"`
	OutputVolumeState           string     `bun:"output_volume_state,notnull"`
	OutputMuteState             string     `bun:"output_mute_state,notnull"`
	InputGainState              string     `bun:"input_gain_state,notnull"`
	ErrorCode                   string          `bun:"error_code,notnull"`
	ErrorDetail                 string          `bun:"error_detail,notnull"`
	Capabilities                json.RawMessage `bun:"capabilities,type:jsonb,notnull"`
	ReportedAt                  *time.Time      `bun:"reported_at"`
	DesiredUpdatedAt            *time.Time      `bun:"desired_updated_at"`
	UpdatedAt                   time.Time       `bun:"updated_at,notnull"`
}

// abcAudioAuditModel maps abc_audio_audit_events.
type abcAudioAuditModel struct {
	bun.BaseModel `bun:"table:abc_audio_audit_events,alias:aae"`

	ID              string          `bun:"id,pk"`
	ABCID           string          `bun:"abc_id,notnull"`
	RequestID       string          `bun:"request_id,notnull"`
	ActorUserID     string          `bun:"actor_user_id,notnull"`
	ActorRole       string          `bun:"actor_role,notnull"`
	DesiredRevision int64           `bun:"desired_revision,notnull"`
	PreviousDesired json.RawMessage `bun:"previous_desired,type:jsonb,notnull"`
	NewDesired      json.RawMessage `bun:"new_desired,type:jsonb,notnull"`
	Outcome         string          `bun:"outcome,notnull"`
	CreatedAt       time.Time       `bun:"created_at,notnull"`
}
