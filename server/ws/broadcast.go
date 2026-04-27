package ws

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/pion/webrtc/v4"
	"nhooyr.io/websocket"

	crosstalk "github.com/aleksclark/crosstalk/server"
	ctpion "github.com/aleksclark/crosstalk/server/pion"
)

// BroadcastSignalingHandler is an HTTP handler that upgrades connections to
// WebSocket and implements WebRTC signaling for broadcast listeners.
// Authentication is via a broadcast token in the query string (not Bearer auth).
// The created PeerConnection is receive-only with no control data channel.
type BroadcastSignalingHandler struct {
	BroadcastTokenStore crosstalk.BroadcastTokenStore
	PeerManager         *ctpion.PeerManager
	Orchestrator        *ctpion.Orchestrator
}

// ServeHTTP validates the broadcast token, upgrades to WebSocket, creates a
// receive-only PeerConnection, registers as a listener, and runs the signaling
// read loop.
func (h *BroadcastSignalingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Extract and validate broadcast token from query parameter.
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "missing broadcast token", http.StatusUnauthorized)
		return
	}

	bt, err := h.BroadcastTokenStore.ValidateBroadcastToken(token)
	if err != nil {
		http.Error(w, "invalid or expired broadcast token", http.StatusUnauthorized)
		return
	}

	sessionID := bt.SessionID

	// 2. Upgrade to WebSocket.
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // CORS handled at a higher layer.
	})
	if err != nil {
		slog.Error("broadcast: websocket upgrade failed", "err", err)
		return
	}
	defer conn.Close(websocket.StatusInternalError, "unexpected close")

	// 3. Create a receive-only PeerConnection (no control data channel).
	peer, err := h.PeerManager.CreateListenerPeerConnection()
	if err != nil {
		slog.Error("broadcast: failed to create listener peer connection", "err", err)
		conn.Close(websocket.StatusInternalError, "peer connection failed")
		return
	}
	defer h.PeerManager.RemovePeer(peer.ID)

	slog.Info("broadcast: listener signaling started",
		"peer", peer.ID, "session", sessionID)

	// 4. Register the peer as a listener in the Orchestrator.
	if err := h.Orchestrator.AddListener(peer, sessionID); err != nil {
		slog.Error("broadcast: failed to add listener to session",
			"peer", peer.ID, "session", sessionID, "err", err)
		conn.Close(websocket.StatusInternalError, "failed to join session")
		return
	}
	defer h.Orchestrator.RemoveListener(peer.ID)

	// 5. Register ICE candidate callback → trickle candidates to client.
	ctx := r.Context()
	peer.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return // Gathering complete.
		}
		candidate := c.ToJSON()
		msg := SignalMessage{
			Type:      "ice",
			Candidate: &candidate,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			slog.Error("broadcast: failed to marshal ICE candidate",
				"peer", peer.ID, "err", err)
			return
		}
		if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
			slog.Debug("broadcast: failed to send ICE candidate",
				"peer", peer.ID, "err", err)
		}
	})

	// 6. Register renegotiation callback → forward server offers to client.
	peer.OnNegotiationNeeded(func(offer webrtc.SessionDescription) {
		msg := SignalMessage{
			Type: "offer",
			SDP:  offer.SDP,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			slog.Error("broadcast: failed to marshal renegotiation offer",
				"peer", peer.ID, "err", err)
			return
		}
		if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
			slog.Debug("broadcast: failed to send renegotiation offer",
				"peer", peer.ID, "err", err)
		}
	})

	// 7. Read loop: process signaling messages from the client.
	h.readLoop(ctx, conn, peer)
}

// readLoop reads and processes signaling messages until the WebSocket closes
// or the context is cancelled.
func (h *BroadcastSignalingHandler) readLoop(ctx context.Context, conn *websocket.Conn, peer *ctpion.PeerConn) {
	for {
		readCtx, cancel := context.WithTimeout(ctx, readTimeout)
		_, data, err := conn.Read(readCtx)
		cancel()
		if err != nil {
			if errors.Is(err, context.Canceled) ||
				websocket.CloseStatus(err) == websocket.StatusNormalClosure ||
				websocket.CloseStatus(err) == websocket.StatusGoingAway {
				slog.Info("broadcast: listener signaling ended", "peer", peer.ID)
			} else if errors.Is(err, context.DeadlineExceeded) {
				slog.Info("broadcast: listener signaling timed out", "peer", peer.ID)
			} else {
				slog.Error("broadcast: websocket read error", "peer", peer.ID, "err", err)
			}
			conn.Close(websocket.StatusNormalClosure, "")
			return
		}

		var msg SignalMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			slog.Error("broadcast: invalid signaling message", "peer", peer.ID, "err", err)
			continue
		}

		switch msg.Type {
		case "offer":
			h.handleOffer(ctx, conn, peer, msg)
		case "answer":
			h.handleAnswer(peer, msg)
		case "ice":
			h.handleICE(peer, msg)
		default:
			slog.Warn("broadcast: unknown signaling message type",
				"peer", peer.ID, "type", msg.Type)
		}
	}
}

// handleOffer processes an SDP offer, creates an answer, and sends it back.
func (h *BroadcastSignalingHandler) handleOffer(ctx context.Context, conn *websocket.Conn, peer *ctpion.PeerConn, msg SignalMessage) {
	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  msg.SDP,
	}

	answer, err := peer.HandleOffer(offer)
	if err != nil {
		slog.Error("broadcast: failed to handle offer", "peer", peer.ID, "err", err)
		return
	}

	resp := SignalMessage{
		Type: "answer",
		SDP:  answer.SDP,
	}
	data, err := json.Marshal(resp)
	if err != nil {
		slog.Error("broadcast: failed to marshal answer", "peer", peer.ID, "err", err)
		return
	}

	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		slog.Error("broadcast: failed to send answer", "peer", peer.ID, "err", err)
	}
}

// handleICE adds a remote ICE candidate to the peer connection.
func (h *BroadcastSignalingHandler) handleICE(peer *ctpion.PeerConn, msg SignalMessage) {
	if msg.Candidate == nil {
		return
	}
	if err := peer.AddICECandidate(*msg.Candidate); err != nil {
		slog.Error("broadcast: failed to add ICE candidate", "peer", peer.ID, "err", err)
	}
}

// handleAnswer processes an SDP answer from the client in response to a
// server-initiated renegotiation offer.
func (h *BroadcastSignalingHandler) handleAnswer(peer *ctpion.PeerConn, msg SignalMessage) {
	answer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  msg.SDP,
	}
	if err := peer.HandleAnswer(answer); err != nil {
		slog.Error("broadcast: failed to handle answer", "peer", peer.ID, "err", err)
	}
}
