package pion

import (
	"fmt"
	"log/slog"

	"github.com/pion/webrtc/v4"
	"google.golang.org/protobuf/proto"

	crosstalk "github.com/anthropics/crosstalk/server"
	crosstalkv1 "github.com/anthropics/crosstalk/proto/gen/go/crosstalk/v1"
)

// ControlHandler processes protobuf messages on the control data channel.
type ControlHandler struct {
	Peer           *PeerConn
	Orchestrator   *Orchestrator
	SessionService crosstalk.SessionService
	ServerVersion  string

	// Callbacks
	OnHello       func(peer *PeerConn, hello *crosstalkv1.Hello)
	OnJoinSession func(peer *PeerConn, join *crosstalkv1.JoinSession)
	OnLogEntry    func(peer *PeerConn, entry *crosstalkv1.LogEntry)
}

// Install replaces the PeerConn's data channel OnMessage handler with the
// protobuf control-message dispatcher.
func (h *ControlHandler) Install() {
	h.Peer.control.OnMessage(func(msg webrtc.DataChannelMessage) {
		h.dispatch(msg.Data)
	})
}

// dispatch unmarshals the raw bytes into a ControlMessage and routes to the
// appropriate handler based on the payload oneof.
func (h *ControlHandler) dispatch(data []byte) {
	var cm crosstalkv1.ControlMessage
	if err := proto.Unmarshal(data, &cm); err != nil {
		slog.Error("control: unmarshal failed", "peer", h.Peer.ID, "err", err)
		return
	}

	switch payload := cm.GetPayload().(type) {
	case *crosstalkv1.ControlMessage_Hello:
		h.handleHello(payload.Hello)
	case *crosstalkv1.ControlMessage_ClientStatus:
		h.handleClientStatus(payload.ClientStatus)
	case *crosstalkv1.ControlMessage_ChannelStatus:
		h.handleChannelStatus(payload.ChannelStatus)
	case *crosstalkv1.ControlMessage_LogEntry:
		h.handleLogEntry(payload.LogEntry)
	case *crosstalkv1.ControlMessage_JoinSession:
		h.handleJoinSession(payload.JoinSession)
	default:
		slog.Warn("control: unhandled message type", "peer", h.Peer.ID, "payload", fmt.Sprintf("%T", cm.GetPayload()))
	}
}

// handleHello stores client capabilities on the PeerConn and responds with a
// Welcome message containing the assigned client_id and server version.
func (h *ControlHandler) handleHello(hello *crosstalkv1.Hello) {
	slog.Debug("control: received Hello", "peer", h.Peer.ID,
		"sources", len(hello.GetSources()),
		"sinks", len(hello.GetSinks()),
		"codecs", len(hello.GetCodecs()))

	h.Peer.mu.Lock()
	h.Peer.Sources = hello.GetSources()
	h.Peer.Sinks = hello.GetSinks()
	h.Peer.Codecs = hello.GetCodecs()
	h.Peer.mu.Unlock()

	if h.OnHello != nil {
		h.OnHello(h.Peer, hello)
	}

	welcome := &crosstalkv1.ControlMessage{
		Payload: &crosstalkv1.ControlMessage_Welcome{
			Welcome: &crosstalkv1.Welcome{
				ClientId:      h.Peer.ID,
				ServerVersion: h.ServerVersion,
			},
		},
	}
	if err := h.Peer.SendControlMessage(welcome); err != nil {
		slog.Error("control: send Welcome failed", "peer", h.Peer.ID, "err", err)
	}
}

// handleClientStatus logs the status update and stores it for future use.
func (h *ControlHandler) handleClientStatus(status *crosstalkv1.ClientStatus) {
	slog.Debug("control: received ClientStatus", "peer", h.Peer.ID, "state", status.GetState())
}

// handleChannelStatus logs the channel status report.
func (h *ControlHandler) handleChannelStatus(status *crosstalkv1.ChannelStatus) {
	slog.Debug("control: received ChannelStatus", "peer", h.Peer.ID,
		"channel", status.GetChannelId(), "state", status.GetState())
}

// handleLogEntry forwards to the OnLogEntry callback or logs it directly.
func (h *ControlHandler) handleLogEntry(entry *crosstalkv1.LogEntry) {
	if h.OnLogEntry != nil {
		h.OnLogEntry(h.Peer, entry)
		return
	}
	slog.Info("control: client log", "peer", h.Peer.ID,
		"severity", entry.GetSeverity(),
		"source", entry.GetSource(),
		"message", entry.GetMessage())
}

// handleJoinSession delegates to the Orchestrator if available, otherwise
// falls back to the basic session lookup and role storage.
func (h *ControlHandler) handleJoinSession(join *crosstalkv1.JoinSession) {
	slog.Debug("control: received JoinSession", "peer", h.Peer.ID,
		"session_id", join.GetSessionId(), "role", join.GetRole())

	if h.OnJoinSession != nil {
		h.OnJoinSession(h.Peer, join)
	}

	// Delegate to orchestrator if available — it handles validation,
	// cardinality, binding evaluation, and BindChannel commands.
	if h.Orchestrator != nil {
		if err := h.Orchestrator.JoinSession(h.Peer, join.GetSessionId(), join.GetRole()); err != nil {
			slog.Warn("control: orchestrator rejected join", "peer", h.Peer.ID, "err", err)
			h.sendSessionEvent(crosstalkv1.SessionEventType_SESSION_ROLE_REJECTED, join.GetSessionId(), err.Error())
		}
		return
	}

	// Fallback: basic session lookup (no orchestrator).
	session, err := h.SessionService.FindSessionByID(join.GetSessionId())
	if err != nil || session == nil {
		msg := "session not found"
		if err != nil {
			msg = fmt.Sprintf("session lookup failed: %v", err)
		}
		h.sendSessionEvent(crosstalkv1.SessionEventType_SESSION_ROLE_REJECTED, join.GetSessionId(), msg)
		return
	}

	if join.GetRole() == "" {
		h.sendSessionEvent(crosstalkv1.SessionEventType_SESSION_ROLE_REJECTED, join.GetSessionId(), "role must not be empty")
		return
	}

	// Store session membership on the peer.
	h.Peer.mu.Lock()
	h.Peer.SessionID = join.GetSessionId()
	h.Peer.Role = join.GetRole()
	h.Peer.mu.Unlock()

	h.sendSessionEvent(crosstalkv1.SessionEventType_SESSION_CLIENT_JOINED, join.GetSessionId(), "joined as "+join.GetRole())
}

// sendSessionEvent sends a SessionEvent ControlMessage to the peer.
func (h *ControlHandler) sendSessionEvent(evType crosstalkv1.SessionEventType, sessionID, message string) {
	resp := &crosstalkv1.ControlMessage{
		Payload: &crosstalkv1.ControlMessage_SessionEvent{
			SessionEvent: &crosstalkv1.SessionEvent{
				Type:      evType,
				SessionId: sessionID,
				Message:   message,
			},
		},
	}
	if err := h.Peer.SendControlMessage(resp); err != nil {
		slog.Error("control: send SessionEvent failed", "peer", h.Peer.ID, "err", err)
	}
}
