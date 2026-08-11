// Package sessionmedia provides a finite WebRTC publisher for session media.
// It dials /api/sessions/{id}/ws with a one-time media ticket.
package sessionmedia

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"nhooyr.io/websocket"
)

// Connection is a one-shot session media publisher.
type Connection struct {
	mu sync.Mutex

	ws   *websocket.Conn
	pc   *webrtc.PeerConnection
	track *webrtc.TrackLocalStaticRTP

	pendingLocal  []webrtc.ICECandidateInit
	pendingRemote []webrtc.ICECandidateInit
	wsOpen        bool
	remoteSet     bool

	connected chan struct{}
	connOnce  sync.Once
	closed    bool

	writeMu sync.Mutex
	logger  *slog.Logger
}

// New returns a closed Connection ready for Connect.
func New() *Connection {
	return &Connection{
		connected: make(chan struct{}),
		logger:    slog.Default(),
	}
}

// Connect dials the session WebSocket, negotiates WebRTC, and waits until connected.
func (c *Connection) Connect(ctx context.Context, host, sessionID, ticket string) error {
	if strings.TrimSpace(host) == "" || strings.TrimSpace(sessionID) == "" || strings.TrimSpace(ticket) == "" {
		return fmt.Errorf("host, session id, and ticket are required")
	}
	base := strings.TrimRight(strings.TrimSpace(host), "/")
	wsURL := httpToWS(base) + "/api/sessions/" + url.PathEscape(sessionID) + "/ws?token=" + url.QueryEscape(ticket)

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	})
	if err != nil {
		return fmt.Errorf("create peer connection: %w", err)
	}
	c.mu.Lock()
	c.pc = pc
	c.mu.Unlock()

	track, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
		"ct-play", "ct-play",
	)
	if err != nil {
		_ = pc.Close()
		return fmt.Errorf("create track: %w", err)
	}
	sender, err := pc.AddTrack(track)
	if err != nil {
		_ = pc.Close()
		return fmt.Errorf("add track: %w", err)
	}
	go func() {
		buf := make([]byte, 1500)
		for {
			if _, _, rerr := sender.Read(buf); rerr != nil {
				return
			}
		}
	}()
	c.mu.Lock()
	c.track = track
	c.mu.Unlock()

	// Optional control channel for negotiation parity with other clients.
	if _, err := pc.CreateDataChannel("control", &webrtc.DataChannelInit{Ordered: boolPtr(true)}); err != nil {
		_ = pc.Close()
		return fmt.Errorf("create control channel: %w", err)
	}

	markConnected := func() {
		c.connOnce.Do(func() { close(c.connected) })
	}
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		c.logger.Info("sessionmedia ICE state", "state", state.String())
		if state == webrtc.ICEConnectionStateConnected || state == webrtc.ICEConnectionStateCompleted {
			markConnected()
		}
	})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateConnected {
			markConnected()
		}
	})

	ws, resp, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		_ = pc.Close()
		if resp != nil && (resp.StatusCode == 401 || resp.StatusCode == 403) {
			return fmt.Errorf("media admission rejected (HTTP %d)", resp.StatusCode)
		}
		return fmt.Errorf("dial session websocket: %w", err)
	}
	c.mu.Lock()
	c.ws = ws
	c.wsOpen = true
	// Flush any local candidates generated before dial completed.
	local := append([]webrtc.ICECandidateInit(nil), c.pendingLocal...)
	c.pendingLocal = nil
	c.mu.Unlock()
	for _, cand := range local {
		if err := c.sendICE(ctx, cand); err != nil {
			c.logger.Debug("failed to flush local ICE", "error", err.Error())
		}
	}

	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}
		init := candidate.ToJSON()
		c.mu.Lock()
		open := c.wsOpen
		if !open {
			c.pendingLocal = append(c.pendingLocal, init)
			c.mu.Unlock()
			return
		}
		c.mu.Unlock()
		if err := c.sendICE(ctx, init); err != nil {
			c.logger.Debug("send ICE failed", "error", err.Error())
		}
	})

	// Start read loop before offer so answer/candidates are not missed.
	readErr := make(chan error, 1)
	go func() { readErr <- c.readLoop(ctx) }()

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		_ = c.Close()
		return fmt.Errorf("create offer: %w", err)
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		_ = c.Close()
		return fmt.Errorf("set local description: %w", err)
	}
	if err := c.sendJSON(ctx, map[string]string{"type": "offer", "sdp": offer.SDP}); err != nil {
		_ = c.Close()
		return fmt.Errorf("send offer: %w", err)
	}

	// Wait for ICE connected or failure/timeout.
	select {
	case <-c.connected:
		return nil
	case err := <-readErr:
		_ = c.Close()
		if err != nil {
			return fmt.Errorf("signaling: %w", err)
		}
		return fmt.Errorf("signaling closed before connected")
	case <-ctx.Done():
		_ = c.Close()
		return ctx.Err()
	case <-time.After(45 * time.Second):
		_ = c.Close()
		return fmt.Errorf("timed out waiting for media connection")
	}
}

// WriteRTP writes a complete marshaled RTP packet to the local track.
func (c *Connection) WriteRTP(packet []byte) error {
	c.mu.Lock()
	track := c.track
	closed := c.closed
	c.mu.Unlock()
	if closed || track == nil {
		return fmt.Errorf("media connection is not open")
	}
	var pkt rtp.Packet
	if err := pkt.Unmarshal(packet); err != nil {
		return fmt.Errorf("invalid RTP packet: %w", err)
	}
	return track.WriteRTP(&pkt)
}

// Close releases WebSocket and peer connection resources.
func (c *Connection) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	ws := c.ws
	pc := c.pc
	c.ws = nil
	c.pc = nil
	c.track = nil
	c.wsOpen = false
	c.mu.Unlock()

	if pc != nil {
		_ = pc.Close()
	}
	if ws != nil {
		_ = ws.Close(websocket.StatusNormalClosure, "closing")
	}
	return nil
}

func (c *Connection) readLoop(ctx context.Context) error {
	for {
		_, data, err := c.ws.Read(ctx)
		if err != nil {
			return err
		}
		var msg struct {
			Type      string                   `json:"type"`
			SDP       string                   `json:"sdp"`
			Candidate *webrtc.ICECandidateInit `json:"candidate"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		switch msg.Type {
		case "answer":
			c.mu.Lock()
			pc := c.pc
			c.mu.Unlock()
			if pc == nil {
				continue
			}
			if err := pc.SetRemoteDescription(webrtc.SessionDescription{
				Type: webrtc.SDPTypeAnswer,
				SDP:  msg.SDP,
			}); err != nil {
				c.logger.Debug("set remote answer failed", "error", err.Error())
				continue
			}
			c.mu.Lock()
			c.remoteSet = true
			pending := append([]webrtc.ICECandidateInit(nil), c.pendingRemote...)
			c.pendingRemote = nil
			c.mu.Unlock()
			for _, cand := range pending {
				_ = pc.AddICECandidate(cand)
			}
		case "offer":
			// Server renegotiation: answer it.
			c.mu.Lock()
			pc := c.pc
			c.mu.Unlock()
			if pc == nil {
				continue
			}
			if err := pc.SetRemoteDescription(webrtc.SessionDescription{
				Type: webrtc.SDPTypeOffer,
				SDP:  msg.SDP,
			}); err != nil {
				continue
			}
			ans, err := pc.CreateAnswer(nil)
			if err != nil {
				continue
			}
			if err := pc.SetLocalDescription(ans); err != nil {
				continue
			}
			_ = c.sendJSON(ctx, map[string]string{"type": "answer", "sdp": ans.SDP})
		case "ice", "candidate":
			if msg.Candidate == nil {
				continue
			}
			c.mu.Lock()
			pc := c.pc
			remoteSet := c.remoteSet
			if !remoteSet {
				c.pendingRemote = append(c.pendingRemote, *msg.Candidate)
				c.mu.Unlock()
				continue
			}
			c.mu.Unlock()
			if pc != nil {
				_ = pc.AddICECandidate(*msg.Candidate)
			}
		}
	}
}

func (c *Connection) sendICE(ctx context.Context, cand webrtc.ICECandidateInit) error {
	return c.sendJSON(ctx, map[string]any{
		"type":      "candidate",
		"candidate": cand,
	})
}

func (c *Connection) sendJSON(ctx context.Context, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.mu.Lock()
	ws := c.ws
	c.mu.Unlock()
	if ws == nil {
		return fmt.Errorf("websocket closed")
	}
	return ws.Write(ctx, websocket.MessageText, raw)
}

func httpToWS(u string) string {
	if strings.HasPrefix(u, "https://") {
		return "wss://" + strings.TrimPrefix(u, "https://")
	}
	if strings.HasPrefix(u, "http://") {
		return "ws://" + strings.TrimPrefix(u, "http://")
	}
	return u
}

func boolPtr(b bool) *bool { return &b }
