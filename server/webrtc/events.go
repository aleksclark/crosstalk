// Package webrtc implements the v3 WebRTC layer with verbose debug logging
// as a first-class feature. Every ICE/DTLS/signaling event is captured in a
// per-peer ring buffer and queryable via debug API endpoints.
package webrtc

import (
	"encoding/json"
	"sync"
	"time"
)

// EventType enumerates all captured WebRTC lifecycle events.
type EventType string

const (
	EventICECandidateLocal    EventType = "ice_candidate_local"
	EventICECandidateRemote   EventType = "ice_candidate_remote"
	EventICEStateChange       EventType = "ice_state_change"
	EventICEGatheringChange   EventType = "ice_gathering_change"
	EventDTLSStateChange      EventType = "dtls_state_change"
	EventSignalingStateChange EventType = "signaling_state_change"
	EventSDPOffer             EventType = "sdp_offer"
	EventSDPAnswer            EventType = "sdp_answer"
	EventTrackAdded           EventType = "track_added"
	EventTrackRemoved         EventType = "track_removed"
	EventDataChannelOpen      EventType = "data_channel_open"
	EventDataChannelClose     EventType = "data_channel_close"
	EventDataChannelError     EventType = "data_channel_error"
	EventDataChannelMessage   EventType = "data_channel_message"
	EventConnectionClosed     EventType = "connection_closed"
	EventControlHello         EventType = "control_hello"
	EventControlWelcome       EventType = "control_welcome"
	EventControlPing          EventType = "control_ping"
	EventControlPong          EventType = "control_pong"
	EventSignalingMessage     EventType = "signaling_message"
)

// Event represents a single captured WebRTC lifecycle event.
type Event struct {
	Timestamp time.Time       `json:"timestamp"`
	PeerID    string          `json:"peer_id"`
	Type      EventType       `json:"type"`
	Detail    json.RawMessage `json:"detail"`
}

// DefaultRingSize is the number of events kept per peer connection.
const DefaultRingSize = 200

// EventRing is a fixed-size ring buffer of Events. It is safe for concurrent
// use from multiple goroutines.
type EventRing struct {
	mu     sync.Mutex
	events []Event
	size   int
	head   int // next write position
	count  int // total events written (for knowing if wrapped)
}

// NewEventRing creates a ring buffer that holds up to capacity events.
func NewEventRing(capacity int) *EventRing {
	if capacity <= 0 {
		capacity = DefaultRingSize
	}
	return &EventRing{
		events: make([]Event, capacity),
		size:   capacity,
	}
}

// Push adds an event to the ring buffer. If the buffer is full, the oldest
// event is overwritten.
func (r *EventRing) Push(e Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events[r.head] = e
	r.head = (r.head + 1) % r.size
	r.count++
}

// Snapshot returns a copy of all events in chronological order (oldest first).
func (r *EventRing) Snapshot() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()

	n := r.count
	if n > r.size {
		n = r.size
	}

	out := make([]Event, n)
	if r.count <= r.size {
		// Not yet wrapped — events are in [0..head).
		copy(out, r.events[:n])
	} else {
		// Wrapped — oldest is at head, read from head to end, then 0 to head.
		first := r.size - r.head
		copy(out[:first], r.events[r.head:])
		copy(out[first:], r.events[:r.head])
	}
	return out
}

// Len returns the number of events currently stored.
func (r *EventRing) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.count > r.size {
		return r.size
	}
	return r.count
}

// Total returns the total number of events ever pushed (including overwritten).
func (r *EventRing) Total() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count
}

// MakeEvent creates an Event with the current timestamp and a JSON-serialized detail.
func MakeEvent(peerID string, eventType EventType, detail any) Event {
	var raw json.RawMessage
	if detail != nil {
		data, err := json.Marshal(detail)
		if err != nil {
			raw = json.RawMessage(`{"error":"marshal failed"}`)
		} else {
			raw = data
		}
	} else {
		raw = json.RawMessage(`{}`)
	}
	return Event{
		Timestamp: time.Now(),
		PeerID:    peerID,
		Type:      eventType,
		Detail:    raw,
	}
}
