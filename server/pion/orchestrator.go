package pion

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/pion/webrtc/v4"

	crosstalk "github.com/aleksclark/crosstalk/server"
	crosstalkv1 "github.com/aleksclark/crosstalk/proto/gen/go/crosstalk/v1"
)

// Orchestrator manages live session state and coordinates channel bindings
// between connected peers. It sits between the control channel handler and the
// Pion peer connections, evaluating template mappings as clients join/leave.
type Orchestrator struct {
	SessionService         crosstalk.SessionService
	SessionTemplateService crosstalk.SessionTemplateService
	RecordingPath          string // base directory for recording files; empty disables recording

	mu       sync.Mutex
	sessions map[string]*LiveSession // keyed by session ID
}

// LiveSession tracks runtime state for an active session.
type LiveSession struct {
	Session   *crosstalk.Session
	Template  *crosstalk.SessionTemplate
	Clients   map[string]*LiveClient  // keyed by peer ID
	Bindings  map[string]*LiveBinding // keyed by channel ID
	StartedAt time.Time               // when the first client joined
}

// LiveClient tracks a connected client's role and peer.
type LiveClient struct {
	Peer *PeerConn
	Role string
}

// LiveBinding tracks an active binding with its forwarding state.
type LiveBinding struct {
	Binding    crosstalk.ActiveBinding
	ChannelID  string
	SourcePeer *PeerConn
	SinkPeer   *PeerConn // nil for record/broadcast
	// Track forwarding goroutine control.
	stopForward func()
	// Recording state (non-nil for record bindings).
	Recorder    *Recorder
	RecordStart time.Time
}

// NewOrchestrator creates an Orchestrator with the given service dependencies.
func NewOrchestrator(ss crosstalk.SessionService, sts crosstalk.SessionTemplateService) *Orchestrator {
	return &Orchestrator{
		SessionService:         ss,
		SessionTemplateService: sts,
		sessions:               make(map[string]*LiveSession),
	}
}

// JoinSession validates the session and role, enforces cardinality, registers
// the peer, evaluates bindings, and sends BindChannel commands to affected peers.
func (o *Orchestrator) JoinSession(peer *PeerConn, sessionID, role string) error {
	// Look up the session.
	session, err := o.SessionService.FindSessionByID(sessionID)
	if err != nil || session == nil {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	if session.Status == crosstalk.SessionEnded {
		return fmt.Errorf("session already ended: %s", sessionID)
	}

	// Look up the template.
	tmpl, err := o.SessionTemplateService.FindTemplateByID(session.TemplateID)
	if err != nil || tmpl == nil {
		return fmt.Errorf("template not found for session %s", sessionID)
	}

	// Validate role exists in template.
	var foundRole *crosstalk.Role
	for i := range tmpl.Roles {
		if tmpl.Roles[i].Name == role {
			foundRole = &tmpl.Roles[i]
			break
		}
	}
	if foundRole == nil {
		return fmt.Errorf("role %q not defined in template %s", role, tmpl.ID)
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	// Get or create the live session.
	ls := o.sessions[sessionID]
	if ls == nil {
		ls = &LiveSession{
			Session:   session,
			Template:  tmpl,
			Clients:   make(map[string]*LiveClient),
			Bindings:  make(map[string]*LiveBinding),
			StartedAt: time.Now(),
		}
		o.sessions[sessionID] = ls
	}

	// Enforce single-client cardinality.
	if !foundRole.MultiClient {
		for _, c := range ls.Clients {
			if c.Role == role {
				return fmt.Errorf("role %q already occupied (single-client)", role)
			}
		}
	}

	// Register the client.
	ls.Clients[peer.ID] = &LiveClient{
		Peer: peer,
		Role: role,
	}

	// Store session membership on the peer.
	peer.mu.Lock()
	peer.SessionID = sessionID
	peer.Role = role
	peer.mu.Unlock()

	slog.Info("orchestrator: client joined session",
		"peer", peer.ID, "session", sessionID, "role", role)

	// Send joined event to the joining peer.
	sendSessionEvent(peer, crosstalkv1.SessionEventType_SESSION_CLIENT_JOINED, sessionID,
		"joined as "+role)

	// Evaluate bindings with the new set of connected roles.
	o.evaluateBindings(ls)

	return nil
}

// LeaveSession removes a client from its session, re-evaluates bindings,
// sends UnbindChannel for deactivated bindings, and stops forwarding.
func (o *Orchestrator) LeaveSession(peer *PeerConn) {
	o.mu.Lock()
	defer o.mu.Unlock()

	peer.mu.Lock()
	sessionID := peer.SessionID
	peer.SessionID = ""
	peer.Role = ""
	peer.mu.Unlock()

	if sessionID == "" {
		return
	}

	ls := o.sessions[sessionID]
	if ls == nil {
		return
	}

	delete(ls.Clients, peer.ID)

	slog.Info("orchestrator: client left session",
		"peer", peer.ID, "session", sessionID)

	// Notify remaining clients.
	for _, c := range ls.Clients {
		sendSessionEvent(c.Peer, crosstalkv1.SessionEventType_SESSION_CLIENT_LEFT,
			sessionID, "peer "+peer.ID+" left")
	}

	// Re-evaluate bindings — some may need to be deactivated.
	o.evaluateBindings(ls)

	// Clean up empty sessions.
	if len(ls.Clients) == 0 {
		delete(o.sessions, sessionID)
	}
}

// EndSession sends SessionEnded to all clients, stops all forwarding,
// writes session metadata, and cleans up the live session.
func (o *Orchestrator) EndSession(sessionID string) {
	o.mu.Lock()
	defer o.mu.Unlock()

	ls := o.sessions[sessionID]
	if ls == nil {
		return
	}

	// Collect recording metadata before deactivating bindings.
	var recordingFiles []RecordingFile
	for _, lb := range ls.Bindings {
		if lb.Recorder != nil {
			recordingFiles = append(recordingFiles, RecordingFile{
				Path:          lb.Recorder.Path(),
				SourceRole:    lb.Binding.SourceRole,
				SourceChannel: lb.Binding.SourceChannel,
				StartedAt:     lb.RecordStart,
			})
		}
	}

	// Deactivate all bindings.
	for channelID, lb := range ls.Bindings {
		o.deactivateBinding(ls, channelID, lb)
	}

	// Write session-meta.json if any recordings were made.
	if len(recordingFiles) > 0 && o.RecordingPath != "" {
		dir := filepath.Join(o.RecordingPath, sessionID)
		meta := &SessionMeta{
			SessionID:    sessionID,
			TemplateName: ls.Template.Name,
			StartedAt:    ls.StartedAt,
			EndedAt:      time.Now(),
			Files:        recordingFiles,
		}
		if err := WriteSessionMeta(dir, meta); err != nil {
			slog.Error("orchestrator: write session meta failed",
				"session", sessionID, "err", err)
		}
	}

	// Notify all clients.
	for _, c := range ls.Clients {
		sendSessionEvent(c.Peer, crosstalkv1.SessionEventType_SESSION_ENDED,
			sessionID, "session ended")
	}

	delete(o.sessions, sessionID)

	slog.Info("orchestrator: session ended", "session", sessionID)
}

// connectedRoles returns the set of distinct role names for the live session.
func connectedRoles(ls *LiveSession) map[string]bool {
	roles := make(map[string]bool, len(ls.Clients))
	for _, c := range ls.Clients {
		roles[c.Role] = true
	}
	return roles
}

// evaluateBindings computes which template mappings should be active given
// the currently connected roles, diffs against existing bindings, and
// activates/deactivates as needed. Must be called with o.mu held.
func (o *Orchestrator) evaluateBindings(ls *LiveSession) {
	roles := connectedRoles(ls)
	desired := crosstalk.ResolveBindings(ls.Template, roles)

	// Build a set of desired binding keys for fast lookup.
	desiredSet := make(map[string]crosstalk.ActiveBinding, len(desired))
	for _, b := range desired {
		key := bindingKey(b)
		desiredSet[key] = b
	}

	// Deactivate bindings that are no longer desired.
	for channelID, lb := range ls.Bindings {
		key := bindingKey(lb.Binding)
		if _, ok := desiredSet[key]; !ok {
			o.deactivateBinding(ls, channelID, lb)
		}
	}

	// Build set of currently active binding keys.
	activeSet := make(map[string]bool, len(ls.Bindings))
	for _, lb := range ls.Bindings {
		activeSet[bindingKey(lb.Binding)] = true
	}

	// Activate new bindings.
	for _, b := range desired {
		key := bindingKey(b)
		if activeSet[key] {
			continue // already active
		}
		o.activateBinding(ls, b)
	}
}

// bindingKey returns a unique string key for an ActiveBinding based on its
// source and sink mapping.
func bindingKey(b crosstalk.ActiveBinding) string {
	return b.Mapping.Source + "->" + b.Mapping.Sink
}

// peerForRole returns the first peer connected with the given role, or nil.
func peerForRole(ls *LiveSession, role string) *PeerConn {
	for _, c := range ls.Clients {
		if c.Role == role {
			return c.Peer
		}
	}
	return nil
}

// activateBinding sends BindChannel commands to the relevant peers and starts
// SFU forwarding for role→role bindings. Must be called with o.mu held.
func (o *Orchestrator) activateBinding(ls *LiveSession, b crosstalk.ActiveBinding) {
	channelID := ulid.Make().String()

	sourcePeer := peerForRole(ls, b.SourceRole)
	if sourcePeer == nil {
		return // shouldn't happen if ResolveBindings is correct
	}

	lb := &LiveBinding{
		Binding:    b,
		ChannelID:  channelID,
		SourcePeer: sourcePeer,
	}

	// Tell source peer to send audio on this channel.
	sendBindChannel(sourcePeer, channelID, b.SourceChannel, crosstalkv1.Direction_SOURCE, channelID)

	switch b.SinkType {
	case "role":
		sinkPeer := peerForRole(ls, b.SinkRole)
		if sinkPeer == nil {
			return
		}
		lb.SinkPeer = sinkPeer

		// Tell sink peer to receive audio on this channel.
		sendBindChannel(sinkPeer, channelID, b.SinkChannel, crosstalkv1.Direction_SINK, channelID)

		// Start SFU track forwarding.
		stop, err := ForwardTrack(sourcePeer, sinkPeer, channelID)
		if err != nil {
			slog.Error("orchestrator: failed to start track forwarding",
				"channel", channelID, "err", err)
		} else {
			lb.stopForward = stop
		}

	case "record":
		o.startRecording(ls, lb)
	}

	ls.Bindings[channelID] = lb

	slog.Info("orchestrator: activated binding",
		"channel", channelID,
		"source", b.Mapping.Source, "sink", b.Mapping.Sink)
}

// deactivateBinding sends UnbindChannel to affected peers and stops
// forwarding. Must be called with o.mu held.
func (o *Orchestrator) deactivateBinding(ls *LiveSession, channelID string, lb *LiveBinding) {
	if lb.stopForward != nil {
		lb.stopForward()
	}

	// Close recorder if active.
	if lb.Recorder != nil {
		if err := lb.Recorder.Close(); err != nil {
			slog.Error("orchestrator: close recorder failed",
				"channel", channelID, "err", err)
		}
	}

	// Send UnbindChannel to source.
	sendUnbindChannel(lb.SourcePeer, channelID)

	// Send UnbindChannel to sink if role→role.
	if lb.SinkPeer != nil {
		sendUnbindChannel(lb.SinkPeer, channelID)
	}

	delete(ls.Bindings, channelID)

	slog.Info("orchestrator: deactivated binding",
		"channel", channelID,
		"source", lb.Binding.Mapping.Source, "sink", lb.Binding.Mapping.Sink)
}

// startRecording sets up a Recorder for a record binding and wires it to the
// source peer's incoming audio track. On any error it logs and skips recording
// rather than failing the session. Must be called with o.mu held.
func (o *Orchestrator) startRecording(ls *LiveSession, lb *LiveBinding) {
	if o.RecordingPath == "" {
		slog.Warn("orchestrator: recording path not configured, skipping recording",
			"channel", lb.ChannelID)
		return
	}

	sessionID := ls.Session.ID
	dir := filepath.Join(o.RecordingPath, sessionID)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		slog.Error("orchestrator: failed to create recording directory",
			"dir", dir, "err", err)
		return
	}

	now := time.Now()
	fileName := RecordingFileName(lb.Binding.SourceRole, lb.Binding.SourceChannel, now)
	filePath := filepath.Join(dir, fileName)

	rec, err := NewRecorder(filePath)
	if err != nil {
		slog.Error("orchestrator: failed to create recorder",
			"path", filePath, "err", err)
		return
	}

	lb.Recorder = rec
	lb.RecordStart = now

	// Register OnTrack on the source peer to capture incoming audio and write
	// RTP packets to the recorder. The forwarding goroutine runs until the
	// track ends or the stop function is called.
	var (
		stopOnce sync.Once
		done     = make(chan struct{})
	)

	lb.stopForward = func() {
		stopOnce.Do(func() { close(done) })
	}

	lb.SourcePeer.pc.OnTrack(func(remoteTrack *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		go func() {
			for {
				select {
				case <-done:
					return
				default:
				}

				pkt, _, readErr := remoteTrack.ReadRTP()
				if readErr != nil {
					if readErr != io.EOF {
						slog.Debug("orchestrator: recording track read error",
							"channel", lb.ChannelID, "err", readErr)
					}
					return
				}

				if writeErr := rec.WriteRTP(pkt); writeErr != nil {
					slog.Debug("orchestrator: recording WriteRTP failed",
						"channel", lb.ChannelID, "err", writeErr)
				}
			}
		}()
	})

	slog.Info("orchestrator: started recording",
		"channel", lb.ChannelID, "path", filePath)
}

// GetLiveSession returns the live session for the given session ID, or nil.
// This is primarily useful for testing and inspection.
func (o *Orchestrator) GetLiveSession(sessionID string) *LiveSession {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.sessions[sessionID]
}

// sendBindChannel sends a BindChannel control message to a peer.
func sendBindChannel(peer *PeerConn, channelID, localName string, dir crosstalkv1.Direction, trackID string) {
	msg := &crosstalkv1.ControlMessage{
		Payload: &crosstalkv1.ControlMessage_BindChannel{
			BindChannel: &crosstalkv1.BindChannel{
				ChannelId: channelID,
				LocalName: localName,
				Direction: dir,
				TrackId:   trackID,
			},
		},
	}
	if err := peer.SendControlMessage(msg); err != nil {
		slog.Error("orchestrator: send BindChannel failed",
			"peer", peer.ID, "channel", channelID, "err", err)
	}
}

// sendUnbindChannel sends an UnbindChannel control message to a peer.
func sendUnbindChannel(peer *PeerConn, channelID string) {
	msg := &crosstalkv1.ControlMessage{
		Payload: &crosstalkv1.ControlMessage_UnbindChannel{
			UnbindChannel: &crosstalkv1.UnbindChannel{
				ChannelId: channelID,
			},
		},
	}
	if err := peer.SendControlMessage(msg); err != nil {
		slog.Error("orchestrator: send UnbindChannel failed",
			"peer", peer.ID, "channel", channelID, "err", err)
	}
}

// sendSessionEvent sends a SessionEvent control message to a peer.
func sendSessionEvent(peer *PeerConn, evType crosstalkv1.SessionEventType, sessionID, message string) {
	msg := &crosstalkv1.ControlMessage{
		Payload: &crosstalkv1.ControlMessage_SessionEvent{
			SessionEvent: &crosstalkv1.SessionEvent{
				Type:      evType,
				SessionId: sessionID,
				Message:   message,
			},
		},
	}
	if err := peer.SendControlMessage(msg); err != nil {
		slog.Error("orchestrator: send SessionEvent failed",
			"peer", peer.ID, "type", evType, "err", err)
	}
}
