package webrtc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
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

// signalOut serializes outbound signaling frames on one WebSocket.
//
// Pion may emit OnICECandidate from another goroutine as soon as
// SetLocalDescription runs inside HandleOffer/Negotiate. Without coordination
// those trickle candidates can be written before the matching SDP answer/offer
// (and concurrent conn.Write is unsafe). Hold ICE until the local SDP is on
// the wire, then flush in order under a single mutex.
type signalOut struct {
	mu         sync.Mutex
	conn       *websocket.Conn
	ctx        context.Context
	peer       *PeerConn
	hold       int // >0 while a local SDP is being prepared/written
	pendingICE [][]byte
}

func newSignalOut(ctx context.Context, conn *websocket.Conn, peer *PeerConn) *signalOut {
	return &signalOut{conn: conn, ctx: ctx, peer: peer}
}

// HoldICE starts buffering local ICE until the next successful WriteSDP.
func (o *signalOut) HoldICE() {
	o.mu.Lock()
	o.hold++
	o.mu.Unlock()
}

// AbortHold drops one hold level without writing SDP (e.g. HandleOffer failed).
// Pending ICE is discarded when no holds remain — there is no local description
// for clients to pair them with.
func (o *signalOut) AbortHold() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.hold > 0 {
		o.hold--
	}
	if o.hold == 0 {
		o.pendingICE = nil
	}
}

// ReleaseLeftoverHold aborts a hold that was not consumed by WriteSDP.
// Used after Negotiate when early-return paths skip OnNegotiationNeeded.
func (o *signalOut) ReleaseLeftoverHold() {
	o.mu.Lock()
	leftover := o.hold > 0
	o.mu.Unlock()
	if leftover {
		o.AbortHold()
	}
}

// WriteSDP writes an offer/answer, ends one hold, and flushes buffered ICE.
func (o *signalOut) WriteSDP(msgType, sdp string) error {
	seq := msgSeq.Add(1)
	msg := SignalMessage{Type: msgType, SDP: sdp, Seq: seq}
	data, err := json.Marshal(msg)
	if err != nil {
		o.AbortHold()
		return err
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	if err := o.conn.Write(o.ctx, websocket.MessageText, data); err != nil {
		if o.hold > 0 {
			o.hold--
		}
		if o.hold == 0 {
			o.pendingICE = nil
		}
		return err
	}

	o.peer.events.Push(MakeEvent(o.peer.ID, EventSignalingMessage, map[string]any{
		"direction": "outbound",
		"type":      msgType,
		"seq":       seq,
	}))

	if o.hold > 0 {
		o.hold--
	}
	if o.hold == 0 && len(o.pendingICE) > 0 {
		pending := o.pendingICE
		o.pendingICE = nil
		for _, frame := range pending {
			if err := o.conn.Write(o.ctx, websocket.MessageText, frame); err != nil {
				return err
			}
		}
	}
	return nil
}

// WriteICE writes a trickle candidate, or buffers it while a local SDP hold is active.
func (o *signalOut) WriteICE(candidate webrtc.ICECandidateInit) {
	seq := msgSeq.Add(1)
	msg := SignalMessage{
		Type:      "candidate",
		Candidate: &candidate,
		Seq:       seq,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		slog.Error("webrtc: marshal ICE candidate", "peer", o.peer.ID, "err", err)
		return
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	// Record the outbound signaling event when the frame is accepted for send
	// (including while buffered) so seq/type remain observable to debug APIs.
	o.peer.events.Push(MakeEvent(o.peer.ID, EventSignalingMessage, map[string]any{
		"direction": "outbound",
		"type":      "candidate",
		"seq":       seq,
	}))

	if o.hold > 0 {
		o.pendingICE = append(o.pendingICE, data)
		return
	}
	if err := o.conn.Write(o.ctx, websocket.MessageText, data); err != nil {
		slog.Debug("webrtc: send ICE candidate failed", "peer", o.peer.ID, "err", err)
	}
}

// SignalingHandler upgrades HTTP to WebSocket and runs WebRTC signaling.
// It creates an instrumented PeerConnection and logs every message.
type SignalingHandler struct {
	PeerManager   *PeerManager
	ServerVersion string

	// AuthFunc is an optional authentication function. If set, it's called with
	// the HTTP request before upgrade. Return an error to reject.
	AuthFunc func(r *http.Request) error

	// OnPeer, if set, is called with each freshly created peer (and the
	// originating request) before the signaling loop begins. It is the hook
	// used to bridge a peer's media into a session mixer. It returns
	// initiateOffer=true to request a server-initiated SDP offer (for
	// receive-only peers that publish no track of their own). Returning an
	// error aborts the session.
	OnPeer func(peer *PeerConn, r *http.Request) (initiateOffer bool, err error)

	// Limits, when non-zero fields are set, override PeerManager admission
	// bounds for this handler's message checks. Peer count still uses the
	// manager. Zero fields fall back to the manager's limits / defaults.
	Limits AdmissionLimits
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

	// Reject before upgrade when the manager is at peer capacity or closed.
	if h.PeerManager != nil {
		if h.PeerManager.Closed() {
			http.Error(w, ErrPeerManagerClosed.Error(), http.StatusServiceUnavailable)
			return
		}
		limits := h.admissionLimits()
		if err := CheckPeerCount(h.PeerManager.Count(), limits.MaxPeers); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
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
		status := websocket.StatusInternalError
		if errors.Is(err, ErrTooManyPeers) || errors.Is(err, ErrPeerManagerClosed) {
			status = websocket.StatusTryAgainLater
		}
		conn.Close(status, "peer connection failed")
		return
	}
	defer h.PeerManager.RemovePeer(peer.ID)

	slog.Info("webrtc: signaling session started", "peer", peer.ID)

	// Optional per-peer setup (e.g. bridging media into a session). Runs before
	// the signaling loop so any server-added tracks are included in the answer.
	var initiateOffer bool
	if h.OnPeer != nil {
		var err error
		initiateOffer, err = h.OnPeer(peer, r)
		if err != nil {
			slog.Error("webrtc: OnPeer setup failed", "peer", peer.ID, "err", err)
			conn.Close(websocket.StatusInternalError, "peer setup failed")
			return
		}
	}

	// Outbound writer: serialize WS writes and keep ICE after local SDP.
	ctx := r.Context()
	out := newSignalOut(ctx, conn, peer)

	// Register ICE candidate trickle callback.
	peer.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		// Emit "candidate" — the type the browser SPAs (and Go test client)
		// expect for trickled ICE.
		out.WriteICE(c.ToJSON())
	})

	// Register renegotiation callback.
	peer.OnNegotiationNeeded(func(offer webrtc.SessionDescription) {
		if err := out.WriteSDP("offer", offer.SDP); err != nil {
			slog.Debug("webrtc: send renegotiation offer failed", "peer", peer.ID, "err", err)
		}
	})

	// For receive-only peers (e.g. broadcast listeners) the server drives
	// negotiation: creating the offer now includes the tracks added in OnPeer.
	// Hold ICE across SetLocalDescription inside Negotiate so candidates cannot
	// precede the server offer on the wire.
	if initiateOffer {
		out.HoldICE()
		peer.Negotiate()
		// Negotiate invokes OnNegotiationNeeded → WriteSDP on success. If it
		// returned without writing (not stable / create failed), drop the hold.
		out.ReleaseLeftoverHold()
	}

	// Read loop: process signaling messages.
	h.readLoop(ctx, out, peer)
}

// admissionLimits merges handler overrides with manager defaults.
func (h *SignalingHandler) admissionLimits() AdmissionLimits {
	var base AdmissionLimits
	if h.PeerManager != nil {
		base = h.PeerManager.AdmissionLimits()
	} else {
		base = AdmissionLimits{}.WithDefaults()
	}
	// Explicit handler fields override.
	if h.Limits.MaxPeers > 0 {
		base.MaxPeers = h.Limits.MaxPeers
	}
	if h.Limits.MaxSDPBytes > 0 {
		base.MaxSDPBytes = h.Limits.MaxSDPBytes
	}
	if h.Limits.MaxRemoteICECandidates > 0 {
		base.MaxRemoteICECandidates = h.Limits.MaxRemoteICECandidates
	}
	if h.Limits.MaxSignalingMessageBytes > 0 {
		base.MaxSignalingMessageBytes = h.Limits.MaxSignalingMessageBytes
	}
	return base.WithDefaults()
}

// readLoop reads signaling messages until the connection closes.
func (h *SignalingHandler) readLoop(ctx context.Context, out *signalOut, peer *PeerConn) {
	limits := h.admissionLimits()
	conn := out.conn

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

		// Reject oversized frames before JSON unmarshal / SDP work.
		if err := CheckSignalingMessageSize(len(data), limits.MaxSignalingMessageBytes); err != nil {
			slog.Warn("webrtc: signaling message rejected",
				"peer", peer.ID, "err", err, "bytes", len(data))
			continue
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
			if err := CheckSDPSize(msg.SDP, limits.MaxSDPBytes); err != nil {
				slog.Warn("webrtc: offer SDP rejected", "peer", peer.ID, "err", err)
				continue
			}
			h.handleOffer(out, peer, msg)
		case "answer":
			if err := CheckSDPSize(msg.SDP, limits.MaxSDPBytes); err != nil {
				slog.Warn("webrtc: answer SDP rejected", "peer", peer.ID, "err", err)
				continue
			}
			h.handleAnswer(peer, msg)
		case "ice", "candidate":
			h.handleICE(peer, msg)
		default:
			slog.Warn("webrtc: unknown signaling type", "peer", peer.ID, "type", msg.Type)
		}
	}
}

// handleOffer processes an SDP offer and sends back an answer.
// Local ICE is held until the answer is written so clients never observe a
// trickle candidate before the SDP answer (CI flake class on MessageSequencing).
func (h *SignalingHandler) handleOffer(out *signalOut, peer *PeerConn, msg SignalMessage) {
	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  msg.SDP,
	}

	out.HoldICE()
	answer, err := peer.HandleOffer(offer)
	if err != nil {
		out.AbortHold()
		slog.Error("webrtc: handle offer failed", "peer", peer.ID, "err", err)
		return
	}

	if err := out.WriteSDP("answer", answer.SDP); err != nil {
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
