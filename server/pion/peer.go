package pion

import (
	"fmt"
	"sync"

	"github.com/oklog/ulid/v2"
	"github.com/pion/webrtc/v4"
	"google.golang.org/protobuf/proto"

	crosstalk "github.com/aleksclark/crosstalk/server"
	crosstalkv1 "github.com/aleksclark/crosstalk/proto/gen/go/crosstalk/v1"
)

// PeerManager creates and tracks WebRTC peer connections using the Pion library.
// It holds the ICE server configuration derived from [crosstalk.WebRTCConfig].
type PeerManager struct {
	iceServers []webrtc.ICEServer

	// api is an optional *webrtc.API used to create peer connections. When nil,
	// the default webrtc.NewPeerConnection path is used instead. Tests inject a
	// custom API (e.g. with mDNS disabled) via newPeerManagerWithAPI.
	api *webrtc.API

	mu    sync.Mutex
	peers map[string]*PeerConn
}

// NewPeerManager returns a PeerManager configured with STUN/TURN servers from cfg.
func NewPeerManager(cfg crosstalk.WebRTCConfig) *PeerManager {
	servers := make([]webrtc.ICEServer, 0, 2)

	if len(cfg.STUNServers) > 0 {
		servers = append(servers, webrtc.ICEServer{
			URLs: cfg.STUNServers,
		})
	}

	if cfg.TURN.Enabled && cfg.TURN.Server != "" {
		servers = append(servers, webrtc.ICEServer{
			URLs:           []string{cfg.TURN.Server},
			Username:       cfg.TURN.Username,
			Credential:     cfg.TURN.Credential,
			CredentialType: webrtc.ICECredentialTypePassword,
		})
	}

	return &PeerManager{
		iceServers: servers,
		peers:      make(map[string]*PeerConn),
	}
}

// NewPeerManagerWithAPI is like NewPeerManager but accepts a custom webrtc.API,
// which is useful for testing (e.g. disabling mDNS).
func NewPeerManagerWithAPI(cfg crosstalk.WebRTCConfig, api *webrtc.API) *PeerManager {
	pm := NewPeerManager(cfg)
	pm.api = api
	return pm
}

// CreatePeerConnection creates a new Pion PeerConnection with the configured
// ICE servers, wraps it in a [PeerConn], registers it in the manager, and
// creates the server-side "control" data channel.
func (pm *PeerManager) CreatePeerConnection() (*PeerConn, error) {
	rtcCfg := webrtc.Configuration{
		ICEServers: pm.iceServers,
	}

	var (
		pc  *webrtc.PeerConnection
		err error
	)

	if pm.api != nil {
		pc, err = pm.api.NewPeerConnection(rtcCfg)
	} else {
		pc, err = webrtc.NewPeerConnection(rtcCfg)
	}
	if err != nil {
		return nil, fmt.Errorf("pion: creating peer connection: %w", err)
	}

	conn := &PeerConn{
		ID: ulid.Make().String(),
		pc: pc,
	}

	// Create the server-owned "control" data channel.
	if err := conn.createControlChannel(); err != nil {
		pc.Close() //nolint:errcheck
		return nil, fmt.Errorf("pion: creating control data channel: %w", err)
	}

	pm.mu.Lock()
	pm.peers[conn.ID] = conn
	pm.mu.Unlock()

	return conn, nil
}

// RemovePeer closes the peer connection identified by id and removes it from
// the manager. It is a no-op if the id is not found.
func (pm *PeerManager) RemovePeer(id string) {
	pm.mu.Lock()
	conn, ok := pm.peers[id]
	if ok {
		delete(pm.peers, id)
	}
	pm.mu.Unlock()

	if ok {
		conn.Close() //nolint:errcheck
	}
}

// Count returns the number of active peer connections.
func (pm *PeerManager) Count() int {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return len(pm.peers)
}

// PeerConn wraps a Pion [webrtc.PeerConnection] with a unique ID and a
// server-owned control data channel.
type PeerConn struct {
	// ID is a ULID that uniquely identifies this peer connection.
	ID string

	pc      *webrtc.PeerConnection
	control *webrtc.DataChannel

	// Client capabilities reported via Hello.
	mu      sync.Mutex
	Sources []*crosstalkv1.SourceInfo
	Sinks   []*crosstalkv1.SinkInfo
	Codecs  []*crosstalkv1.CodecInfo

	// Session membership set by JoinSession.
	SessionID string
	Role      string
}

// OnICEConnectionStateChange registers a callback invoked when the ICE
// connection state changes (e.g. connected, disconnected, failed).
func (c *PeerConn) OnICEConnectionStateChange(f func(webrtc.ICEConnectionState)) {
	c.pc.OnICEConnectionStateChange(f)
}

// HandleOffer sets the remote offer SDP, creates an answer, sets it as the
// local description, and returns the answer.
func (c *PeerConn) HandleOffer(offer webrtc.SessionDescription) (webrtc.SessionDescription, error) {
	if err := c.pc.SetRemoteDescription(offer); err != nil {
		return webrtc.SessionDescription{}, fmt.Errorf("pion: set remote description: %w", err)
	}

	answer, err := c.pc.CreateAnswer(nil)
	if err != nil {
		return webrtc.SessionDescription{}, fmt.Errorf("pion: create answer: %w", err)
	}

	if err := c.pc.SetLocalDescription(answer); err != nil {
		return webrtc.SessionDescription{}, fmt.Errorf("pion: set local description: %w", err)
	}

	return answer, nil
}

// AddICECandidate adds a remote ICE candidate to the underlying peer connection.
func (c *PeerConn) AddICECandidate(candidate webrtc.ICECandidateInit) error {
	return c.pc.AddICECandidate(candidate)
}

// OnICECandidate registers a callback invoked when a new local ICE candidate
// is discovered. A nil candidate signals that gathering is complete.
func (c *PeerConn) OnICECandidate(f func(*webrtc.ICECandidate)) {
	c.pc.OnICECandidate(f)
}

// SendControlMessage marshals a protobuf ControlMessage and sends it on the
// control data channel.
func (c *PeerConn) SendControlMessage(msg *crosstalkv1.ControlMessage) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("pion: marshal control message: %w", err)
	}
	return c.control.Send(data)
}

// Close closes the underlying Pion PeerConnection and any associated data
// channels.
func (c *PeerConn) Close() error {
	return c.pc.Close()
}

// boolPtr returns a pointer to b.
func boolPtr(b bool) *bool {
	return &b
}
