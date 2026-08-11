package crosstalk

import "context"

// SessionService manages sessions.
type SessionService interface {
	List(ctx context.Context) ([]Session, error)
	// ListPage returns a bounded, sorted, optionally filtered page of sessions.
	// Assignment scope (RestrictToIDs) is applied before pagination.
	ListPage(ctx context.Context, q ListQuery) (SessionPage, error)
	Get(ctx context.Context, id string) (*Session, error)
	Create(ctx context.Context, s *Session) error
	Update(ctx context.Context, s *Session) error
	Delete(ctx context.Context, id string) error
	GetByBroadcastToken(ctx context.Context, token string) (*Session, error)
	RegenerateBroadcastToken(ctx context.Context, id string) (string, error)
	// TransitionState applies a lifecycle transition.
	// When fenceGeneration is non-nil the update is fenced: it only succeeds
	// when the session's current owner_generation matches. Terminal publishes
	// from a stale owner must fail closed.
	TransitionState(ctx context.Context, id string, to SessionState, fenceGeneration *uint64) error
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
	// ListPage returns a bounded page of ABCs with batch-resolved session names.
	// RestrictToIDs scopes by assigned session_id before pagination.
	ListPage(ctx context.Context, q ListQuery) (ABCPage, error)
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
	// ListTranslatorsPage returns a bounded page of translator accounts with
	// assigned session IDs and names resolved in one bounded lookup.
	ListTranslatorsPage(ctx context.Context, q ListQuery) (TranslatorPage, error)
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

// RecordingService manages recording metadata.
type RecordingService interface {
	Create(ctx context.Context, r *Recording) error
	FindBySession(ctx context.Context, sessionID string) ([]Recording, error)
	FindByID(ctx context.Context, id string) (*Recording, error)
	Update(ctx context.Context, r *Recording) error
	List(ctx context.Context) ([]Recording, error)
}

// MediaTicketService persists and consumes one-time media admission tickets.
type MediaTicketService interface {
	// Issue persists a hashed nonce and returns the plaintext nonce once via
	// the caller's input; the store only keeps the hash.
	Issue(ctx context.Context, ticket *MediaTicket, nonce string) error
	// Consume atomically marks an unconsumed, unexpired ticket as used when
	// the stored owner_generation matches. Returns the ticket row on success.
	Consume(ctx context.Context, nonce string, ownerGeneration uint64) (*MediaTicket, error)
	// GetByNonceHash looks up a ticket by hash without consuming it.
	GetByNonceHash(ctx context.Context, nonceHash string) (*MediaTicket, error)
}
