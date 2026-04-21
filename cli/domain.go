// Package crosstalk defines the core domain types for the CrossTalk CLI client.
// This package contains no external dependencies. Implementations live in
// subpackages grouped by dependency (pipewire/, pion/).
package crosstalk

// Config holds the CLI client configuration.
type Config struct {
	ServerURL  string `json:"server_url"`
	Token      string `json:"token"`
	SourceName string `json:"source_name,omitempty"`
	SinkName   string `json:"sink_name,omitempty"`
	LogLevel   string `json:"log_level,omitempty"`
}

// Source represents an audio source (input device).
type Source struct {
	Name string
	Type string // "audio" or "video"
}

// Sink represents an audio sink (output device).
type Sink struct {
	Name string
	Type string // "audio" or "video"
}

// Codec represents a supported audio/video codec.
type Codec struct {
	Name      string // e.g. "opus/48000/2"
	MediaType string // "audio" or "video"
}

// PipeWireService defines operations for discovering PipeWire audio devices.
type PipeWireService interface {
	// Discover returns the available audio sources and sinks.
	Discover() ([]Source, []Sink, error)
}

// SignalingMessage is a WebSocket signaling message for WebRTC negotiation.
type SignalingMessage struct {
	Type      string `json:"type"`                // "offer", "answer", "ice"
	SDP       string `json:"sdp,omitempty"`       // SDP payload for offer/answer
	Candidate string `json:"candidate,omitempty"` // ICE candidate string
}

// WebRTCTokenResponse is the response from POST /api/webrtc/token.
type WebRTCTokenResponse struct {
	Token string `json:"token"`
}

// ValidationError represents a configuration validation failure.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}
