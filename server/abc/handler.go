// Package abc provides server-side Audio Booth Connector lifecycle management.
// It handles ABC authentication via opaque token hash lookup, auto-joins ABCs
// to their assigned sessions, tracks connection state, and delivers restart commands.
package abc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"

	crosstalk "github.com/aleksclark/crosstalk/server"
	crosstalkv2 "github.com/aleksclark/crosstalk/server/proto/v2"
	"github.com/aleksclark/crosstalk/server/webrtc"
)

// Handler manages ABC connections on the server side.
// It authenticates ABCs via token hash, auto-joins them to assigned sessions,
// tracks connection state, and delivers restart commands.
type Handler struct {
	abcService     crosstalk.ABCService
	sessionService crosstalk.SessionService
	peerManager    *webrtc.PeerManager
	serverVersion  string

	mu    sync.RWMutex
	// peerToABC maps peer ID -> ABC record for connected ABCs.
	peerToABC map[string]*crosstalk.ABC
}

// NewHandler creates an ABC handler.
func NewHandler(
	abcSvc crosstalk.ABCService,
	sessionSvc crosstalk.SessionService,
	pm *webrtc.PeerManager,
	serverVersion string,
) *Handler {
	return &Handler{
		abcService:     abcSvc,
		sessionService: sessionSvc,
		peerManager:    pm,
		serverVersion:  serverVersion,
		peerToABC:      make(map[string]*crosstalk.ABC),
	}
}

// HashToken creates a SHA-256 hash of a raw token for lookup.
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// Authenticate validates a raw ABC token and returns the associated ABC record.
// It looks up the token hash in the database. Returns an error if not found.
func (h *Handler) Authenticate(ctx context.Context, token string) (*crosstalk.ABC, error) {
	if token == "" {
		return nil, fmt.Errorf("abc: empty token")
	}

	hash := HashToken(token)
	abc, err := h.abcService.GetByTokenHash(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("abc: authentication failed: %w", err)
	}

	return abc, nil
}

// OnConnect handles an ABC peer connection. It should be called when the control
// channel Hello message identifies the peer as an ABC client.
// It updates the ABC's connection state and sends a SessionAssignment if assigned.
func (h *Handler) OnConnect(ctx context.Context, peer *webrtc.PeerConn, abc *crosstalk.ABC) error {
	slog.Info("abc: client connected",
		"abc_id", abc.ID,
		"abc_name", abc.Name,
		"peer_id", peer.ID,
	)

	// Track the peer-to-ABC mapping.
	h.mu.Lock()
	h.peerToABC[peer.ID] = abc
	h.mu.Unlock()

	// Update connection state in DB.
	now := time.Now().UTC()
	abc.Connected = true
	abc.LastSeen = &now
	if err := h.abcService.Update(ctx, abc); err != nil {
		slog.Error("abc: failed to update connection state", "abc_id", abc.ID, "err", err)
		return fmt.Errorf("abc: update connection state: %w", err)
	}

	// If assigned to a session, send SessionAssignment.
	if abc.SessionID != nil && *abc.SessionID != "" {
		if err := h.sendSessionAssignment(peer, *abc.SessionID); err != nil {
			slog.Error("abc: failed to send session assignment",
				"abc_id", abc.ID,
				"session_id", *abc.SessionID,
				"err", err,
			)
			return err
		}
	}

	return nil
}

// OnDisconnect handles an ABC peer disconnection.
// It updates the ABC's connection state in the database.
func (h *Handler) OnDisconnect(ctx context.Context, peerID string) {
	h.mu.Lock()
	abc, ok := h.peerToABC[peerID]
	if ok {
		delete(h.peerToABC, peerID)
	}
	h.mu.Unlock()

	if !ok {
		return
	}

	slog.Info("abc: client disconnected", "abc_id", abc.ID, "peer_id", peerID)

	abc.Connected = false
	now := time.Now().UTC()
	abc.LastSeen = &now
	if err := h.abcService.Update(ctx, abc); err != nil {
		slog.Error("abc: failed to update disconnect state", "abc_id", abc.ID, "err", err)
	}
}

// OnSourceStatus handles a SourceStatus message from an ABC peer.
// It updates the ABC's LastSeen timestamp.
func (h *Handler) OnSourceStatus(ctx context.Context, peerID string, status *crosstalkv2.SourceStatus) {
	h.mu.RLock()
	abc, ok := h.peerToABC[peerID]
	h.mu.RUnlock()

	if !ok {
		return
	}

	slog.Debug("abc: source status",
		"abc_id", abc.ID,
		"source", status.GetSourceName(),
		"active", status.GetActive(),
		"peak", status.GetPeakLevel(),
	)

	// Update LastSeen.
	now := time.Now().UTC()
	abc.LastSeen = &now
	if err := h.abcService.Update(ctx, abc); err != nil {
		slog.Error("abc: failed to update last_seen", "abc_id", abc.ID, "err", err)
	}
}

// SendRestart sends a RestartCommand to the specified ABC.
// The ABC is identified by its ID; the handler looks up the currently connected peer.
func (h *Handler) SendRestart(ctx context.Context, abcID string, reason string) error {
	h.mu.RLock()
	var peerID string
	for pid, abc := range h.peerToABC {
		if abc.ID == abcID {
			peerID = pid
			break
		}
	}
	h.mu.RUnlock()

	if peerID == "" {
		return fmt.Errorf("abc: not connected (id=%s)", abcID)
	}

	peer := h.peerManager.FindPeer(peerID)
	if peer == nil {
		return fmt.Errorf("abc: peer not found (peer_id=%s)", peerID)
	}

	msg := &crosstalkv2.ControlMessage{
		Payload: &crosstalkv2.ControlMessage_Restart{
			Restart: &crosstalkv2.RestartCommand{
				Reason: reason,
			},
		},
	}

	slog.Info("abc: sending restart command", "abc_id", abcID, "peer_id", peerID, "reason", reason)
	return peer.SendControlMessage(msg)
}

// GetABCForPeer returns the ABC record associated with a connected peer, if any.
func (h *Handler) GetABCForPeer(peerID string) (*crosstalk.ABC, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	abc, ok := h.peerToABC[peerID]
	return abc, ok
}

// sendSessionAssignment sends a SessionAssignment message to the peer.
func (h *Handler) sendSessionAssignment(peer *webrtc.PeerConn, sessionID string) error {
	msg := &crosstalkv2.ControlMessage{
		Payload: &crosstalkv2.ControlMessage_SessionAssignment{
			SessionAssignment: &crosstalkv2.SessionAssignment{
				SessionId: sessionID,
				Role:      "abc",
			},
		},
	}

	slog.Info("abc: sending session assignment",
		"peer_id", peer.ID,
		"session_id", sessionID,
	)

	return peer.SendControlMessage(msg)
}
