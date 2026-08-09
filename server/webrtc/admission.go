package webrtc

import (
	"errors"
	"fmt"
)

// Default admission bounds applied when a limit field is zero.
const (
	// DefaultMaxSDPBytes is the max accepted SDP body size (offer/answer).
	DefaultMaxSDPBytes = 64 << 10 // 64 KiB

	// DefaultMaxRemoteICECandidates is the max remote ICE candidates per peer.
	DefaultMaxRemoteICECandidates = 64

	// DefaultMaxPeers is unlimited (0). Callers may set a positive cap.
	DefaultMaxPeers = 0
)

// Sentinel errors for admission rejection. Callers can errors.Is these.
var (
	ErrSDPTooLarge              = errors.New("webrtc: SDP exceeds maximum size")
	ErrTooManyRemoteICE         = errors.New("webrtc: too many remote ICE candidates")
	ErrTooManyPeers             = errors.New("webrtc: peer limit reached")
	ErrPeerManagerClosed        = errors.New("webrtc: peer manager is closed")
	ErrSignalingMessageTooLarge = errors.New("webrtc: signaling message exceeds maximum size")
)

// AdmissionLimits bounds signaling and peer-creation work.
// Zero-valued fields fall back to the Default* constants via WithDefaults.
// MaxPeers == 0 means unlimited even after WithDefaults.
type AdmissionLimits struct {
	// MaxPeers caps concurrent peers in the manager. 0 = unlimited.
	MaxPeers int

	// MaxSDPBytes caps offer/answer SDP payload size. 0 → DefaultMaxSDPBytes.
	MaxSDPBytes int

	// MaxRemoteICECandidates caps trickled remote ICE candidates per peer.
	// 0 → DefaultMaxRemoteICECandidates.
	MaxRemoteICECandidates int

	// MaxSignalingMessageBytes caps a single raw WebSocket signaling frame.
	// 0 → MaxSDPBytes + 4 KiB envelope headroom after defaults are applied.
	MaxSignalingMessageBytes int
}

// WithDefaults returns a copy with zero fields filled in.
func (l AdmissionLimits) WithDefaults() AdmissionLimits {
	if l.MaxSDPBytes <= 0 {
		l.MaxSDPBytes = DefaultMaxSDPBytes
	}
	if l.MaxRemoteICECandidates <= 0 {
		l.MaxRemoteICECandidates = DefaultMaxRemoteICECandidates
	}
	// MaxPeers stays 0 (unlimited) unless explicitly set.
	if l.MaxSignalingMessageBytes <= 0 {
		// Envelope JSON around SDP is small; leave headroom above MaxSDPBytes.
		l.MaxSignalingMessageBytes = l.MaxSDPBytes + 4<<10
	}
	return l
}

// CheckSDPSize rejects oversized SDP before SetRemoteDescription / parsing work.
func CheckSDPSize(sdp string, maxBytes int) error {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxSDPBytes
	}
	if len(sdp) > maxBytes {
		return fmt.Errorf("%w: %d > %d", ErrSDPTooLarge, len(sdp), maxBytes)
	}
	return nil
}

// CheckSignalingMessageSize rejects oversized raw signaling frames early.
func CheckSignalingMessageSize(n, maxBytes int) error {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxSDPBytes + 4<<10
	}
	if n > maxBytes {
		return fmt.Errorf("%w: %d > %d", ErrSignalingMessageTooLarge, n, maxBytes)
	}
	return nil
}

// CheckPeerCount rejects when the manager is at capacity.
// maxPeers <= 0 means unlimited.
func CheckPeerCount(current, maxPeers int) error {
	if maxPeers <= 0 {
		return nil
	}
	if current >= maxPeers {
		return fmt.Errorf("%w: %d >= %d", ErrTooManyPeers, current, maxPeers)
	}
	return nil
}

// CheckRemoteICECount rejects when a peer has received too many remote candidates.
func CheckRemoteICECount(current, maxCandidates int) error {
	if maxCandidates <= 0 {
		maxCandidates = DefaultMaxRemoteICECandidates
	}
	if current >= maxCandidates {
		return fmt.Errorf("%w: %d >= %d", ErrTooManyRemoteICE, current, maxCandidates)
	}
	return nil
}
