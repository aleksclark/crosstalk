package webrtc

import (
	"log/slog"
	"time"

	"github.com/pion/webrtc/v4"
	"google.golang.org/protobuf/proto"

	crosstalkv2 "github.com/aleksclark/crosstalk/server/proto/v2"
)

// MaxControlMessageBytes is the hard cap for inbound control data-channel
// frames before unmarshal / application callbacks run.
const MaxControlMessageBytes = 16 * 1024

// ControlHandler processes protobuf messages on the control data channel.
// It handles the v2 protocol (Hello/Welcome/PingPong/etc.) and emits debug events.
type ControlHandler struct {
	Peer          *PeerConn
	ServerVersion string

	// AssignedSessionID, when set, is sent back to the client in the Welcome so
	// an ABC knows which session it has been bridged into.
	AssignedSessionID string

	// Callbacks for application-level processing.
	OnHello              func(peer *PeerConn, hello *crosstalkv2.Hello)
	OnSourceStatus       func(peer *PeerConn, status *crosstalkv2.SourceStatus)
	OnAudioControlReport func(peer *PeerConn, report *crosstalkv2.AudioControlReport)
}

// Install registers the message handler on the peer's control data channel.
func (h *ControlHandler) Install() {
	dc := h.Peer.DataChannel()
	if dc == nil {
		slog.Warn("webrtc: control handler install: no data channel", "peer", h.Peer.ID)
		return
	}

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		h.Peer.events.Push(MakeEvent(h.Peer.ID, EventDataChannelMessage, map[string]any{
			"label": dc.Label(),
			"bytes": len(msg.Data),
		}))
		h.dispatch(msg.Data)
	})

	slog.Debug("webrtc: control handler installed", "peer", h.Peer.ID)
}

// dispatch unmarshals and routes protobuf messages.
func (h *ControlHandler) dispatch(data []byte) {
	if len(data) > MaxControlMessageBytes {
		slog.Warn("webrtc: control message rejected: oversized",
			"peer", h.Peer.ID,
			"bytes", len(data),
			"max", MaxControlMessageBytes,
		)
		return
	}

	var cm crosstalkv2.ControlMessage
	if err := proto.Unmarshal(data, &cm); err != nil {
		slog.Error("webrtc: control unmarshal failed", "peer", h.Peer.ID, "err", err)
		return
	}

	switch payload := cm.GetPayload().(type) {
	case *crosstalkv2.ControlMessage_Hello:
		h.handleHello(payload.Hello)
	case *crosstalkv2.ControlMessage_SourceStatus:
		h.handleSourceStatus(payload.SourceStatus)
	case *crosstalkv2.ControlMessage_AudioControlReport:
		h.handleAudioControlReport(payload.AudioControlReport)
	case *crosstalkv2.ControlMessage_Ping:
		h.handlePing(payload.Ping)
	case *crosstalkv2.ControlMessage_LogEntry:
		h.handleLogEntry(payload.LogEntry)
	default:
		slog.Warn("webrtc: unhandled control message type", "peer", h.Peer.ID)
	}
}

// handleAudioControlReport forwards a board audio inventory/apply report.
func (h *ControlHandler) handleAudioControlReport(report *crosstalkv2.AudioControlReport) {
	if report == nil {
		return
	}
	slog.Debug("webrtc: received AudioControlReport",
		"peer", h.Peer.ID,
		"desired_revision", report.GetDesiredRevision(),
		"command_id", report.GetCommandId(),
		"devices", len(report.GetDevices()),
		"error_code", report.GetErrorCode(),
	)
	if h.OnAudioControlReport != nil {
		h.OnAudioControlReport(h.Peer, report)
	}
}

// handleHello processes the client Hello and responds with Welcome.
func (h *ControlHandler) handleHello(hello *crosstalkv2.Hello) {
	slog.Info("webrtc: received Hello",
		"peer", h.Peer.ID,
		"client_type", hello.GetClientType(),
		"client_name", hello.GetClientName(),
		"capabilities", len(hello.GetCapabilities()))

	h.Peer.SetClientInfo(hello.GetClientType(), hello.GetClientName())
	h.Peer.events.Push(MakeEvent(h.Peer.ID, EventControlHello, map[string]string{
		"client_type": hello.GetClientType(),
		"client_name": hello.GetClientName(),
	}))

	if h.OnHello != nil {
		h.OnHello(h.Peer, hello)
	}

	// Send Welcome response.
	welcome := &crosstalkv2.ControlMessage{
		Payload: &crosstalkv2.ControlMessage_Welcome{
			Welcome: &crosstalkv2.Welcome{
				PeerId:            h.Peer.ID,
				ServerVersion:     h.ServerVersion,
				AssignedSessionId: h.AssignedSessionID,
			},
		},
	}
	h.Peer.events.Push(MakeEvent(h.Peer.ID, EventControlWelcome, map[string]string{
		"peer_id":             h.Peer.ID,
		"server_version":      h.ServerVersion,
		"assigned_session_id": h.AssignedSessionID,
	}))

	if err := h.sendMessage(welcome); err != nil {
		slog.Error("webrtc: send Welcome failed", "peer", h.Peer.ID, "err", err)
	} else {
		slog.Info("webrtc: Welcome sent", "peer", h.Peer.ID)
	}
}

// handleSourceStatus processes source status updates.
func (h *ControlHandler) handleSourceStatus(status *crosstalkv2.SourceStatus) {
	slog.Debug("webrtc: received SourceStatus",
		"peer", h.Peer.ID,
		"source", status.GetSourceName(),
		"active", status.GetActive(),
		"peak", status.GetPeakLevel())

	if h.OnSourceStatus != nil {
		h.OnSourceStatus(h.Peer, status)
	}
}

// handlePing responds to a PingPong message with the same timestamp (pong).
func (h *ControlHandler) handlePing(ping *crosstalkv2.PingPong) {
	now := time.Now().UnixMilli()
	latency := now - ping.GetSentAt()

	h.Peer.events.Push(MakeEvent(h.Peer.ID, EventControlPing, map[string]any{
		"sent_at":    ping.GetSentAt(),
		"received_at": now,
		"latency_ms": latency,
	}))

	slog.Debug("webrtc: ping received",
		"peer", h.Peer.ID,
		"latency_ms", latency)

	// Respond with pong (same message with current time).
	pong := &crosstalkv2.ControlMessage{
		Payload: &crosstalkv2.ControlMessage_Ping{
			Ping: &crosstalkv2.PingPong{
				SentAt: now,
			},
		},
	}

	h.Peer.events.Push(MakeEvent(h.Peer.ID, EventControlPong, map[string]any{
		"sent_at": now,
	}))

	if err := h.sendMessage(pong); err != nil {
		slog.Error("webrtc: send pong failed", "peer", h.Peer.ID, "err", err)
	}
}

// handleLogEntry logs a client-sent log entry.
func (h *ControlHandler) handleLogEntry(entry *crosstalkv2.LogEntry) {
	slog.Info("webrtc: client log",
		"peer", h.Peer.ID,
		"severity", entry.GetSeverity(),
		"source", entry.GetSource(),
		"message", entry.GetMessage())
}

// sendMessage marshals and sends a protobuf control message.
func (h *ControlHandler) sendMessage(msg *crosstalkv2.ControlMessage) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	dc := h.Peer.DataChannel()
	if dc == nil {
		return nil
	}
	return dc.Send(data)
}

// SendControlMessage sends a protobuf message on the peer's data channel.
func (c *PeerConn) SendControlMessage(msg *crosstalkv2.ControlMessage) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	dc := c.DataChannel()
	if dc == nil {
		return nil
	}
	return dc.Send(data)
}
