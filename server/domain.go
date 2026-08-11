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

// ---------------------------------------------------------------------------
// ABC remote USB audio control (desired / observed / audit)
// ---------------------------------------------------------------------------

// ABCAudioControlState is the per-control apply/readback state.
type ABCAudioControlState string

const (
	ABCAudioControlUnknown        ABCAudioControlState = "unknown"
	ABCAudioControlPending        ABCAudioControlState = "pending"
	ABCAudioControlApplied        ABCAudioControlState = "applied"
	ABCAudioControlUnsupported    ABCAudioControlState = "unsupported"
	ABCAudioControlError          ABCAudioControlState = "error"
	ABCAudioControlDeviceMismatch ABCAudioControlState = "device_mismatch"
)

// ValidABCAudioControlState reports whether s is a persisted control state.
func ValidABCAudioControlState(s ABCAudioControlState) bool {
	switch s {
	case ABCAudioControlUnknown, ABCAudioControlPending, ABCAudioControlApplied,
		ABCAudioControlUnsupported, ABCAudioControlError, ABCAudioControlDeviceMismatch:
		return true
	default:
		return false
	}
}

// ABCAudioOverallState is the derived aggregate status for an ABC's audio control.
type ABCAudioOverallState string

const (
	ABCAudioOverallUnconfigured   ABCAudioOverallState = "unconfigured"
	ABCAudioOverallOffline        ABCAudioOverallState = "offline"
	ABCAudioOverallStale          ABCAudioOverallState = "stale"
	ABCAudioOverallPending        ABCAudioOverallState = "pending"
	ABCAudioOverallError          ABCAudioOverallState = "error"
	ABCAudioOverallDeviceMismatch ABCAudioOverallState = "device_mismatch"
	ABCAudioOverallUnsupported    ABCAudioOverallState = "unsupported"
	ABCAudioOverallPartial        ABCAudioOverallState = "partial"
	ABCAudioOverallApplied        ABCAudioOverallState = "applied"
)

// ABC audio domain sentinel errors.
var (
	ErrABCAudioNotFound         = errors.New("abc audio settings not found")
	ErrABCAudioABCNotFound      = errors.New("abc not found")
	ErrABCAudioRevisionConflict = errors.New("abc audio revision conflict")
	ErrABCAudioInvalidDesired   = errors.New("abc audio desired invalid")
	ErrABCAudioInvalidReport    = errors.New("abc audio report invalid")
	ErrABCAudioStaleReport      = errors.New("abc audio report stale")
)

// MaxABCAudioRequestIDLen is the maximum allowed request_id length.
const MaxABCAudioRequestIDLen = 64

// MaxABCAudioErrorDetailBytes caps sanitized error_detail storage.
const MaxABCAudioErrorDetailBytes = 256

// ABCAudioDesired is the absolute desired hardware audio state.
// All fields are required for a configured (revision >= 1) desired state.
type ABCAudioDesired struct {
	OutputDeviceUID     string
	OutputVolumePercent int // inclusive 0..100
	OutputMuted         bool
	InputDeviceUID      string
	InputGainPercent    int // inclusive 0..100
}

// ABCAudioCapability describes one discovered audio endpoint on the board.
// Extra diagnostic fields may be present in the JSON capabilities blob.
type ABCAudioCapability struct {
	DeviceUID          string         `json:"device_uid"`
	Direction          string         `json:"direction,omitempty"` // "input", "output", or "both"
	Backend            string         `json:"backend,omitempty"`
	VendorID           string         `json:"vendor_id,omitempty"`
	ProductID          string         `json:"product_id,omitempty"`
	Serial             string         `json:"serial,omitempty"`
	Path               string         `json:"path,omitempty"`
	ALSACardID         string         `json:"alsa_card_id,omitempty"`
	CardName           string         `json:"card_name,omitempty"`
	SupportsVolume     bool           `json:"supports_volume,omitempty"`
	SupportsMute       bool           `json:"supports_mute,omitempty"`
	SupportsGain       bool           `json:"supports_gain,omitempty"`
	Extra              map[string]any `json:"extra,omitempty"`
}

// ABCAudioObservation is a board-reported inventory and/or apply result.
// DesiredRevision 0 is inventory-only and must not overwrite applied desired state.
type ABCAudioObservation struct {
	DesiredRevision             uint64
	CommandID                   string
	OutputDeviceUID             string
	ObservedOutputVolumePercent *int
	ObservedOutputMuted         *bool
	InputDeviceUID              string
	ObservedInputGainPercent    *int
	OutputVolumeState           ABCAudioControlState
	OutputMuteState             ABCAudioControlState
	InputGainState              ABCAudioControlState
	ErrorCode                   string
	ErrorDetail                 string
	Capabilities                []ABCAudioCapability
	ReportedAt                  time.Time
}

// ABCAudioDesiredView is the durable desired snapshot embedded in status.
// Nil pointer fields / empty UIDs indicate unconfigured (revision 0).
type ABCAudioDesiredView struct {
	Revision            uint64
	CommandID           string
	OutputDeviceUID     string
	OutputVolumePercent *int
	OutputMuted         *bool
	InputDeviceUID      string
	InputGainPercent    *int
	UpdatedAt           *time.Time
}

// ABCAudioReportedView is the durable observed snapshot embedded in status.
type ABCAudioReportedView struct {
	Revision                    uint64
	CommandID                   string
	OutputDeviceUID             string
	ObservedOutputVolumePercent *int
	ObservedOutputMuted         *bool
	InputDeviceUID              string
	ObservedInputGainPercent    *int
	OutputVolumeState           ABCAudioControlState
	OutputMuteState             ABCAudioControlState
	InputGainState              ABCAudioControlState
	ErrorCode                   string
	ErrorDetail                 string
	Capabilities                []ABCAudioCapability
	ReportedAt                  *time.Time
}

// ABCAudioStatus is the combined durable audio control view for one ABC.
type ABCAudioStatus struct {
	ABCID            string
	Connected        bool
	Desired          ABCAudioDesiredView
	Reported         ABCAudioReportedView
	OverallState     ABCAudioOverallState
	Stale            bool
	AcceptedRevision uint64
}

// ABCAudioAuditOutcome is the audit row outcome for a SetDesired attempt.
type ABCAudioAuditOutcome string

const (
	ABCAudioAuditAccepted ABCAudioAuditOutcome = "accepted"
	ABCAudioAuditNoOp     ABCAudioAuditOutcome = "no_op"
)

// ABCAudioAuditEvent is an append-only desired-state change record.
type ABCAudioAuditEvent struct {
	ID              string
	ABCID           string
	RequestID       string
	ActorUserID     string
	ActorRole       string
	DesiredRevision uint64
	PreviousDesired ABCAudioDesiredView
	NewDesired      ABCAudioDesiredView
	Outcome         ABCAudioAuditOutcome
	CreatedAt       time.Time
}

// ABCAudioCommandID returns the deterministic command id for a desired revision.
func ABCAudioCommandID(abcID string, desiredRevision uint64) string {
	return fmt.Sprintf("abc-audio/%s/%d", abcID, desiredRevision)
}

// ValidateABCAudioDesired checks absolute desired fields for a configured save.
func ValidateABCAudioDesired(d ABCAudioDesired) error {
	if d.OutputDeviceUID == "" || d.InputDeviceUID == "" {
		return fmt.Errorf("%w: device UIDs required", ErrABCAudioInvalidDesired)
	}
	if d.OutputVolumePercent < 0 || d.OutputVolumePercent > 100 {
		return fmt.Errorf("%w: output volume percent out of range", ErrABCAudioInvalidDesired)
	}
	if d.InputGainPercent < 0 || d.InputGainPercent > 100 {
		return fmt.Errorf("%w: input gain percent out of range", ErrABCAudioInvalidDesired)
	}
	return nil
}

// ValidateABCAudioPercent reports whether n is in inclusive 0..100.
func ValidateABCAudioPercent(n int) bool {
	return n >= 0 && n <= 100
}

// SanitizeABCAudioErrorDetail trims and caps error detail to MaxABCAudioErrorDetailBytes.
// Non-printable bytes (except space/tab/newline) are replaced with '?'.
func SanitizeABCAudioErrorDetail(s string) string {
	if s == "" {
		return ""
	}
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '	' || c == '\n' || (c >= 0x20 && c < 0x7f) {
			b = append(b, c)
			continue
		}
		// Allow UTF-8 continuation/start bytes as opaque text, but drop C0/C1 controls.
		if c >= 0x80 {
			b = append(b, c)
			continue
		}
		b = append(b, '?')
	}
	if len(b) > MaxABCAudioErrorDetailBytes {
		b = b[:MaxABCAudioErrorDetailBytes]
	}
	return string(b)
}

// DeriveABCAudioOverallState derives aggregate state in the locked priority order:
// unconfigured, offline, stale, pending, error, device_mismatch, unsupported, partial, applied.
//
// connected and stale are response-time facts supplied by the caller (store may
// leave Connected=false and Stale=false; API/reconcile layers set them).
func DeriveABCAudioOverallState(status ABCAudioStatus) ABCAudioOverallState {
	if status.Desired.Revision == 0 {
		return ABCAudioOverallUnconfigured
	}
	if !status.Connected {
		return ABCAudioOverallOffline
	}
	if status.Stale {
		return ABCAudioOverallStale
	}

	vol := status.Reported.OutputVolumeState
	mute := status.Reported.OutputMuteState
	gain := status.Reported.InputGainState
	if vol == "" {
		vol = ABCAudioControlUnknown
	}
	if mute == "" {
		mute = ABCAudioControlUnknown
	}
	if gain == "" {
		gain = ABCAudioControlUnknown
	}

	// Pending if desired is ahead of a conclusive report, or any control is pending/unknown.
	if status.Desired.Revision > status.Reported.Revision {
		return ABCAudioOverallPending
	}
	if hasControlState(vol, mute, gain, ABCAudioControlPending) ||
		hasControlState(vol, mute, gain, ABCAudioControlUnknown) {
		return ABCAudioOverallPending
	}
	if hasControlState(vol, mute, gain, ABCAudioControlError) {
		return ABCAudioOverallError
	}
	if hasControlState(vol, mute, gain, ABCAudioControlDeviceMismatch) {
		return ABCAudioOverallDeviceMismatch
	}
	if hasControlState(vol, mute, gain, ABCAudioControlUnsupported) {
		// All unsupported → unsupported; mix of applied+unsupported → partial.
		if allControlState(vol, mute, gain, ABCAudioControlUnsupported) {
			return ABCAudioOverallUnsupported
		}
		return ABCAudioOverallPartial
	}
	if allControlState(vol, mute, gain, ABCAudioControlApplied) {
		return ABCAudioOverallApplied
	}
	// Mixed conclusive non-error states (e.g. applied + something else unexpected).
	return ABCAudioOverallPartial
}

func hasControlState(vol, mute, gain, want ABCAudioControlState) bool {
	return vol == want || mute == want || gain == want
}

func allControlState(vol, mute, gain, want ABCAudioControlState) bool {
	return vol == want && mute == want && gain == want
}

// UnconfiguredABCAudioStatus returns the empty status for an existing ABC with
// no audio settings row yet.
func UnconfiguredABCAudioStatus(abcID string, connected bool) *ABCAudioStatus {
	st := &ABCAudioStatus{
		ABCID:     abcID,
		Connected: connected,
		Desired: ABCAudioDesiredView{
			Revision: 0,
		},
		Reported: ABCAudioReportedView{
			Revision:          0,
			OutputVolumeState: ABCAudioControlUnknown,
			OutputMuteState:   ABCAudioControlUnknown,
			InputGainState:    ABCAudioControlUnknown,
			Capabilities:      nil,
		},
		OverallState:     ABCAudioOverallUnconfigured,
		Stale:            false,
		AcceptedRevision: 0,
	}
	return st
}
