package pion

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	crosstalk "github.com/anthropics/crosstalk/cli"
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

	// onControlOpen is called when the control data channel opens.
	onControlOpen func()
	// onControlMessage is called when a message is received on the control channel.
	onControlMessage func([]byte)
	// onConnectionStateChange is called on ICE connection state changes.
	onConnectionStateChange func(webrtc.ICEConnectionState)

	ctx    context.Context
	cancel context.CancelFunc
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

// NewConnection creates a new Connection.
func NewConnection(serverURL, webrtcToken string, opts ...ConnectionOption) *Connection {
	ctx, cancel := context.WithCancel(context.Background())
	c := &Connection{
		serverURL:   serverURL,
		webrtcToken: webrtcToken,
		ctx:         ctx,
		cancel:      cancel,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Connect establishes the WebSocket signaling connection, creates a WebRTC
// peer connection, and performs the SDP offer/answer exchange.
func (c *Connection) Connect(ctx context.Context) error {
	// 1. Open WebSocket
	wsURL := c.serverURL + "/ws/signaling?token=" + c.webrtcToken
	// Convert http(s) to ws(s) scheme
	wsURL = httpToWS(wsURL)

	slog.Info("connecting to signaling WebSocket", "url", wsURL)

	wsConn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
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
		msg := crosstalk.SignalingMessage{
			Type:      "ice",
			Candidate: candidate.ToJSON().Candidate,
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

	// 6. Read signaling messages (SDP answer + ICE candidates)
	if err := c.readSignalingLoop(ctx); err != nil {
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

// Close cleanly shuts down the connection.
func (c *Connection) Close() error {
	c.cancel()

	c.mu.Lock()
	defer c.mu.Unlock()

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

		case "ice":
			if msg.Candidate == "" {
				continue
			}
			slog.Debug("received ICE candidate")
			if err := pc.AddICECandidate(webrtc.ICECandidateInit{
				Candidate: msg.Candidate,
			}); err != nil {
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
