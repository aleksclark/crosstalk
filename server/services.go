package crosstalk

import "context"

// SessionService manages sessions.
type SessionService interface {
	List(ctx context.Context) ([]Session, error)
	Get(ctx context.Context, id string) (*Session, error)
	Create(ctx context.Context, s *Session) error
	Update(ctx context.Context, s *Session) error
	Delete(ctx context.Context, id string) error
	GetByBroadcastToken(ctx context.Context, token string) (*Session, error)
	RegenerateBroadcastToken(ctx context.Context, id string) (string, error)
}

// ChannelService manages channels within sessions.
type ChannelService interface {
	List(ctx context.Context, sessionID string) ([]Channel, error)
	Get(ctx context.Context, id string) (*Channel, error)
	Create(ctx context.Context, c *Channel) error
	Update(ctx context.Context, c *Channel) error
	Delete(ctx context.Context, id string) error
}

// SourceService manages sources.
type SourceService interface {
	List(ctx context.Context, sessionID string) ([]Source, error)
	Get(ctx context.Context, id string) (*Source, error)
	Create(ctx context.Context, s *Source) error
	Update(ctx context.Context, s *Source) error
	Delete(ctx context.Context, id string) error
}

// MixService manages channel mix state.
type MixService interface {
	GetMix(ctx context.Context, channelID string) ([]MixEntry, error)
	SetMix(ctx context.Context, channelID string, entries []MixEntry) error
}

// ABCService manages Audio Broadcast Clients.
type ABCService interface {
	List(ctx context.Context) ([]ABC, error)
	Get(ctx context.Context, id string) (*ABC, error)
	Create(ctx context.Context, abc *ABC) error
	Update(ctx context.Context, abc *ABC) error
	Delete(ctx context.Context, id string) error
	GetByTokenHash(ctx context.Context, tokenHash string) (*ABC, error)
}

// UserService manages user accounts.
type UserService interface {
	List(ctx context.Context) ([]User, error)
	Get(ctx context.Context, id string) (*User, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
	Create(ctx context.Context, u *User) error
	Update(ctx context.Context, u *User) error
	Delete(ctx context.Context, id string) error
	ListByRole(ctx context.Context, role string) ([]User, error)
	AssignSessions(ctx context.Context, translatorID string, sessionIDs []string) error
	GetAssignedSessions(ctx context.Context, translatorID string) ([]string, error)
}

// RefreshTokenService manages refresh tokens.
type RefreshTokenService interface {
	Create(ctx context.Context, rt *RefreshToken) error
	GetByHash(ctx context.Context, hash string) (*RefreshToken, error)
	DeleteByHash(ctx context.Context, hash string) error
	DeleteByUserID(ctx context.Context, userID string) error
}
