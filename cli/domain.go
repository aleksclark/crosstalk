// Package crosstalk defines the core domain types for the CrossTalk CLI client.
// This package contains no external dependencies. Implementations live in
// subpackages grouped by dependency (pipewire/, pion/).
package crosstalk

import (
	"context"
	"encoding/json"
)

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
	Type      string          `json:"type"`                // "offer", "answer", "ice"
	SDP       string          `json:"sdp,omitempty"`       // SDP payload for offer/answer
	Candidate json.RawMessage `json:"candidate,omitempty"` // ICE candidate (object from server or string)
}

// WebRTCTokenResponse is the response from POST /api/webrtc/token.
type WebRTCTokenResponse struct {
	Token string `json:"token"`
}

// WiFiNetwork describes a wireless network discovered during a scan.
type WiFiNetwork struct {
	SSID    string // network name (empty SSIDs are hidden networks)
	Signal  int    // signal strength, 0-100
	Secured bool   // true if the network requires a passphrase
	Active  bool   // true if the device is currently connected to it
}

// WiFiManager controls the device's wireless connectivity. Implementations
// wrap a platform network manager (e.g. nmcli/NetworkManager). It is used by
// the captive-portal provisioning flow that runs when the box cannot reach the
// configured WiFi network.
type WiFiManager interface {
	// Online reports whether the device currently has full connectivity via a
	// station (client) wireless connection.
	Online(ctx context.Context) bool
	// Scan returns the wireless networks currently visible to the device,
	// sorted by descending signal strength.
	Scan(ctx context.Context) ([]WiFiNetwork, error)
	// Connect joins the given network, replacing any prior saved profile for
	// that SSID. It blocks until the connection is established or fails.
	// An empty passphrase joins an open network.
	Connect(ctx context.Context, ssid, passphrase string) error
	// StartHotspot brings up a local access point with the given SSID and
	// passphrase so a phone can reach the captive portal.
	StartHotspot(ctx context.Context, ssid, passphrase string) error
	// StopHotspot tears down the access point started by StartHotspot.
	StopHotspot(ctx context.Context) error
}

// EthernetManager controls the device's wired network interface.
// Implementations wrap a platform network manager (e.g. nmcli/NetworkManager).
// It is used by the captive-portal provisioning flow to bring up wired
// ethernet in DHCP mode as a fallback management/connectivity path when WiFi
// cannot connect.
type EthernetManager interface {
	// EnableDHCP brings up the wired ethernet interface as a DHCP client. It
	// returns the interface name it acted on. If no wired device exists it
	// returns an empty name and a nil error (nothing to do).
	EnableDHCP(ctx context.Context) (iface string, err error)
	// EthernetStatus reports the wired interface state. up is true when a wired
	// device has an IPv4 address; iface and ipv4 identify it.
	EthernetStatus(ctx context.Context) (iface, ipv4 string, up bool)
}

// ValidationError represents a configuration validation failure.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}
