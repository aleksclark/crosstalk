package api

import "time"

// --- Error ---

// ErrorDetail represents an API error.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// --- Auth ---

type LoginRequest struct {
	Body struct {
		Username string `json:"username" minLength:"1" doc:"Username"`
		Password string `json:"password" minLength:"1" doc:"Password"`
	}
}

type LoginResponse struct {
	Body struct {
		AccessToken  string `json:"access_token" doc:"JWT access token"`
		RefreshToken string `json:"refresh_token" doc:"Opaque refresh token"`
	}
}

type RefreshRequest struct {
	Body struct {
		RefreshToken string `json:"refresh_token" minLength:"1" doc:"Refresh token"`
	}
}

type RefreshResponse struct {
	Body struct {
		AccessToken  string `json:"access_token" doc:"JWT access token"`
		RefreshToken string `json:"refresh_token" doc:"Opaque refresh token"`
	}
}

type LogoutRequest struct {
	Body struct {
		RefreshToken string `json:"refresh_token" minLength:"1" doc:"Refresh token to revoke"`
	}
}

type LogoutResponse struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

// --- Sessions ---

type SessionOut struct {
	ID             string    `json:"id" doc:"Session ULID"`
	Name           string    `json:"name" doc:"Session name"`
	Description    string    `json:"description" doc:"Session description"`
	BroadcastToken string    `json:"broadcast_token,omitempty" doc:"Broadcast token"`
	CreatedAt      time.Time `json:"created_at" doc:"Creation time"`
	UpdatedAt      time.Time `json:"updated_at" doc:"Last update time"`
}

type ListSessionsRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
}

type ListSessionsResponse struct {
	Body struct {
		Data []SessionOut `json:"data"`
	}
}

type CreateSessionRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
	Body          struct {
		Name        string `json:"name" minLength:"1" doc:"Session name"`
		Description string `json:"description,omitempty" doc:"Session description"`
	}
}

type CreateSessionResponse struct {
	Body SessionOut
}

type GetSessionRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
	ID            string `path:"id" doc:"Session ID"`
}

type GetSessionResponse struct {
	Body SessionOut
}

type UpdateSessionRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
	ID            string `path:"id" doc:"Session ID"`
	Body          struct {
		Name        string `json:"name" minLength:"1" doc:"Session name"`
		Description string `json:"description,omitempty" doc:"Session description"`
	}
}

type UpdateSessionResponse struct {
	Body SessionOut
}

type DeleteSessionRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
	ID            string `path:"id" doc:"Session ID"`
}

type DeleteSessionResponse struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

type GetBroadcastURLRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
	ID            string `path:"id" doc:"Session ID"`
}

type GetBroadcastURLResponse struct {
	Body struct {
		BroadcastToken string `json:"broadcast_token" doc:"Broadcast token"`
		URL            string `json:"url" doc:"Full broadcast URL"`
	}
}

type RegenerateBroadcastURLRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
	ID            string `path:"id" doc:"Session ID"`
}

type RegenerateBroadcastURLResponse struct {
	Body struct {
		BroadcastToken string `json:"broadcast_token" doc:"New broadcast token"`
		URL            string `json:"url" doc:"Full broadcast URL"`
	}
}

// --- Channels ---

type ChannelOut struct {
	ID        string    `json:"id" doc:"Channel ULID"`
	SessionID string    `json:"session_id" doc:"Parent session ID"`
	Name      string    `json:"name" doc:"Channel name"`
	Type      string    `json:"type" doc:"Channel type (feed or broadcast)"`
	CreatedAt time.Time `json:"created_at" doc:"Creation time"`
}

type ListChannelsRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
	ID            string `path:"id" doc:"Session ID"`
}

type ListChannelsResponse struct {
	Body struct {
		Data []ChannelOut `json:"data"`
	}
}

type CreateChannelRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
	ID            string `path:"id" doc:"Session ID"`
	Body          struct {
		Name string `json:"name" minLength:"1" doc:"Channel name"`
		Type string `json:"type" enum:"feed,broadcast" doc:"Channel type"`
	}
}

type CreateChannelResponse struct {
	Body ChannelOut
}

type UpdateChannelRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
	ID            string `path:"id" doc:"Session ID"`
	ChID          string `path:"ch_id" doc:"Channel ID"`
	Body          struct {
		Name string `json:"name" minLength:"1" doc:"Channel name"`
		Type string `json:"type" enum:"feed,broadcast" doc:"Channel type"`
	}
}

type UpdateChannelResponse struct {
	Body ChannelOut
}

type DeleteChannelRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
	ID            string `path:"id" doc:"Session ID"`
	ChID          string `path:"ch_id" doc:"Channel ID"`
}

type DeleteChannelResponse struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

// --- Mix ---

type MixEntryOut struct {
	ID        string  `json:"id" doc:"Mix entry ID"`
	ChannelID string  `json:"channel_id" doc:"Channel ID"`
	SourceID  string  `json:"source_id" doc:"Source ID"`
	Muted     bool    `json:"muted" doc:"Whether source is muted"`
	Level     float64 `json:"level" doc:"Volume level (0.0-2.0)"`
}

type GetMixRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
	ID            string `path:"id" doc:"Session ID"`
	ChID          string `path:"ch_id" doc:"Channel ID"`
}

type GetMixResponse struct {
	Body struct {
		Data []MixEntryOut `json:"data"`
	}
}

type MixEntryInput struct {
	SourceID string  `json:"source_id" doc:"Source ID"`
	Muted    bool    `json:"muted" doc:"Whether source is muted"`
	Level    float64 `json:"level" minimum:"0" maximum:"2" doc:"Volume level"`
}

type UpdateMixRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
	ID            string `path:"id" doc:"Session ID"`
	ChID          string `path:"ch_id" doc:"Channel ID"`
	Body          struct {
		Entries []MixEntryInput `json:"entries" doc:"Mix entries to set"`
	}
}

type UpdateMixResponse struct {
	Body struct {
		Data []MixEntryOut `json:"data"`
	}
}

// --- Sources ---

type SourceOut struct {
	ID        string    `json:"id" doc:"Source ULID"`
	SessionID string    `json:"session_id" doc:"Parent session ID"`
	Name      string    `json:"name" doc:"Source name"`
	Origin    string    `json:"origin" doc:"How the source connected (abc, translator, admin)"`
	PeerID    *string   `json:"peer_id,omitempty" doc:"Associated peer ID"`
	Connected bool      `json:"connected" doc:"Whether the source is currently connected"`
	FirstSeen time.Time `json:"first_seen" doc:"First time the source was seen"`
	LastSeen  time.Time `json:"last_seen" doc:"Last time the source was seen"`
}

type ListSourcesRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
	ID            string `path:"id" doc:"Session ID"`
}

type ListSourcesResponse struct {
	Body struct {
		Data []SourceOut `json:"data"`
	}
}

// --- ABCs ---

type ABCOut struct {
	ID               string     `json:"id" doc:"ABC ULID"`
	Name             string     `json:"name" doc:"ABC name"`
	SessionID        *string    `json:"session_id,omitempty" doc:"Assigned session ID"`
	MonitorChannelID *string    `json:"monitor_channel_id,omitempty" doc:"Channel the ABC monitors for return audio"`
	Connected        bool       `json:"connected" doc:"Whether ABC is currently connected"`
	LastSeen         *time.Time `json:"last_seen,omitempty" doc:"Last time ABC was seen"`
	CreatedAt        time.Time  `json:"created_at" doc:"Creation time"`
}

type ListABCsRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
}

type ListABCsResponse struct {
	Body struct {
		Data []ABCOut `json:"data"`
	}
}

type CreateABCRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
	Body          struct {
		Name string `json:"name" minLength:"1" doc:"ABC name"`
	}
}

type CreateABCResponse struct {
	Body struct {
		ID    string `json:"id" doc:"ABC ID"`
		Name  string `json:"name" doc:"ABC name"`
		Token string `json:"token" doc:"API token (shown once)"`
	}
}

type GetABCRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
	ID            string `path:"id" doc:"ABC ID"`
}

type GetABCResponse struct {
	Body ABCOut
}

type UpdateABCRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
	ID            string `path:"id" doc:"ABC ID"`
	Body          struct {
		Name             string  `json:"name,omitempty" doc:"ABC name"`
		SessionID        *string `json:"session_id,omitempty" doc:"Assigned session ID"`
		MonitorChannelID *string `json:"monitor_channel_id,omitempty" doc:"Channel the ABC monitors for return audio"`
	}
}

type UpdateABCResponse struct {
	Body ABCOut
}

type DeleteABCRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
	ID            string `path:"id" doc:"ABC ID"`
}

type DeleteABCResponse struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

type RestartABCRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
	ID            string `path:"id" doc:"ABC ID"`
}

type RestartABCResponse struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

// --- Translators ---

type TranslatorOut struct {
	ID        string    `json:"id" doc:"User ULID"`
	Username  string    `json:"username" doc:"Username"`
	Sessions  []string  `json:"sessions,omitempty" doc:"Assigned session IDs"`
	CreatedAt time.Time `json:"created_at" doc:"Creation time"`
}

type ListTranslatorsRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
}

type ListTranslatorsResponse struct {
	Body struct {
		Data []TranslatorOut `json:"data"`
	}
}

type CreateTranslatorRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
	Body          struct {
		Username string `json:"username" minLength:"1" doc:"Username"`
		Password string `json:"password" minLength:"1" doc:"Password"`
	}
}

type CreateTranslatorResponse struct {
	Body TranslatorOut
}

type UpdateTranslatorRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
	ID            string `path:"id" doc:"Translator ID"`
	Body          struct {
		Username string `json:"username,omitempty" doc:"Username"`
		Password string `json:"password,omitempty" doc:"Password (leave empty to keep current)"`
	}
}

type UpdateTranslatorResponse struct {
	Body TranslatorOut
}

type DeleteTranslatorRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
	ID            string `path:"id" doc:"Translator ID"`
}

type DeleteTranslatorResponse struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

type AssignTranslatorSessionsRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
	ID            string `path:"id" doc:"Translator ID"`
	Body          struct {
		SessionIDs []string `json:"session_ids" doc:"Session IDs to assign"`
	}
}

type AssignTranslatorSessionsResponse struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

// --- Users ---

type UserOut struct {
	ID        string    `json:"id" doc:"User ULID"`
	Username  string    `json:"username" doc:"Username"`
	Role      string    `json:"role" doc:"User role"`
	CreatedAt time.Time `json:"created_at" doc:"Creation time"`
}

type ListUsersRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
}

type ListUsersResponse struct {
	Body struct {
		Data []UserOut `json:"data"`
	}
}

type CreateUserRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
	Body          struct {
		Username string `json:"username" minLength:"1" doc:"Username"`
		Password string `json:"password" minLength:"1" doc:"Password"`
		Role     string `json:"role" enum:"admin,translator" doc:"User role"`
	}
}

type CreateUserResponse struct {
	Body UserOut
}

type DeleteUserRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
	ID            string `path:"id" doc:"User ID"`
}

type DeleteUserResponse struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

// --- Public ---

type GetBroadcastInfoRequest struct {
	ID string `path:"id" doc:"Session ID"`
}

type GetBroadcastInfoResponse struct {
	Body struct {
		SessionID   string `json:"session_id" doc:"Session ID"`
		SessionName string `json:"session_name" doc:"Session name"`
		Active      bool   `json:"active" doc:"Whether session is active"`
	}
}

// --- WebRTC ---

type WebRTCTokenRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
}

type WebRTCTokenResponse struct {
	Body struct {
		Token     string    `json:"token" doc:"Short-lived WebRTC signaling token"`
		ExpiresAt time.Time `json:"expires_at" doc:"Token expiration time"`
	}
}

// --- Recordings ---

type RecordingOut struct {
	ID        string     `json:"id" doc:"Recording ULID"`
	SessionID string     `json:"session_id" doc:"Session ID"`
	SourceID  *string    `json:"source_id,omitempty" doc:"Source ID (if source recording)"`
	ChannelID *string    `json:"channel_id,omitempty" doc:"Channel ID (if channel recording)"`
	FilePath  string     `json:"file_path" doc:"Relative file path"`
	StartedAt time.Time  `json:"started_at" doc:"Recording start time"`
	EndedAt   *time.Time `json:"ended_at,omitempty" doc:"Recording end time"`
	SizeBytes int64      `json:"size_bytes" doc:"File size in bytes"`
}

type ListRecordingsRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
	ID            string `path:"id" doc:"Session ID"`
}

type ListRecordingsResponse struct {
	Body struct {
		Data []RecordingOut `json:"data"`
	}
}

type DownloadRecordingRequest struct {
	Authorization string `header:"Authorization" doc:"Bearer token"`
	ID            string `path:"id" doc:"Recording ID"`
}

type DownloadRecordingResponse struct {
	Body struct {
		FilePath string `json:"file_path" doc:"Absolute file path for download"`
	}
}
