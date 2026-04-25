package pion

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/pion/webrtc/v4"
	"google.golang.org/protobuf/proto"

	crosstalk "github.com/aleksclark/crosstalk/server"
	crosstalkv1 "github.com/aleksclark/crosstalk/proto/gen/go/crosstalk/v1"
)

// ControlHandler processes protobuf (and JSON) messages on the control data
// channel. Protobuf is the primary wire format (used by ct-client / K2B).
// Browser clients send JSON; when the first message fails protobuf decode but
// succeeds as JSON, the peer is switched to JSON mode and all responses are
// sent back as JSON.
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
// control-message dispatcher. It installs on the server-created DC and also
// registers the callback so that client-created DCs (captured by OnDataChannel
// in createControlChannel) use the same dispatcher.
func (h *ControlHandler) Install() {
	slog.Debug("control: installing handler", "peer", h.Peer.ID)

	dispatchFn := func(data []byte) {
		h.dispatch(data)
	}

	h.Peer.mu.Lock()
	h.Peer.onControlMessage = dispatchFn
	clientDC := h.Peer.clientControl
	h.Peer.mu.Unlock()

	if clientDC != nil {
		clientDC.OnMessage(func(msg webrtc.DataChannelMessage) {
			slog.Debug("control: message received (client DC)",
				"peer", h.Peer.ID, "bytes", len(msg.Data))
			dispatchFn(msg.Data)
		})
	} else {
		h.Peer.control.OnMessage(func(msg webrtc.DataChannelMessage) {
			slog.Debug("control: message received (server DC)",
				"peer", h.Peer.ID, "bytes", len(msg.Data))
			dispatchFn(msg.Data)
		})
	}
}

// dispatch unmarshals the raw bytes into a ControlMessage and routes to the
// appropriate handler based on the payload oneof. If protobuf decoding fails,
// it falls back to JSON parsing for browser clients.
func (h *ControlHandler) dispatch(data []byte) {
	var cm crosstalkv1.ControlMessage
	if err := proto.Unmarshal(data, &cm); err != nil {
		// Protobuf decode failed — try JSON fallback for browser clients.
		if h.dispatchJSON(data) {
			return
		}
		slog.Error("control: unmarshal failed (neither protobuf nor JSON)",
			"peer", h.Peer.ID, "err", err)
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

// jsonControlMessage mirrors the JSON structure sent by browser clients.
// The browser sends objects like: {"hello": {...}}, {"joinSession": {...}}.
type jsonControlMessage struct {
	Hello       *jsonHello       `json:"hello,omitempty"`
	JoinSession *jsonJoinSession `json:"joinSession,omitempty"`
	LogEntry    *jsonLogEntry    `json:"logEntry,omitempty"`
}

type jsonHello struct {
	Sources []jsonSourceInfo `json:"sources,omitempty"`
	Sinks   []jsonSinkInfo   `json:"sinks,omitempty"`
	Codecs  []jsonCodecInfo  `json:"codecs,omitempty"`
}

type jsonSourceInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type jsonSinkInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type jsonCodecInfo struct {
	Name      string `json:"name"`
	MediaType string `json:"mediaType"`
}

type jsonJoinSession struct {
	SessionID string `json:"sessionId"`
	Role      string `json:"role"`
}

type jsonLogEntry struct {
	Timestamp int64  `json:"timestamp"`
	Severity  string `json:"severity"`
	Source    string `json:"source"`
	Message  string `json:"message"`
}

// dispatchJSON attempts to parse data as a JSON control message. If
// successful, it switches the peer to JSON mode and dispatches the message.
// Returns true if the data was valid JSON and was handled.
func (h *ControlHandler) dispatchJSON(data []byte) bool {
	var jm jsonControlMessage
	if err := json.Unmarshal(data, &jm); err != nil {
		return false
	}

	// Switch peer to JSON mode on first successful JSON parse.
	h.Peer.mu.Lock()
	if !h.Peer.jsonMode {
		slog.Info("control: switching peer to JSON mode", "peer", h.Peer.ID)
		h.Peer.jsonMode = true
	}
	h.Peer.mu.Unlock()

	switch {
	case jm.Hello != nil:
		hello := &crosstalkv1.Hello{}
		for _, s := range jm.Hello.Sources {
			hello.Sources = append(hello.Sources, &crosstalkv1.SourceInfo{
				Name: s.Name, Type: s.Type,
			})
		}
		for _, s := range jm.Hello.Sinks {
			hello.Sinks = append(hello.Sinks, &crosstalkv1.SinkInfo{
				Name: s.Name, Type: s.Type,
			})
		}
		for _, c := range jm.Hello.Codecs {
			hello.Codecs = append(hello.Codecs, &crosstalkv1.CodecInfo{
				Name: c.Name, MediaType: c.MediaType,
			})
		}
		h.handleHello(hello)

	case jm.JoinSession != nil:
		join := &crosstalkv1.JoinSession{
			SessionId: jm.JoinSession.SessionID,
			Role:      jm.JoinSession.Role,
		}
		h.handleJoinSession(join)

	case jm.LogEntry != nil:
		entry := &crosstalkv1.LogEntry{
			Timestamp: jm.LogEntry.Timestamp,
			Source:    jm.LogEntry.Source,
			Message:   jm.LogEntry.Message,
		}
		h.handleLogEntry(entry)

	default:
		slog.Warn("control: JSON message has no recognised payload",
			"peer", h.Peer.ID, "raw", string(data))
	}

	return true
}

// controlMessageToJSON converts a protobuf ControlMessage to a JSON byte
// slice using the same camelCase keys the browser client expects.
func controlMessageToJSON(msg *crosstalkv1.ControlMessage) ([]byte, error) {
	envelope := make(map[string]any)

	switch payload := msg.GetPayload().(type) {
	case *crosstalkv1.ControlMessage_Welcome:
		envelope["welcome"] = map[string]any{
			"clientId":      payload.Welcome.GetClientId(),
			"serverVersion": payload.Welcome.GetServerVersion(),
		}
	case *crosstalkv1.ControlMessage_BindChannel:
		envelope["bindChannel"] = map[string]any{
			"channelId": payload.BindChannel.GetChannelId(),
			"localName": payload.BindChannel.GetLocalName(),
			"direction": directionToString(payload.BindChannel.GetDirection()),
			"trackId":   payload.BindChannel.GetTrackId(),
		}
	case *crosstalkv1.ControlMessage_UnbindChannel:
		envelope["unbindChannel"] = map[string]any{
			"channelId": payload.UnbindChannel.GetChannelId(),
		}
	case *crosstalkv1.ControlMessage_SessionEvent:
		envelope["sessionEvent"] = map[string]any{
			"type":      payload.SessionEvent.GetType().String(),
			"sessionId": payload.SessionEvent.GetSessionId(),
			"message":   payload.SessionEvent.GetMessage(),
		}
	case *crosstalkv1.ControlMessage_LogEntry:
		envelope["logEntry"] = map[string]any{
			"timestamp": payload.LogEntry.GetTimestamp(),
			"severity":  payload.LogEntry.GetSeverity().String(),
			"source":    payload.LogEntry.GetSource(),
			"message":   payload.LogEntry.GetMessage(),
		}
	default:
		return nil, fmt.Errorf("unsupported message type for JSON: %T", msg.GetPayload())
	}

	return json.Marshal(envelope)
}

// directionToString converts a protobuf Direction enum to the string the
// browser client expects. The browser TypeScript types define these as
// uppercase "SOURCE" | "SINK" (see webrtc-types.ts BindChannelMessage).
func directionToString(d crosstalkv1.Direction) string {
	switch d {
	case crosstalkv1.Direction_SOURCE:
		return "SOURCE"
	case crosstalkv1.Direction_SINK:
		return "SINK"
	default:
		return "UNKNOWN"
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
	} else {
		slog.Info("control: Welcome sent", "peer", h.Peer.ID, "json_mode", h.Peer.jsonMode)
	}
}

// handleClientStatus logs the status update.
// TODO(Phase 5): update client registry with reported state, sources, sinks, codecs.
func (h *ControlHandler) handleClientStatus(status *crosstalkv1.ClientStatus) {
	slog.Debug("control: received ClientStatus", "peer", h.Peer.ID, "state", status.GetState())
}

// handleChannelStatus logs the channel status report.
// TODO(Phase 5): update channel binding state in session registry.
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
