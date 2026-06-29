package webrtc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/pion/webrtc/v4"
	"nhooyr.io/websocket"
)

// readTimeout is the max time to wait for a signaling message.
const readTimeout = 24 * time.Hour

// SignalMessage is the JSON envelope for WebSocket signaling messages.
type SignalMessage struct {
	Type      string                   `json:"type"`                // "offer", "answer", "ice"
	SDP       string                   `json:"sdp,omitempty"`       // SDP for offer/answer
	Candidate *webrtc.ICECandidateInit `json:"candidate,omitempty"` // ICE candidate
	Seq       int64                    `json:"seq,omitempty"`       // Message sequence number
}

// msgSeq is a global atomic sequence counter for correlation.
var msgSeq atomic.Int64

// SignalingHandler upgrades HTTP to WebSocket and runs WebRTC signaling.
// It creates an instrumented PeerConnection and logs every message.
type SignalingHandler struct {
	PeerManager   *PeerManager
	ServerVersion string

	// AuthFunc is an optional authentication function. If set, it's called with
	// the HTTP request before upgrade. Return an error to reject.
	AuthFunc func(r *http.Request) error
}

// ServeHTTP handles the WebSocket signaling upgrade and message loop.
func (h *SignalingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Optional auth check.
	if h.AuthFunc != nil {
		if err := h.AuthFunc(r); err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
	}

	// Upgrade to WebSocket.
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		slog.Error("webrtc: websocket upgrade failed", "err", err)
		return
	}
	defer conn.Close(websocket.StatusInternalError, "unexpected close")

	// Create instrumented PeerConnection.
	peer, err := h.PeerManager.CreatePeerConnection()
	if err != nil {
		slog.Error("webrtc: failed to create peer connection", "err", err)
		conn.Close(websocket.StatusInternalError, "peer connection failed")
		return
	}
	defer h.PeerManager.RemovePeer(peer.ID)

	slog.Info("webrtc: signaling session started", "peer", peer.ID)

	// Register ICE candidate trickle callback.
	ctx := r.Context()
	peer.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		candidate := c.ToJSON()
		seq := msgSeq.Add(1)
		msg := SignalMessage{
			Type:      "ice",
			Candidate: &candidate,
			Seq:       seq,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			slog.Error("webrtc: marshal ICE candidate", "peer", peer.ID, "err", err)
			return
		}
		peer.events.Push(MakeEvent(peer.ID, EventSignalingMessage, map[string]any{
			"direction": "outbound",
			"type":      "ice",
			"seq":       seq,
		}))
		if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
			slog.Debug("webrtc: send ICE candidate failed", "peer", peer.ID, "err", err)
		}
	})

	// Register renegotiation callback.
	peer.OnNegotiationNeeded(func(offer webrtc.SessionDescription) {
		seq := msgSeq.Add(1)
		msg := SignalMessage{
			Type: "offer",
			SDP:  offer.SDP,
			Seq:  seq,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			slog.Error("webrtc: marshal renegotiation offer", "peer", peer.ID, "err", err)
			return
		}
		peer.events.Push(MakeEvent(peer.ID, EventSignalingMessage, map[string]any{
			"direction": "outbound",
			"type":      "offer",
			"seq":       seq,
		}))
		if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
			slog.Debug("webrtc: send renegotiation offer failed", "peer", peer.ID, "err", err)
		}
	})

	// Read loop: process signaling messages.
	h.readLoop(ctx, conn, peer)
}

// readLoop reads signaling messages until the connection closes.
func (h *SignalingHandler) readLoop(ctx context.Context, conn *websocket.Conn, peer *PeerConn) {
	for {
		readCtx, cancel := context.WithTimeout(ctx, readTimeout)
		_, data, err := conn.Read(readCtx)
		cancel()
		if err != nil {
			if errors.Is(err, context.Canceled) ||
				websocket.CloseStatus(err) == websocket.StatusNormalClosure ||
				websocket.CloseStatus(err) == websocket.StatusGoingAway {
				slog.Info("webrtc: signaling session ended", "peer", peer.ID)
			} else if errors.Is(err, context.DeadlineExceeded) {
				slog.Info("webrtc: signaling session timed out", "peer", peer.ID)
			} else {
				slog.Error("webrtc: websocket read error", "peer", peer.ID, "err", err)
			}
			conn.Close(websocket.StatusNormalClosure, "")
			return
		}

		var msg SignalMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			slog.Error("webrtc: invalid signaling message", "peer", peer.ID, "err", err)
			continue
		}

		// Assign sequence number if not provided.
		if msg.Seq == 0 {
			msg.Seq = msgSeq.Add(1)
		}

		// Log every inbound message.
		peer.events.Push(MakeEvent(peer.ID, EventSignalingMessage, map[string]any{
			"direction": "inbound",
			"type":      msg.Type,
			"seq":       msg.Seq,
		}))
		slog.Debug("webrtc: signaling message received",
			"peer", peer.ID, "type", msg.Type, "seq", msg.Seq)

		switch msg.Type {
		case "offer":
			h.handleOffer(ctx, conn, peer, msg)
		case "answer":
			h.handleAnswer(peer, msg)
		case "ice":
			h.handleICE(peer, msg)
		default:
			slog.Warn("webrtc: unknown signaling type", "peer", peer.ID, "type", msg.Type)
		}
	}
}

// handleOffer processes an SDP offer and sends back an answer.
func (h *SignalingHandler) handleOffer(ctx context.Context, conn *websocket.Conn, peer *PeerConn, msg SignalMessage) {
	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  msg.SDP,
	}

	answer, err := peer.HandleOffer(offer)
	if err != nil {
		slog.Error("webrtc: handle offer failed", "peer", peer.ID, "err", err)
		return
	}

	seq := msgSeq.Add(1)
	resp := SignalMessage{
		Type: "answer",
		SDP:  answer.SDP,
		Seq:  seq,
	}
	data, err := json.Marshal(resp)
	if err != nil {
		slog.Error("webrtc: marshal answer", "peer", peer.ID, "err", err)
		return
	}

	peer.events.Push(MakeEvent(peer.ID, EventSignalingMessage, map[string]any{
		"direction": "outbound",
		"type":      "answer",
		"seq":       seq,
	}))

	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		slog.Error("webrtc: send answer failed", "peer", peer.ID, "err", err)
	}
}

// handleAnswer handles an SDP answer from the client.
func (h *SignalingHandler) handleAnswer(peer *PeerConn, msg SignalMessage) {
	answer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  msg.SDP,
	}
	if err := peer.HandleAnswer(answer); err != nil {
		slog.Error("webrtc: handle answer failed", "peer", peer.ID, "err", err)
	}
}

// handleICE adds a remote ICE candidate.
func (h *SignalingHandler) handleICE(peer *PeerConn, msg SignalMessage) {
	if msg.Candidate == nil {
		return
	}
	if err := peer.AddICECandidate(*msg.Candidate); err != nil {
		slog.Error("webrtc: add ICE candidate failed", "peer", peer.ID, "err", err,
			"candidate", fmt.Sprintf("%.60s", msg.Candidate.Candidate))
	}
}
