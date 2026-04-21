package ws

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/pion/webrtc/v4"
	"nhooyr.io/websocket"

	crosstalk "github.com/anthropics/crosstalk/server"
	ctpion "github.com/anthropics/crosstalk/server/pion"
)

// readTimeout is the maximum duration to wait for a single WebSocket message.
const readTimeout = 60 * time.Second

// SignalMessage is the JSON envelope for all WebSocket signaling messages.
type SignalMessage struct {
	Type      string                   `json:"type"`                // "offer", "answer", "ice"
	SDP       string                   `json:"sdp,omitempty"`       // SDP for offer/answer
	Candidate *webrtc.ICECandidateInit `json:"candidate,omitempty"` // ICE candidate for trickle
}

// SignalingHandler is an HTTP handler that upgrades connections to WebSocket
// and implements the WebRTC signaling protocol. Clients authenticate via a
// token query parameter, then exchange SDP offers/answers and ICE candidates.
type SignalingHandler struct {
	TokenService crosstalk.TokenService
	PeerManager  *ctpion.PeerManager
}

// hashToken returns the hex-encoded SHA-256 hash of a plaintext token.
// This duplicates http.HashToken to avoid a dependency on the http package.
func hashToken(plaintext string) string {
	h := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(h[:])
}

// ServeHTTP authenticates the request, upgrades to WebSocket, creates a
// PeerConnection, and runs the signaling read loop.
func (h *SignalingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Extract and validate token from query parameter.
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}

	hash := hashToken(token)
	apiToken, err := h.TokenService.FindTokenByHash(hash)
	if err != nil || apiToken == nil {
		http.Error(w, "invalid or expired token", http.StatusUnauthorized)
		return
	}

	// 2. Upgrade to WebSocket.
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // CORS handled at a higher layer.
	})
	if err != nil {
		slog.Error("websocket upgrade failed", "err", err)
		return
	}
	defer conn.Close(websocket.StatusInternalError, "unexpected close")

	// 3. Create a server-side PeerConnection.
	peer, err := h.PeerManager.CreatePeerConnection()
	if err != nil {
		slog.Error("failed to create peer connection", "err", err)
		conn.Close(websocket.StatusInternalError, "peer connection failed")
		return
	}
	defer h.PeerManager.RemovePeer(peer.ID)

	slog.Info("signaling session started", "peer", peer.ID, "token", apiToken.Name)

	// 4. Register ICE candidate callback → trickle candidates to client.
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
			slog.Error("failed to marshal ICE candidate", "peer", peer.ID, "err", err)
			return
		}
		if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
			slog.Debug("failed to send ICE candidate", "peer", peer.ID, "err", err)
		}
	})

	// 5. Read loop: process signaling messages from the client.
	h.readLoop(ctx, conn, peer)
}

// readLoop reads and processes signaling messages until the WebSocket closes
// or the context is cancelled.
func (h *SignalingHandler) readLoop(ctx context.Context, conn *websocket.Conn, peer *ctpion.PeerConn) {
	for {
		readCtx, cancel := context.WithTimeout(ctx, readTimeout)
		_, data, err := conn.Read(readCtx)
		cancel()
		if err != nil {
			if errors.Is(err, context.Canceled) ||
				websocket.CloseStatus(err) == websocket.StatusNormalClosure ||
				websocket.CloseStatus(err) == websocket.StatusGoingAway {
				slog.Info("signaling session ended", "peer", peer.ID)
			} else if errors.Is(err, context.DeadlineExceeded) {
				slog.Info("signaling session timed out", "peer", peer.ID)
			} else {
				slog.Error("websocket read error", "peer", peer.ID, "err", err)
			}
			conn.Close(websocket.StatusNormalClosure, "")
			return
		}

		var msg SignalMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			slog.Error("invalid signaling message", "peer", peer.ID, "err", err)
			continue
		}

		switch msg.Type {
		case "offer":
			h.handleOffer(ctx, conn, peer, msg)
		case "ice":
			h.handleICE(peer, msg)
		default:
			slog.Warn("unknown signaling message type", "peer", peer.ID, "type", msg.Type)
		}
	}
}

// handleOffer processes an SDP offer, creates an answer, and sends it back.
func (h *SignalingHandler) handleOffer(ctx context.Context, conn *websocket.Conn, peer *ctpion.PeerConn, msg SignalMessage) {
	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  msg.SDP,
	}

	answer, err := peer.HandleOffer(offer)
	if err != nil {
		slog.Error("failed to handle offer", "peer", peer.ID, "err", err)
		return
	}

	resp := SignalMessage{
		Type: "answer",
		SDP:  answer.SDP,
	}
	data, err := json.Marshal(resp)
	if err != nil {
		slog.Error("failed to marshal answer", "peer", peer.ID, "err", err)
		return
	}

	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		slog.Error("failed to send answer", "peer", peer.ID, "err", err)
	}
}

// handleICE adds a remote ICE candidate to the peer connection.
func (h *SignalingHandler) handleICE(peer *ctpion.PeerConn, msg SignalMessage) {
	if msg.Candidate == nil {
		return
	}
	if err := peer.AddICECandidate(*msg.Candidate); err != nil {
		slog.Error("failed to add ICE candidate", "peer", peer.ID, "err", err)
	}
}
