package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/aleksclark/crosstalk/server/webrtc"
)

// mountWebRTC wires the WebRTC signaling endpoint and the admin debug API onto
// the router. It is a no-op when no PeerManager is configured (e.g. unit tests
// that only exercise the REST API).
func (s *Server) mountWebRTC() {
	pm := s.services.PeerManager
	if pm == nil {
		return
	}

	// Generic signaling endpoint. Peers (CLI/ABC clients) connect here; each
	// connection registers a peer in the PeerManager, which the debug API and
	// the admin Debug page then reflect as live state.
	signaling := &webrtc.SignalingHandler{
		PeerManager:   pm,
		ServerVersion: "3.0.0",
	}
	s.router.Handle("/ws/signaling", signaling)

	// Debug API — admin-only. Returns live peer state and per-peer event logs.
	dbg := &webrtc.DebugHandler{PeerManager: pm}
	s.router.Route("/api/debug", func(r chi.Router) {
		r.Use(s.requireAdminMiddleware)
		r.Get("/peers", dbg.HandleListPeers)
		r.Get("/peers/{id}", dbg.HandlePeerDetailFromPath)
		r.Get("/peers/{id}/events", dbg.HandlePeerEventsFromPath)
		r.Get("/peers/{id}/sdp", dbg.HandlePeerSDPFromPath)
	})
}

// requireAdminMiddleware rejects requests that lack a valid admin JWT.
func (s *Server) requireAdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		claims, err := s.auth.ValidateAccessToken(parts[1])
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if claims.Role != "admin" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
