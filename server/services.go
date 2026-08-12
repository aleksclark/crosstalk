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

// ABCAudioService manages durable desired/observed ABC USB audio control state.
// SetDesired and RecordReport own transactions, revision ordering, idempotency,
// and audit writes; handlers must not reimplement those rules.
type ABCAudioService interface {
	// Get returns durable audio status for an existing ABC. When no settings
	// row exists yet, it returns an unconfigured status without creating a row.
	// Returns ErrABCAudioABCNotFound when the ABC itself is missing.
	Get(ctx context.Context, abcID string) (*ABCAudioStatus, error)

	// SetDesired absolutely replaces desired state when expectedRevision matches.
	// Duplicate (abcID, requestID) returns the originally accepted status.
	// Byte-equal desired is a successful no-op (no revision bump).
	// Conflicting expectedRevision returns ErrABCAudioRevisionConflict.
	SetDesired(ctx context.Context, abcID, actorID, actorRole, requestID string,
		expectedRevision uint64, desired ABCAudioDesired) (*ABCAudioStatus, error)

	// RecordReport persists a validated board observation. Stale reports whose
	// desired revision is lower than the persisted reported revision are ignored
	// (status returned unchanged; no error). Inventory-only revision 0 may create
	// a settings row and update capabilities without overwriting desired state.
	RecordReport(ctx context.Context, abcID string, report ABCAudioObservation) (*ABCAudioStatus, error)

	// ListAudit returns recent audit events for an ABC (newest first). limit<=0
	// uses a sensible default. Optional helper for operators/tests.
	ListAudit(ctx context.Context, abcID string, limit int) ([]ABCAudioAuditEvent, error)
}
