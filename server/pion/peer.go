package pion

import (
	"fmt"
	"log/slog"
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

	// Monitor ICE connection state and log transitions. On disconnected,
	// failed, or closed states, remove the peer from the registry to free
	// resources. In the SFU model, ICE disconnection is effectively terminal
	// — reconnection is handled at the WebSocket/signaling layer.
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		logger := slog.With("peer_id", conn.ID, "ice_state", state.String())
		switch state {
		case webrtc.ICEConnectionStateConnected:
			logger.Info("peer ICE connected")
		case webrtc.ICEConnectionStateDisconnected:
			logger.Warn("peer ICE disconnected, removing peer")
			pm.RemovePeer(conn.ID)
		case webrtc.ICEConnectionStateFailed:
			logger.Error("peer ICE failed, removing peer")
			pm.RemovePeer(conn.ID)
		case webrtc.ICEConnectionStateClosed:
			logger.Info("peer ICE closed")
		default:
			logger.Debug("peer ICE state change")
		}
	})

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

// ListPeers returns a snapshot of all active peer connections.
func (pm *PeerManager) ListPeers() []*PeerConn {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	out := make([]*PeerConn, 0, len(pm.peers))
	for _, p := range pm.peers {
		out = append(out, p)
	}
	return out
}

// FindPeer returns the peer with the given ID, or nil if not found.
func (pm *PeerManager) FindPeer(id string) *PeerConn {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.peers[id]
}

// ListPeerInfo returns a summary of all active peers for the REST API.
func (pm *PeerManager) ListPeerInfo() []crosstalk.PeerInfo {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	out := make([]crosstalk.PeerInfo, 0, len(pm.peers))
	for _, p := range pm.peers {
		out = append(out, crosstalk.PeerInfo{
			ID:        p.ID,
			SessionID: p.SessionID,
			Role:      p.Role,
		})
	}
	return out
}

// PeerConn wraps a Pion [webrtc.PeerConnection] with a unique ID and a
// server-owned control data channel.
type PeerConn struct {
	// ID is a ULID that uniquely identifies this peer connection.
	ID string

	pc      *webrtc.PeerConnection
	control *webrtc.DataChannel

	// clientControl is the control data channel created by the remote client.
	// WebRTC data channels are matched by label, but when both sides create a
	// channel named "control" they get two independent channels. The server
	// installs its handler on the server-created one (control) but the client
	// sends messages on its own — which arrives here via OnDataChannel.
	clientControl *webrtc.DataChannel

	// jsonMode indicates the peer speaks JSON on the control channel instead
	// of protobuf. Detected automatically on the first message: if protobuf
	// unmarshal fails but JSON parse succeeds, the peer is switched to JSON
	// mode and all subsequent outbound messages are sent as JSON.
	jsonMode bool

	// onControlMessage is an optional callback invoked for every message
	// received on any control data channel (server-created or client-created).
	// Set by ControlHandler.Install().
	onControlMessage func(data []byte)

	// Client capabilities reported via Hello.
	mu      sync.Mutex
	Sources []*crosstalkv1.SourceInfo
	Sinks   []*crosstalkv1.SinkInfo
	Codecs  []*crosstalkv1.CodecInfo

	// Session membership set by JoinSession.
	SessionID string
	Role      string

	// onNegotiationNeeded is called when the server adds/removes tracks and the
	// client must renegotiate. Set by the signaling layer before any bindings
	// are activated.
	onNegotiationNeeded func(offer webrtc.SessionDescription)
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

// OnNegotiationNeeded registers a callback invoked when the server adds or
// removes tracks and the client must renegotiate. The callback receives a
// server-created SDP offer that should be forwarded to the client.
func (c *PeerConn) OnNegotiationNeeded(f func(webrtc.SessionDescription)) {
	c.mu.Lock()
	c.onNegotiationNeeded = f
	c.mu.Unlock()
}

// Negotiate triggers renegotiation after adding or removing tracks.
//
// The server creates a new SDP offer and delivers it via the
// OnNegotiationNeeded callback. For WebSocket/JSON peers, the signaling layer
// forwards the offer over the WebSocket and the browser answers. For protobuf
// peers, the offer is delivered via the protocol-specific transport. The client
// answers via HandleAnswer.
//
// Errors are logged, not returned, because renegotiation is best-effort.
func (c *PeerConn) Negotiate() {
	sigState := c.pc.SignalingState()
	slog.Info("pion: negotiate called",
		"peer", c.ID,
		"signaling_state", sigState.String())

	c.mu.Lock()
	cb := c.onNegotiationNeeded
	c.mu.Unlock()

	if cb == nil {
		slog.Debug("pion: negotiate called but no callback registered", "peer", c.ID)
		return
	}

	slog.Info("pion: creating renegotiation offer",
		"peer", c.ID, "signaling_state", sigState.String())

	offer, err := c.pc.CreateOffer(nil)
	if err != nil {
		slog.Error("pion: create renegotiation offer failed",
			"peer", c.ID,
			"signaling_state", sigState.String(),
			"err", err)
		return
	}

	slog.Info("pion: renegotiation offer created", "peer", c.ID)

	if err := c.pc.SetLocalDescription(offer); err != nil {
		slog.Error("pion: set local description for renegotiation",
			"peer", c.ID, "err", err)
		return
	}

	cb(*c.pc.LocalDescription())
}

// HandleAnswer sets the remote answer SDP. This is used for server-initiated
// renegotiation: the server sends an offer, and the client responds with an
// answer that is applied here.
func (c *PeerConn) HandleAnswer(answer webrtc.SessionDescription) error {
	if err := c.pc.SetRemoteDescription(answer); err != nil {
		return fmt.Errorf("pion: set remote description (answer): %w", err)
	}
	return nil
}

// SendControlMessage marshals a ControlMessage and sends it on the control
// data channel. When the peer has been detected as a JSON-speaking client
// (browser), the message is serialised as JSON instead of protobuf.
func (c *PeerConn) SendControlMessage(msg *crosstalkv1.ControlMessage) error {
	c.mu.Lock()
	useJSON := c.jsonMode
	c.mu.Unlock()

	var data []byte
	var err error

	if useJSON {
		data, err = controlMessageToJSON(msg)
		if err != nil {
			return fmt.Errorf("pion: marshal control message as JSON: %w", err)
		}
		return c.sendControlRaw(data)
	}

	data, err = proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("pion: marshal control message: %w", err)
	}
	return c.sendControlRaw(data)
}

// sendControlRaw sends raw bytes on whichever control data channel is
// available, preferring the client-created one (since the client listens on
// the DC it created). When the peer is in JSON mode, data is sent as a text
// frame so browser clients receive it as a string (not ArrayBuffer).
func (c *PeerConn) sendControlRaw(data []byte) error {
	c.mu.Lock()
	dc := c.clientControl
	if dc == nil {
		dc = c.control
	}
	useJSON := c.jsonMode
	c.mu.Unlock()

	dcLabel := "unknown"
	if dc == c.clientControl {
		dcLabel = "client"
	} else if dc == c.control {
		dcLabel = "server"
	}

	if dc == nil {
		return fmt.Errorf("pion: no control data channel available")
	}
	if useJSON {
		slog.Debug("pion: sendControlRaw (text)", "peer", c.ID, "dc", dcLabel, "bytes", len(data))
		return dc.SendText(string(data))
	}
	return dc.Send(data)
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
