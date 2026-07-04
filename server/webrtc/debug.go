package webrtc

import (
	"encoding/json"
	"net/http"
	"strings"
)

// DebugHandler provides HTTP endpoints for inspecting WebRTC peer state.
// All endpoints return JSON.
type DebugHandler struct {
	PeerManager *PeerManager
}

// RegisterRoutes registers all debug API endpoints on the given mux.
// Expects a mux that handles path routing (e.g., chi or stdlib ServeMux).
func (h *DebugHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/debug/peers", h.HandleListPeers)
	mux.HandleFunc("GET /api/debug/peers/{id}", h.handlePeerDetail)
	mux.HandleFunc("GET /api/debug/peers/{id}/events", h.handlePeerEvents)
	mux.HandleFunc("GET /api/debug/peers/{id}/sdp", h.handlePeerSDP)
}

// ServeHTTP implements http.Handler with path-based routing.
// This allows DebugHandler to be used directly without a mux if needed.
func (h *DebugHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	switch {
	case path == "/api/debug/peers" && r.Method == "GET":
		h.HandleListPeers(w, r)
	case strings.HasSuffix(path, "/events") && r.Method == "GET":
		// Extract peer ID from /api/debug/peers/{id}/events.
		h.HandlePeerEventsFromPath(w, r)
	case strings.HasSuffix(path, "/sdp") && r.Method == "GET":
		h.HandlePeerSDPFromPath(w, r)
	case strings.HasPrefix(path, "/api/debug/peers/") && r.Method == "GET":
		h.HandlePeerDetailFromPath(w, r)
	default:
		http.NotFound(w, r)
	}
}

// HandleListPeers returns all active peers with their connection state.
func (h *DebugHandler) HandleListPeers(w http.ResponseWriter, _ *http.Request) {
	states := h.PeerManager.PeerStates()
	writeJSON(w, http.StatusOK, map[string]any{
		"peers": states,
		"count": len(states),
	})
}

// handlePeerDetail returns detailed state for a single peer.
func (h *DebugHandler) handlePeerDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.peerDetail(w, id)
}

func (h *DebugHandler) HandlePeerDetailFromPath(w http.ResponseWriter, r *http.Request) {
	id := extractPeerID(r.URL.Path)
	h.peerDetail(w, id)
}

func (h *DebugHandler) peerDetail(w http.ResponseWriter, id string) {
	peer := h.PeerManager.FindPeer(id)
	if peer == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "peer not found"})
		return
	}
	state := peer.State()
	writeJSON(w, http.StatusOK, state)
}

// handlePeerEvents returns the event ring buffer for a peer.
func (h *DebugHandler) handlePeerEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.peerEvents(w, id)
}

func (h *DebugHandler) HandlePeerEventsFromPath(w http.ResponseWriter, r *http.Request) {
	id := extractPeerID(r.URL.Path)
	h.peerEvents(w, id)
}

func (h *DebugHandler) peerEvents(w http.ResponseWriter, id string) {
	events, ok := h.PeerManager.PeerEvents(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "peer not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"peer_id": id,
		"events":  events,
		"count":   len(events),
	})
}

// handlePeerSDP returns the last offer/answer for a peer.
func (h *DebugHandler) handlePeerSDP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.peerSDP(w, id)
}

func (h *DebugHandler) HandlePeerSDPFromPath(w http.ResponseWriter, r *http.Request) {
	id := extractPeerID(r.URL.Path)
	h.peerSDP(w, id)
}

func (h *DebugHandler) peerSDP(w http.ResponseWriter, id string) {
	offer, answer, found := h.PeerManager.PeerSDP(id)
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "peer not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"peer_id":     id,
		"last_offer":  offer,
		"last_answer": answer,
	})
}

// extractPeerID extracts the peer ID from paths like /api/debug/peers/{id}/events.
func extractPeerID(path string) string {
	// /api/debug/peers/{id} or /api/debug/peers/{id}/events or /api/debug/peers/{id}/sdp
	parts := strings.Split(strings.TrimPrefix(path, "/api/debug/peers/"), "/")
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
