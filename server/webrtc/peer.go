package webrtc

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/pion/ice/v4"
	"github.com/pion/webrtc/v4"

)

// PeerState summarizes a peer's current connection state.
type PeerState struct {
	ID              string `json:"id"`
	CreatedAt       time.Time `json:"created_at"`
	ICEState        string `json:"ice_state"`
	DTLSState       string `json:"dtls_state"`
	SignalingState   string `json:"signaling_state"`
	ICEGathering    string `json:"ice_gathering_state"`
	DataChannelOpen bool   `json:"data_channel_open"`
	ClientType      string `json:"client_type,omitempty"`
	ClientName      string `json:"client_name,omitempty"`
	LastOffer       string `json:"last_offer,omitempty"`
	LastAnswer      string `json:"last_answer,omitempty"`
}

// PeerConn wraps a Pion PeerConnection with full event instrumentation.
type PeerConn struct {
	ID        string
	CreatedAt time.Time

	pc      *webrtc.PeerConnection
	events  *EventRing
	control *webrtc.DataChannel

	mu          sync.Mutex
	clientType  string
	clientName  string
	lastOffer   string
	lastAnswer  string
	dcOpen      bool

	// onNegotiationNeeded is called when the server creates a new offer for
	// server-initiated renegotiation.
	onNegotiationNeeded func(offer webrtc.SessionDescription)

	// onRemoteTrack, if set, is invoked (in addition to debug-event emission)
	// whenever a remote media track is received. Used to route inbound audio
	// into a session mixer.
	onRemoteTrack func(*webrtc.TrackRemote, *webrtc.RTPReceiver)

	// onClose, if set, is invoked once when the peer is closed. Used to tear
	// down session-mixer bridges.
	onClose func()
}

// OnClose registers a callback invoked once when the peer connection closes.
func (c *PeerConn) OnClose(f func()) {
	c.mu.Lock()
	c.onClose = f
	c.mu.Unlock()
}

// OnRemoteTrack registers a handler invoked when a remote media track arrives.
// It does not replace the built-in debug-event instrumentation.
func (c *PeerConn) OnRemoteTrack(f func(*webrtc.TrackRemote, *webrtc.RTPReceiver)) {
	c.mu.Lock()
	c.onRemoteTrack = f
	c.mu.Unlock()
}

// AddTrack attaches a local outbound track to the peer connection.
func (c *PeerConn) AddTrack(track webrtc.TrackLocal) (*webrtc.RTPSender, error) {
	return c.pc.AddTrack(track)
}

// Events returns the event ring buffer for this peer.
func (c *PeerConn) Events() *EventRing {
	return c.events
}

// State returns the current summary state.
func (c *PeerConn) State() PeerState {
	c.mu.Lock()
	defer c.mu.Unlock()
	ps := PeerState{
		ID:            c.ID,
		CreatedAt:     c.CreatedAt,
		ClientType:    c.clientType,
		ClientName:    c.clientName,
		DataChannelOpen: c.dcOpen,
		LastOffer:     c.lastOffer,
		LastAnswer:    c.lastAnswer,
	}
	if c.pc != nil {
		ps.ICEState = c.pc.ICEConnectionState().String()
		ps.SignalingState = c.pc.SignalingState().String()
		// DTLS transport state
		for _, t := range c.pc.GetTransceivers() {
			if dt := t.Sender(); dt != nil {
				if tp := dt.Transport(); tp != nil {
					ps.DTLSState = tp.State().String()
					break
				}
			}
		}
		ps.ICEGathering = c.pc.ICEGatheringState().String()
	}
	return ps
}

// PeerConnection returns the underlying Pion PeerConnection (for signaling).
func (c *PeerConn) PeerConnection() *webrtc.PeerConnection {
	return c.pc
}

// HandleOffer sets the remote offer, creates an answer, captures SDP events.
func (c *PeerConn) HandleOffer(offer webrtc.SessionDescription) (webrtc.SessionDescription, error) {
	// Log the offer SDP.
	c.emitSDPEvent(EventSDPOffer, offer)

	// Handle SDP glare: roll back if we have a pending local offer.
	if c.pc.SignalingState() == webrtc.SignalingStateHaveLocalOffer {
		slog.Info("webrtc: rolling back local offer (glare)", "peer", c.ID)
		if err := c.pc.SetLocalDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeRollback}); err != nil {
			return webrtc.SessionDescription{}, fmt.Errorf("webrtc: rollback: %w", err)
		}
	}

	if err := c.pc.SetRemoteDescription(offer); err != nil {
		return webrtc.SessionDescription{}, fmt.Errorf("webrtc: set remote description: %w", err)
	}

	answer, err := c.pc.CreateAnswer(nil)
	if err != nil {
		return webrtc.SessionDescription{}, fmt.Errorf("webrtc: create answer: %w", err)
	}

	if err := c.pc.SetLocalDescription(answer); err != nil {
		return webrtc.SessionDescription{}, fmt.Errorf("webrtc: set local description: %w", err)
	}

	c.emitSDPEvent(EventSDPAnswer, answer)
	return answer, nil
}

// HandleAnswer sets the remote answer (for server-initiated renegotiation).
func (c *PeerConn) HandleAnswer(answer webrtc.SessionDescription) error {
	c.emitSDPEvent(EventSDPAnswer, answer)
	if err := c.pc.SetRemoteDescription(answer); err != nil {
		return fmt.Errorf("webrtc: set remote description (answer): %w", err)
	}
	return nil
}

// AddICECandidate adds a remote ICE candidate and emits an event.
func (c *PeerConn) AddICECandidate(candidate webrtc.ICECandidateInit) error {
	c.events.Push(MakeEvent(c.ID, EventICECandidateRemote, map[string]string{
		"candidate": candidate.Candidate,
	}))
	slog.Debug("webrtc: adding remote ICE candidate",
		"peer", c.ID, "candidate", candidate.Candidate)
	return c.pc.AddICECandidate(candidate)
}

// OnICECandidate registers a callback and instruments it with event emission.
func (c *PeerConn) OnICECandidate(f func(*webrtc.ICECandidate)) {
	c.pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			c.events.Push(MakeEvent(c.ID, EventICECandidateLocal, map[string]string{
				"candidate": candidate.String(),
				"type":      candidate.Typ.String(),
				"address":   candidate.Address,
				"port":      fmt.Sprintf("%d", candidate.Port),
				"protocol":  candidate.Protocol.String(),
			}))
			slog.Debug("webrtc: local ICE candidate",
				"peer", c.ID,
				"type", candidate.Typ.String(),
				"addr", candidate.Address,
				"port", candidate.Port)
		}
		f(candidate)
	})
}

// OnNegotiationNeeded registers the renegotiation callback.
func (c *PeerConn) OnNegotiationNeeded(f func(webrtc.SessionDescription)) {
	c.mu.Lock()
	c.onNegotiationNeeded = f
	c.mu.Unlock()
}

// Negotiate triggers server-initiated renegotiation.
func (c *PeerConn) Negotiate() {
	if c.pc.SignalingState() != webrtc.SignalingStateStable {
		slog.Debug("webrtc: negotiate deferred, not stable", "peer", c.ID)
		return
	}
	c.mu.Lock()
	cb := c.onNegotiationNeeded
	c.mu.Unlock()
	if cb == nil {
		return
	}

	offer, err := c.pc.CreateOffer(nil)
	if err != nil {
		slog.Error("webrtc: create renegotiation offer", "peer", c.ID, "err", err)
		return
	}
	if err := c.pc.SetLocalDescription(offer); err != nil {
		slog.Error("webrtc: set local description for renegotiation", "peer", c.ID, "err", err)
		return
	}
	c.emitSDPEvent(EventSDPOffer, *c.pc.LocalDescription())
	cb(*c.pc.LocalDescription())
}

// Close closes the underlying PeerConnection and emits a connection_closed event.
func (c *PeerConn) Close() error {
	c.events.Push(MakeEvent(c.ID, EventConnectionClosed, nil))
	c.mu.Lock()
	cb := c.onClose
	c.onClose = nil
	c.mu.Unlock()
	if cb != nil {
		cb()
	}
	if c.pc != nil {
		return c.pc.Close()
	}
	return nil
}

// DataChannel returns the control data channel if set.
func (c *PeerConn) DataChannel() *webrtc.DataChannel {
	return c.control
}

// SetClientInfo stores client metadata reported via Hello.
func (c *PeerConn) SetClientInfo(clientType, clientName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clientType = clientType
	c.clientName = clientName
}

// emitSDPEvent records the SDP and emits a debug event with a summary.
func (c *PeerConn) emitSDPEvent(eventType EventType, sdp webrtc.SessionDescription) {
	c.mu.Lock()
	if eventType == EventSDPOffer {
		c.lastOffer = sdp.SDP
	} else {
		c.lastAnswer = sdp.SDP
	}
	c.mu.Unlock()

	// Parse SDP summary for info-level logging.
	lines := strings.Split(sdp.SDP, "\n")
	mediaCount := 0
	var codecs []string
	for _, line := range lines {
		if strings.HasPrefix(line, "m=") {
			mediaCount++
		}
		if strings.HasPrefix(line, "a=rtpmap:") {
			parts := strings.SplitN(line, " ", 2)
			if len(parts) > 1 {
				codecs = append(codecs, strings.TrimSpace(parts[1]))
			}
		}
	}

	detail := map[string]any{
		"type":        sdp.Type.String(),
		"media_lines": mediaCount,
		"codecs":      codecs,
		"sdp_length":  len(sdp.SDP),
	}

	c.events.Push(MakeEvent(c.ID, eventType, detail))
	slog.Info("webrtc: SDP event",
		"peer", c.ID,
		"event", string(eventType),
		"media_lines", mediaCount,
		"codecs", len(codecs),
		"sdp_bytes", len(sdp.SDP))
}

// PeerManager manages WebRTC peer connections with full instrumentation.
type PeerManager struct {
	iceServers []webrtc.ICEServer
	api        *webrtc.API

	mu    sync.Mutex
	peers map[string]*PeerConn
}

// ICEConfig holds ICE-related configuration matching the v2 WebRTCConfig shape.
type ICEConfig struct {
	STUNServers []string
	TURNServer  string
	TURNUser    string
	TURNCred    string
	PublicIP    string
	UDPMuxPort  int
}

// NewPeerManager creates a PeerManager from ICE config.
func NewPeerManager(cfg ICEConfig) *PeerManager {
	servers := make([]webrtc.ICEServer, 0, 2)

	if len(cfg.STUNServers) > 0 {
		servers = append(servers, webrtc.ICEServer{
			URLs: cfg.STUNServers,
		})
	}

	if cfg.TURNServer != "" {
		servers = append(servers, webrtc.ICEServer{
			URLs:           []string{cfg.TURNServer},
			Username:       cfg.TURNUser,
			Credential:     cfg.TURNCred,
			CredentialType: webrtc.ICECredentialTypePassword,
		})
	}

	pm := &PeerManager{
		iceServers: servers,
		peers:      make(map[string]*PeerConn),
	}

	if cfg.UDPMuxPort > 0 {
		bindHost := "0.0.0.0"
		if h := os.Getenv("FLY_UDP_BIND_HOST"); h != "" {
			bindHost = h
		}

		addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", bindHost, cfg.UDPMuxPort))
		if err != nil {
			slog.Error("webrtc: failed to resolve UDP mux address", "error", err)
		} else {
			conn, err := net.ListenUDP("udp", addr)
			if err != nil {
				slog.Error("webrtc: failed to listen UDP mux", "addr", addr, "error", err)
			} else {
				var se webrtc.SettingEngine
				se.SetICEUDPMux(webrtc.NewICEUDPMux(nil, conn))
				se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)

				if cfg.PublicIP != "" {
					se.SetNAT1To1IPs([]string{cfg.PublicIP}, webrtc.ICECandidateTypeHost)
				}

				pm.api = webrtc.NewAPI(webrtc.WithSettingEngine(se))
				slog.Info("webrtc: UDP mux enabled",
					"bind", addr.String(),
					"public_ip", cfg.PublicIP)
			}
		}
	}

	return pm
}

// NewPeerManagerWithAPI creates a PeerManager with a custom webrtc.API (for testing).
func NewPeerManagerWithAPI(api *webrtc.API) *PeerManager {
	return &PeerManager{
		api:   api,
		peers: make(map[string]*PeerConn),
	}
}

// CreatePeerConnection creates a new instrumented PeerConnection.
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
		return nil, fmt.Errorf("webrtc: creating peer connection: %w", err)
	}

	conn := &PeerConn{
		ID:        ulid.Make().String(),
		CreatedAt: time.Now(),
		pc:        pc,
		events:    NewEventRing(DefaultRingSize),
	}

	// Instrument all callbacks for verbose event capture.
	pm.instrumentPeer(conn)

	// Create control data channel.
	dc, err := pc.CreateDataChannel("control", &webrtc.DataChannelInit{})
	if err != nil {
		_ = pc.Close()
		return nil, fmt.Errorf("webrtc: creating control data channel: %w", err)
	}
	conn.control = dc

	dc.OnOpen(func() {
		conn.mu.Lock()
		conn.dcOpen = true
		conn.mu.Unlock()
		conn.events.Push(MakeEvent(conn.ID, EventDataChannelOpen, map[string]string{
			"label": dc.Label(),
		}))
		slog.Info("webrtc: data channel opened", "peer", conn.ID, "label", dc.Label())
	})

	dc.OnClose(func() {
		conn.mu.Lock()
		conn.dcOpen = false
		conn.mu.Unlock()
		conn.events.Push(MakeEvent(conn.ID, EventDataChannelClose, map[string]string{
			"label": dc.Label(),
		}))
		slog.Info("webrtc: data channel closed", "peer", conn.ID, "label", dc.Label())
	})

	dc.OnError(func(err error) {
		conn.events.Push(MakeEvent(conn.ID, EventDataChannelError, map[string]string{
			"error": err.Error(),
		}))
		slog.Error("webrtc: data channel error", "peer", conn.ID, "err", err)
	})

	pm.mu.Lock()
	pm.peers[conn.ID] = conn
	pm.mu.Unlock()

	slog.Info("webrtc: peer connection created", "peer", conn.ID)
	return conn, nil
}

// instrumentPeer registers all Pion callbacks with event emission.
func (pm *PeerManager) instrumentPeer(conn *PeerConn) {
	pc := conn.pc

	// ICE Connection State.
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		conn.events.Push(MakeEvent(conn.ID, EventICEStateChange, map[string]string{
			"state": state.String(),
		}))
		logger := slog.With("peer", conn.ID, "ice_state", state.String())
		switch state {
		case webrtc.ICEConnectionStateConnected:
			logger.Info("webrtc: ICE connected")
		case webrtc.ICEConnectionStateDisconnected:
			logger.Warn("webrtc: ICE disconnected")
		case webrtc.ICEConnectionStateFailed:
			logger.Error("webrtc: ICE failed")
			pm.RemovePeer(conn.ID)
		case webrtc.ICEConnectionStateClosed:
			logger.Info("webrtc: ICE closed")
		default:
			logger.Debug("webrtc: ICE state change")
		}
	})

	// ICE Gathering State.
	pc.OnICEGatheringStateChange(func(state webrtc.ICEGatheringState) {
		conn.events.Push(MakeEvent(conn.ID, EventICEGatheringChange, map[string]string{
			"state": state.String(),
		}))
		slog.Debug("webrtc: ICE gathering state", "peer", conn.ID, "state", state.String())
	})

	// Signaling State.
	pc.OnSignalingStateChange(func(state webrtc.SignalingState) {
		conn.events.Push(MakeEvent(conn.ID, EventSignalingStateChange, map[string]string{
			"state": state.String(),
		}))
		slog.Debug("webrtc: signaling state change", "peer", conn.ID, "state", state.String())
	})

	// Track.
	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		conn.events.Push(MakeEvent(conn.ID, EventTrackAdded, map[string]string{
			"track_id":  track.ID(),
			"stream_id": track.StreamID(),
			"codec":     track.Codec().MimeType,
			"kind":      track.Kind().String(),
		}))
		slog.Info("webrtc: remote track received",
			"peer", conn.ID,
			"track_id", track.ID(),
			"stream_id", track.StreamID(),
			"codec", track.Codec().MimeType,
			"kind", track.Kind().String())

		conn.mu.Lock()
		handler := conn.onRemoteTrack
		conn.mu.Unlock()
		if handler != nil {
			handler(track, receiver)
		}
	})

	// Connection State (DTLS/SCTP).
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		conn.events.Push(MakeEvent(conn.ID, EventDTLSStateChange, map[string]string{
			"state": state.String(),
		}))
		slog.Debug("webrtc: connection state change", "peer", conn.ID, "state", state.String())
	})
}

// RemovePeer closes and removes a peer from the manager.
func (pm *PeerManager) RemovePeer(id string) {
	pm.mu.Lock()
	conn, ok := pm.peers[id]
	if ok {
		delete(pm.peers, id)
	}
	pm.mu.Unlock()

	if ok {
		_ = conn.Close()
	}
}

// FindPeer returns a peer by ID, or nil if not found.
func (pm *PeerManager) FindPeer(id string) *PeerConn {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.peers[id]
}

// ListPeers returns a snapshot of all active peers.
func (pm *PeerManager) ListPeers() []*PeerConn {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	out := make([]*PeerConn, 0, len(pm.peers))
	for _, p := range pm.peers {
		out = append(out, p)
	}
	return out
}

// Count returns the number of active peers.
func (pm *PeerManager) Count() int {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return len(pm.peers)
}

// PeerStates returns state summaries for all peers (for the debug API).
func (pm *PeerManager) PeerStates() []PeerState {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	out := make([]PeerState, 0, len(pm.peers))
	for _, p := range pm.peers {
		out = append(out, p.State())
	}
	return out
}

// PeerEvents returns events for a specific peer. Returns nil, false if not found.
func (pm *PeerManager) PeerEvents(id string) ([]Event, bool) {
	pm.mu.Lock()
	p, ok := pm.peers[id]
	pm.mu.Unlock()
	if !ok {
		return nil, false
	}
	return p.Events().Snapshot(), true
}

// PeerSDP returns the last offer/answer for a specific peer.
func (pm *PeerManager) PeerSDP(id string) (offer, answer string, found bool) {
	pm.mu.Lock()
	p, ok := pm.peers[id]
	pm.mu.Unlock()
	if !ok {
		return "", "", false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastOffer, p.lastAnswer, true
}

// DebugPeerDetail returns full detail for a single peer.
func (pm *PeerManager) DebugPeerDetail(id string) (json.RawMessage, bool) {
	pm.mu.Lock()
	p, ok := pm.peers[id]
	pm.mu.Unlock()
	if !ok {
		return nil, false
	}

	state := p.State()
	data, _ := json.Marshal(state)
	return data, true
}
