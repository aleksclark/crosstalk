package pion

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	crosstalk "github.com/aleksclark/crosstalk/cli"
	"github.com/pion/webrtc/v4"
	"nhooyr.io/websocket"
)

// Connection manages the WebSocket signaling and WebRTC peer connection.
type Connection struct {
	mu sync.Mutex

	serverURL  string
	webrtcToken string

	wsConn   *websocket.Conn
	peerConn *webrtc.PeerConnection
	controlDC *webrtc.DataChannel

	// Track bindings: channel_id → bound track info
	boundTracks map[string]*BoundTrack

	// onControlOpen is called when the control data channel opens.
	onControlOpen func()
	// onControlMessage is called when a message is received on the control channel.
	onControlMessage func([]byte)
	// onConnectionStateChange is called on ICE connection state changes.
	onConnectionStateChange func(webrtc.ICEConnectionState)
	// onBindChannel is called when a BindChannel message is received.
	onBindChannel func(*BindChannelMsg)
	// onUnbindChannel is called when an UnbindChannel message is received.
	onUnbindChannel func(*UnbindChannelMsg)

	ctx    context.Context
	cancel context.CancelFunc
}

// BoundTrack represents an active track binding for a channel.
type BoundTrack struct {
	ChannelID string
	TrackID   string
	LocalName string
	Direction Direction
	Track     *webrtc.TrackLocalStaticRTP
	Sender    *webrtc.RTPSender
	stopFn    func()
}

// ConnectionOption configures a Connection.
type ConnectionOption func(*Connection)

// WithOnControlOpen sets the callback for when the control channel opens.
func WithOnControlOpen(fn func()) ConnectionOption {
	return func(c *Connection) {
		c.onControlOpen = fn
	}
}

// WithOnControlMessage sets the callback for control channel messages.
func WithOnControlMessage(fn func([]byte)) ConnectionOption {
	return func(c *Connection) {
		c.onControlMessage = fn
	}
}

// WithOnConnectionStateChange sets the callback for connection state changes.
func WithOnConnectionStateChange(fn func(webrtc.ICEConnectionState)) ConnectionOption {
	return func(c *Connection) {
		c.onConnectionStateChange = fn
	}
}

// WithOnBindChannel sets the callback for BindChannel messages.
func WithOnBindChannel(fn func(*BindChannelMsg)) ConnectionOption {
	return func(c *Connection) {
		c.onBindChannel = fn
	}
}

// WithOnUnbindChannel sets the callback for UnbindChannel messages.
func WithOnUnbindChannel(fn func(*UnbindChannelMsg)) ConnectionOption {
	return func(c *Connection) {
		c.onUnbindChannel = fn
	}
}

// NewConnection creates a new Connection.
func NewConnection(serverURL, webrtcToken string, opts ...ConnectionOption) *Connection {
	ctx, cancel := context.WithCancel(context.Background())
	c := &Connection{
		serverURL:   serverURL,
		webrtcToken: webrtcToken,
		ctx:         ctx,
		cancel:      cancel,
		boundTracks: make(map[string]*BoundTrack),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Connect establishes the WebSocket signaling connection, creates a WebRTC
// peer connection, and performs the SDP offer/answer exchange.
func (c *Connection) Connect(ctx context.Context) error {
	// 1. Open WebSocket — use the connection's long-lived context (not the
	// caller's ctx) so the WS survives beyond the initial connect timeout.
	wsURL := c.serverURL + "/ws/signaling?token=" + c.webrtcToken
	wsURL = httpToWS(wsURL)

	slog.Info("connecting to signaling WebSocket", "url", wsURL)

	wsConn, resp, err := websocket.Dial(c.ctx, wsURL, nil)
	if err != nil {
		// Detect 401/403 as auth errors so the client can stop retrying.
		if resp != nil && (resp.StatusCode == 401 || resp.StatusCode == 403) {
			return &AuthError{
				StatusCode: resp.StatusCode,
				Message:    fmt.Sprintf("authentication failed (HTTP %d)", resp.StatusCode),
			}
		}
		return fmt.Errorf("opening signaling websocket: %w", err)
	}
	c.mu.Lock()
	c.wsConn = wsConn
	c.mu.Unlock()

	// 2. Create PeerConnection
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	}

	pc, err := webrtc.NewPeerConnection(config)
	if err != nil {
		wsConn.Close(websocket.StatusNormalClosure, "")
		return fmt.Errorf("creating peer connection: %w", err)
	}
	c.mu.Lock()
	c.peerConn = pc
	c.mu.Unlock()

	// Set ICE connection state handler
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		slog.Info("ICE connection state changed", "state", state.String())
		if c.onConnectionStateChange != nil {
			c.onConnectionStateChange(state)
		}
	})

	// Send ICE candidates to remote peer via WebSocket
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}
		candidateJSON := candidate.ToJSON()
		candidateBytes, err := json.Marshal(candidateJSON)
		if err != nil {
			slog.Error("failed to marshal ICE candidate", "error", err)
			return
		}
		msg := crosstalk.SignalingMessage{
			Type:      "ice",
			Candidate: json.RawMessage(candidateBytes),
		}
		if err := c.sendSignaling(ctx, msg); err != nil {
			slog.Error("failed to send ICE candidate", "error", err)
		}
	})

	// 3. Create control data channel
	dc, err := pc.CreateDataChannel("control", &webrtc.DataChannelInit{
		Ordered: boolPtr(true),
	})
	if err != nil {
		pc.Close()
		wsConn.Close(websocket.StatusNormalClosure, "")
		return fmt.Errorf("creating control data channel: %w", err)
	}
	c.mu.Lock()
	c.controlDC = dc
	c.mu.Unlock()

	dc.OnOpen(func() {
		slog.Info("control data channel opened")
		if c.onControlOpen != nil {
			c.onControlOpen()
		}
	})

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if c.onControlMessage != nil {
			c.onControlMessage(msg.Data)
		}
	})

	// 4. Create SDP offer
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		pc.Close()
		wsConn.Close(websocket.StatusNormalClosure, "")
		return fmt.Errorf("creating SDP offer: %w", err)
	}

	if err := pc.SetLocalDescription(offer); err != nil {
		pc.Close()
		wsConn.Close(websocket.StatusNormalClosure, "")
		return fmt.Errorf("setting local description: %w", err)
	}

	// 5. Send SDP offer over WebSocket
	offerMsg := crosstalk.SignalingMessage{
		Type: "offer",
		SDP:  offer.SDP,
	}
	if err := c.sendSignaling(ctx, offerMsg); err != nil {
		pc.Close()
		wsConn.Close(websocket.StatusNormalClosure, "")
		return fmt.Errorf("sending SDP offer: %w", err)
	}

	// 6. Read signaling messages (SDP answer + ICE candidates).
	// Use c.ctx (not the caller's ctx) so the read loop survives beyond the
	// initial connect timeout and stays alive for renegotiation.
	if err := c.readSignalingLoop(c.ctx); err != nil {
		return fmt.Errorf("signaling: %w", err)
	}

	return nil
}

// SendControl sends a message on the control data channel.
func (c *Connection) SendControl(data []byte) error {
	c.mu.Lock()
	dc := c.controlDC
	c.mu.Unlock()

	if dc == nil {
		return fmt.Errorf("control data channel not open")
	}
	return dc.Send(data)
}

// AddTrack creates an Opus audio track on the PeerConnection and returns
// a BoundTrack that can be used for writing RTP data. The caller is
// responsible for feeding audio data into the track.
func (c *Connection) AddTrack(channelID, trackID, localName string, dir Direction) (*BoundTrack, error) {
	c.mu.Lock()
	pc := c.peerConn
	c.mu.Unlock()

	if pc == nil {
		return nil, fmt.Errorf("peer connection not established")
	}

	track, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
		trackID,
		trackID,
	)
	if err != nil {
		return nil, fmt.Errorf("creating audio track: %w", err)
	}

	sender, err := pc.AddTrack(track)
	if err != nil {
		return nil, fmt.Errorf("adding track to peer connection: %w", err)
	}

	done := make(chan struct{})
	go func() {
		buf := make([]byte, 1500)
		for {
			select {
			case <-done:
				return
			default:
			}
			if _, _, err := sender.Read(buf); err != nil {
				return
			}
		}
	}()

	bt := &BoundTrack{
		ChannelID: channelID,
		TrackID:   trackID,
		LocalName: localName,
		Direction: dir,
		Track:     track,
		Sender:    sender,
		stopFn: func() {
			close(done)
			pc.RemoveTrack(sender)
		},
	}

	c.mu.Lock()
	c.boundTracks[channelID] = bt
	c.mu.Unlock()

	slog.Info("audio track added",
		"channel_id", channelID,
		"track_id", trackID,
		"local_name", localName,
		"direction", dir,
	)

	return bt, nil
}

// RemoveTrack removes a bound track by channel ID.
func (c *Connection) RemoveTrack(channelID string) error {
	c.mu.Lock()
	bt, ok := c.boundTracks[channelID]
	if ok {
		delete(c.boundTracks, channelID)
	}
	c.mu.Unlock()

	if !ok {
		return fmt.Errorf("no bound track for channel %s", channelID)
	}

	if bt.stopFn != nil {
		bt.stopFn()
	}

	slog.Info("audio track removed", "channel_id", channelID)
	return nil
}

// BoundTracks returns a snapshot of currently bound tracks.
func (c *Connection) BoundTracks() map[string]*BoundTrack {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make(map[string]*BoundTrack, len(c.boundTracks))
	for k, v := range c.boundTracks {
		result[k] = v
	}
	return result
}

// Close cleanly shuts down the connection.
func (c *Connection) Close() error {
	c.cancel()

	c.mu.Lock()
	defer c.mu.Unlock()

	for id, bt := range c.boundTracks {
		if bt.stopFn != nil {
			bt.stopFn()
		}
		delete(c.boundTracks, id)
	}
	if c.controlDC != nil {
		c.controlDC.Close()
	}
	if c.peerConn != nil {
		c.peerConn.Close()
	}
	if c.wsConn != nil {
		c.wsConn.Close(websocket.StatusNormalClosure, "closing")
	}
	return nil
}

// ConnectionState returns the current ICE connection state.
func (c *Connection) ConnectionState() webrtc.ICEConnectionState {
	c.mu.Lock()
	pc := c.peerConn
	c.mu.Unlock()

	if pc == nil {
		return webrtc.ICEConnectionStateNew
	}
	return pc.ICEConnectionState()
}

func (c *Connection) sendSignaling(ctx context.Context, msg crosstalk.SignalingMessage) error {
	c.mu.Lock()
	ws := c.wsConn
	c.mu.Unlock()

	if ws == nil {
		return fmt.Errorf("websocket not connected")
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshaling signaling message: %w", err)
	}

	return ws.Write(ctx, websocket.MessageText, data)
}

func (c *Connection) readSignalingLoop(ctx context.Context) error {
	c.mu.Lock()
	ws := c.wsConn
	pc := c.peerConn
	c.mu.Unlock()

	answerReceived := false

	for {
		_, data, err := ws.Read(ctx)
		if err != nil {
			if answerReceived {
				// After getting the answer, WS read errors are expected
				// when ICE completes and the server closes the WS.
				slog.Debug("signaling websocket read ended", "error", err)
				return nil
			}
			return fmt.Errorf("reading signaling message: %w", err)
		}

		var msg crosstalk.SignalingMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			slog.Warn("ignoring unparseable signaling message", "error", err)
			continue
		}

		switch msg.Type {
		case "answer":
			slog.Info("received SDP answer")
			answer := webrtc.SessionDescription{
				Type: webrtc.SDPTypeAnswer,
				SDP:  msg.SDP,
			}
			if err := pc.SetRemoteDescription(answer); err != nil {
				return fmt.Errorf("setting remote description: %w", err)
			}
			answerReceived = true

		case "offer":
			slog.Info("received renegotiation offer from server")
			offer := webrtc.SessionDescription{
				Type: webrtc.SDPTypeOffer,
				SDP:  msg.SDP,
			}
			if err := pc.SetRemoteDescription(offer); err != nil {
				slog.Error("failed to set renegotiation offer", "error", err)
				continue
			}
			answer, err := pc.CreateAnswer(nil)
			if err != nil {
				slog.Error("failed to create renegotiation answer", "error", err)
				continue
			}
			if err := pc.SetLocalDescription(answer); err != nil {
				slog.Error("failed to set local description for renegotiation", "error", err)
				continue
			}
			answerMsg := crosstalk.SignalingMessage{
				Type: "answer",
				SDP:  answer.SDP,
			}
			if err := c.sendSignaling(ctx, answerMsg); err != nil {
				slog.Error("failed to send renegotiation answer", "error", err)
				continue
			}
			slog.Info("sent renegotiation answer")

		case "ice":
			if len(msg.Candidate) == 0 || string(msg.Candidate) == "null" {
				continue
			}
			slog.Debug("received ICE candidate")
			// Parse candidate: server sends ICECandidateInit object
			var candidateInit webrtc.ICECandidateInit
			if err := json.Unmarshal(msg.Candidate, &candidateInit); err != nil {
				// Fallback: try as plain string
				var candidateStr string
				if err2 := json.Unmarshal(msg.Candidate, &candidateStr); err2 != nil {
					slog.Warn("failed to parse ICE candidate", "error", err)
					continue
				}
				candidateInit = webrtc.ICECandidateInit{Candidate: candidateStr}
			}
			if err := pc.AddICECandidate(candidateInit); err != nil {
				slog.Warn("failed to add ICE candidate", "error", err)
			}

		default:
			slog.Warn("unknown signaling message type", "type", msg.Type)
		}
	}
}

func httpToWS(url string) string {
	if len(url) >= 8 && url[:8] == "https://" {
		return "wss://" + url[8:]
	}
	if len(url) >= 7 && url[:7] == "http://" {
		return "ws://" + url[7:]
	}
	return url
}

func boolPtr(b bool) *bool {
	return &b
}
