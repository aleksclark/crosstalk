package abc

import (
	"log/slog"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

// DefaultHelloCapabilities is the Opus offer advertised in Hello. These
// fields describe what the client offers; they do not select the server codec.
var DefaultHelloCapabilities = []AudioCapability{
	{Codec: "opus/48000/2", Channels: 2, SampleRate: 48000},
	{Codec: "opus/48000/1", Channels: 1, SampleRate: 48000},
}

// Config configures a single ABC transport session.
type Config struct {
	// ServerURL is the Crosstalk HTTP origin, for example https://host:8443.
	ServerURL string
	// Token is the long-lived ABC credential. It is never logged.
	Token string
	// ClientName is advertised in Hello.client_name. Defaults to "abc".
	ClientName string
	// PublishTrackID is the local send-track id. Defaults to "abc-mic".
	PublishTrackID string
	// WelcomeTimeout bounds Dial's wait for Welcome. Defaults to 5s.
	WelcomeTimeout time.Duration
	// RequireWelcome makes Dial fail when Welcome does not arrive.
	RequireWelcome bool
	// Logger receives structured events. Tokens are never written.
	Logger *slog.Logger
	// ICEServers overrides the default STUN server. A nil value uses
	// Google's public STUN server. An empty slice disables STUN.
	ICEServers []webrtc.ICEServer
	// DisableMDNS turns off ICE mDNS gathering. Tests should set this.
	DisableMDNS bool
}

// State is a coarse connection lifecycle state.
type State string

const (
	StateDialing    State = "dialing"
	StateConnected  State = "connected"
	StateFailed     State = "failed"
	StateClosed     State = "closed"
)

// Welcome is the server Hello acknowledgement for one connection epoch.
type Welcome struct {
	PeerID            string
	ServerVersion     string
	AssignedSessionID string
	Epoch             uint64
}

// Codec is the SDP-selected audio codec for a track.
type Codec struct {
	MimeType    string
	ClockRate   uint32
	Channels    uint16
	PayloadType uint8
	SDPFmtpLine string
}

// RTPWriter accepts encoded RTP packets for the local send track.
type RTPWriter interface {
	WriteRTP(pkt *rtp.Packet) error
}

// IncomingTrack is an authorized remote audio track.
type IncomingTrack struct {
	ID       string
	StreamID string
	Codec    Codec
	// Pion is the underlying remote track. Device adapters may use it;
	// application code should prefer ReadRTP.
	Pion *webrtc.TrackRemote
}

// ReadRTP reads the next RTP packet from the remote track.
func (t IncomingTrack) ReadRTP() (*rtp.Packet, error) {
	if t.Pion == nil {
		return nil, errTrackClosed
	}
	pkt, _, err := t.Pion.ReadRTP()
	return pkt, err
}

// ICEState is a portable ICE connection state name.
type ICEState string

const (
	ICEStateNew          ICEState = "new"
	ICEStateChecking     ICEState = "checking"
	ICEStateConnected    ICEState = "connected"
	ICEStateCompleted    ICEState = "completed"
	ICEStateDisconnected ICEState = "disconnected"
	ICEStateFailed       ICEState = "failed"
	ICEStateClosed       ICEState = "closed"
	ICEStateUnknown      ICEState = "unknown"
)

// AudioCapability is a codec the client offers in Hello.
type AudioCapability struct {
	Codec      string
	Channels   int32
	SampleRate int32
}

// RestartCommand instructs the client to reconnect.
type RestartCommand struct {
	Reason string
}

// SessionAssignment informs the client of a session role.
type SessionAssignment struct {
	SessionID string
	Role      string
}

// AudioApplyState is a mixer apply/readback result.
type AudioApplyState int

const (
	AudioApplyUnspecified    AudioApplyState = 0
	AudioApplyApplied        AudioApplyState = 1
	AudioApplyUnsupported    AudioApplyState = 2
	AudioApplyError          AudioApplyState = 3
	AudioApplyDeviceMismatch AudioApplyState = 4
	AudioApplyStaleRevision  AudioApplyState = 5
)

// AudioControlCommand is an absolute desired USB mixer state.
type AudioControlCommand struct {
	CommandID       string
	DesiredRevision uint64
	Output          *AudioOutputDesired
	Input           *AudioInputDesired
}

// AudioOutputDesired is the absolute desired playback mixer state.
type AudioOutputDesired struct {
	DeviceUID     string
	VolumePercent *uint32
	Muted         *bool
}

// AudioInputDesired is the absolute desired capture mixer state.
type AudioInputDesired struct {
	DeviceUID   string
	GainPercent *uint32
}

// AudioControlReport is inventory and/or apply result.
type AudioControlReport struct {
	CommandID       string
	DesiredRevision uint64
	Devices         []AudioDeviceCapability
	Output          *AudioOutputObserved
	Input           *AudioInputObserved
	ErrorCode       string
	ErrorDetail     string
}

// AudioDeviceCapability describes one discovered audio endpoint.
type AudioDeviceCapability struct {
	DeviceUID          string
	Direction          string
	Backend            string
	VendorID           string
	ProductID          string
	Serial             string
	Path               string
	CardID             string
	CardName           string
	PCMRoute           string
	SupportsVolume     bool
	SupportsMute       bool
	SupportsGain       bool
	SupportsAGCDisable bool
	VolumeMinDB        *int32
	VolumeMaxDB        *int32
	VolumeStepDB       *int32
	GainMinDB          *int32
	GainMaxDB          *int32
	GainStepDB         *int32
}

// AudioOutputObserved is the effective playback mixer readback.
type AudioOutputObserved struct {
	DeviceUID     string
	VolumePercent *uint32
	Muted         *bool
	VolumeState   AudioApplyState
	MuteState     AudioApplyState
}

// AudioInputObserved is the effective capture mixer readback.
type AudioInputObserved struct {
	DeviceUID   string
	GainPercent *uint32
	GainState   AudioApplyState
}

// ControlMessage is a decoded v2 control-channel payload.
type ControlMessage struct {
	Welcome             *Welcome
	Restart             *RestartCommand
	SessionAssignment   *SessionAssignment
	AudioControlCommand *AudioControlCommand
	AudioControlReport  *AudioControlReport
}
