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

	crosstalk "github.com/aleksclark/crosstalk/server"
	crosstalkv1 "github.com/aleksclark/crosstalk/proto/gen/go/crosstalk/v1"
)

// Orchestrator manages live session state and coordinates channel bindings
// between connected peers. It sits between the control channel handler and the
// Pion peer connections, evaluating template mappings as clients join/leave.
//
// TODO(phase5.5): Session client state (LiveSession, LiveClient, LiveBinding)
// is held only in memory. To survive server restarts, this state needs to be
// persisted — either by reconstructing it from the database on startup or by
// serializing the live session map. This is deferred; see roadmap task 5.5.
type Orchestrator struct {
	SessionService         crosstalk.SessionService
	SessionTemplateService crosstalk.SessionTemplateService
	PeerManager            *PeerManager
	RecordingPath          string

	mu       sync.Mutex
	sessions map[string]*LiveSession // keyed by session ID
}

func (o *Orchestrator) AssignSession(peerID, sessionID, role string) error {
	peer := o.PeerManager.FindPeer(peerID)
	if peer == nil {
		return fmt.Errorf("peer not found: %s", peerID)
	}
	return o.JoinSession(peer, sessionID, role)
}

// LiveSession tracks runtime state for an active session.
type LiveSession struct {
	Session   *crosstalk.Session
	Template  *crosstalk.SessionTemplate
	Clients   map[string]*LiveClient  // keyed by peer ID
	Bindings  map[string]*LiveBinding // keyed by channel ID
	Listeners map[string]*ListenerEntry // keyed by peer ID (broadcast listeners)
	StartedAt time.Time               // when the first client joined
}

// ListenerEntry tracks a broadcast listener peer and its active forwarding
// stop functions (one per broadcast binding being forwarded to this listener).
type ListenerEntry struct {
	Peer          *PeerConn
	StopForwards  map[string]func() // keyed by binding channel ID
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
			Listeners: make(map[string]*ListenerEntry),
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

	// Clean up empty sessions (no clients and no listeners).
	if len(ls.Clients) == 0 && len(ls.Listeners) == 0 {
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
		participants := make(map[string]string, len(ls.Clients))
		for peerID, c := range ls.Clients {
			participants[c.Role] = peerID
		}
		meta := &SessionMeta{
			SessionID:    sessionID,
			TemplateName: ls.Template.Name,
			Participants: participants,
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

	// Close all broadcast listener connections.
	for listenerID, entry := range ls.Listeners {
		for _, stop := range entry.StopForwards {
			stop()
		}
		entry.Peer.Close() //nolint:errcheck
		slog.Debug("orchestrator: closed broadcast listener on session end",
			"peer", listenerID, "session", sessionID)
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

	// Transition session from "waiting" to "active" when the first binding activates.
	if len(ls.Bindings) > 0 && ls.Session.Status == crosstalk.SessionWaiting {
		ls.Session.Status = crosstalk.SessionActive
		if err := o.SessionService.UpdateSessionStatus(ls.Session.ID, crosstalk.SessionActive); err != nil {
			slog.Error("orchestrator: failed to update session status to active",
				"session", ls.Session.ID, "err", err)
		}
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

	case "broadcast":
		// Forward broadcast audio to all connected listeners.
		for listenerID, entry := range ls.Listeners {
			stop, fwdErr := ForwardTrack(sourcePeer, entry.Peer, channelID)
			if fwdErr != nil {
				slog.Error("orchestrator: forward broadcast to listener failed",
					"channel", channelID, "listener", listenerID, "err", fwdErr)
				continue
			}
			entry.StopForwards[channelID] = stop
		}
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

	// Stop broadcast forwarding to all listeners for this binding.
	if lb.Binding.SinkType == "broadcast" {
		for _, entry := range ls.Listeners {
			if stop, ok := entry.StopForwards[channelID]; ok {
				stop()
				delete(entry.StopForwards, channelID)
			}
		}
	}

	// Close recorder if active.
	if lb.Recorder != nil {
		if err := lb.Recorder.Close(); err != nil {
			slog.Error("orchestrator: close recorder failed",
				"channel", channelID, "err", err)
		}
		sendSessionEvent(lb.SourcePeer, crosstalkv1.SessionEventType_SESSION_RECORDING_STOPPED,
			ls.Session.ID, "recording stopped for "+lb.Binding.SourceRole+":"+lb.Binding.SourceChannel)
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

	go func() {
		select {
		case <-done:
			return
		case ev := <-lb.SourcePeer.trackCh:
			remoteTrack := ev.track
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
		}
	}()

	slog.Info("orchestrator: started recording",
		"channel", lb.ChannelID, "path", filePath)

	// Emit recording started event to the source peer.
	sendSessionEvent(lb.SourcePeer, crosstalkv1.SessionEventType_SESSION_RECORDING_STARTED,
		ls.Session.ID, "recording started for "+lb.Binding.SourceRole+":"+lb.Binding.SourceChannel)
}

// GetLiveSession returns the live session for the given session ID, or nil.
// This is primarily useful for testing and inspection.
func (o *Orchestrator) GetLiveSession(sessionID string) *LiveSession {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.sessions[sessionID]
}

// RecordingStatus returns the current recording state for the given session.
// Returns nil if the session is not live or has no recording bindings.
func (o *Orchestrator) RecordingStatus(sessionID string) *crosstalk.RecordingInfo {
	o.mu.Lock()
	defer o.mu.Unlock()

	ls := o.sessions[sessionID]
	if ls == nil {
		return nil
	}

	info := &crosstalk.RecordingInfo{}
	for _, lb := range ls.Bindings {
		if lb.Recorder != nil {
			info.Active = true
			info.FileCount++
			fi, err := os.Stat(lb.Recorder.Path())
			if err == nil {
				info.TotalBytes += fi.Size()
			}
		}
	}
	return info
}

// AddListener adds a broadcast listener peer to the given session. If
// broadcast bindings are already active, forwarding to this listener starts
// immediately. Must be called with the orchestrator mutex NOT held.
func (o *Orchestrator) AddListener(peer *PeerConn, sessionID string) error {
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
			Listeners: make(map[string]*ListenerEntry),
			StartedAt: time.Now(),
		}
		o.sessions[sessionID] = ls
	}

	entry := &ListenerEntry{
		Peer:         peer,
		StopForwards: make(map[string]func()),
	}
	ls.Listeners[peer.ID] = entry

	// Store session membership on the peer.
	peer.mu.Lock()
	peer.SessionID = sessionID
	peer.Role = "__listener"
	peer.mu.Unlock()

	slog.Info("orchestrator: broadcast listener added",
		"peer", peer.ID, "session", sessionID,
		"listener_count", len(ls.Listeners))

	// Forward all active broadcast bindings to the new listener.
	for channelID, lb := range ls.Bindings {
		if lb.Binding.SinkType != "broadcast" {
			continue
		}
		stop, fwdErr := ForwardTrack(lb.SourcePeer, peer, channelID)
		if fwdErr != nil {
			slog.Error("orchestrator: forward broadcast to new listener failed",
				"channel", channelID, "listener", peer.ID, "err", fwdErr)
			continue
		}
		entry.StopForwards[channelID] = stop
	}

	// Notify all authenticated peers about the listener count change.
	countMsg := fmt.Sprintf("listener_count:%d", len(ls.Listeners))
	for _, c := range ls.Clients {
		sendSessionEvent(c.Peer, crosstalkv1.SessionEventType_SESSION_LISTENER_COUNT_CHANGED,
			sessionID, countMsg)
	}

	return nil
}

// RemoveListener removes a broadcast listener from its session and stops
// all forwarding to it. Must be called with the orchestrator mutex NOT held.
func (o *Orchestrator) RemoveListener(peerID string) {
	o.mu.Lock()
	defer o.mu.Unlock()

	// Find the session containing this listener.
	for sessionID, ls := range o.sessions {
		entry, ok := ls.Listeners[peerID]
		if !ok {
			continue
		}

		// Stop all forwards to this listener.
		for _, stop := range entry.StopForwards {
			stop()
		}

		delete(ls.Listeners, peerID)

		slog.Info("orchestrator: broadcast listener removed",
			"peer", peerID, "session", sessionID,
			"listener_count", len(ls.Listeners))

		// Notify all authenticated peers about the listener count change.
		countMsg := fmt.Sprintf("listener_count:%d", len(ls.Listeners))
		for _, c := range ls.Clients {
			sendSessionEvent(c.Peer, crosstalkv1.SessionEventType_SESSION_LISTENER_COUNT_CHANGED,
				sessionID, countMsg)
		}

		// Clean up empty sessions.
		if len(ls.Clients) == 0 && len(ls.Listeners) == 0 {
			delete(o.sessions, sessionID)
		}
		return
	}
}

// ListenerCount returns the number of broadcast listeners for the given session.
func (o *Orchestrator) ListenerCount(sessionID string) int {
	o.mu.Lock()
	defer o.mu.Unlock()
	ls := o.sessions[sessionID]
	if ls == nil {
		return 0
	}
	return len(ls.Listeners)
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
