package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/aleksclark/crosstalk/server/sessionrtc"
	"github.com/aleksclark/crosstalk/server/webrtc"
)

// mountWebRTC wires the WebRTC signaling endpoints and the admin debug API onto
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

	s.mountSessionMedia()
}

// mountSessionMedia wires the session-scoped signaling endpoints that carry
// audio through the SFU. It is a no-op unless both a PeerManager and a
// SessionMedia manager are configured.
//
// Two endpoints:
//
//	GET /api/sessions/{id}/ws?token=<jwt>&produce=<names>&listen=<names>
//	    Authenticated participants (translators/admin/ABC). Client sends the
//	    offer. produce/listen are comma-separated channel names; a "type:feed"
//	    or "type:broadcast" selector expands to all channels of that type. When
//	    omitted, routing defaults by the caller's role.
//
//	GET /ws/broadcast/{id}?token=<broadcast_token>
//	    Public receive-only listeners. The token is the session's broadcast
//	    token. The server sends the offer and streams all broadcast channels.
func (s *Server) mountSessionMedia() {
	pm := s.services.PeerManager
	media := s.services.SessionMedia
	if pm == nil || media == nil {
		return
	}

	s.router.Get("/api/sessions/{id}/ws", func(w http.ResponseWriter, r *http.Request) {
		sessionID := chi.URLParam(r, "id")
		q := r.URL.Query()

		role := "translator"
		if s.auth != nil {
			if claims, err := s.auth.ValidateAccessToken(q.Get("token")); err == nil && claims.Role != "" {
				role = claims.Role
			}
		}

		produce := splitCSV(q.Get("produce"))
		listen := splitCSV(q.Get("listen"))
		if len(produce) == 0 && len(listen) == 0 {
			produce, listen = defaultRoutingNames(role)
		}

		handler := &webrtc.SignalingHandler{
			PeerManager:   pm,
			ServerVersion: "3.0.0",
			OnPeer: func(peer *webrtc.PeerConn, req *http.Request) (bool, error) {
				err := media.Bridge(req.Context(), peer, sessionID, sessionrtc.BridgeOpts{
					Role:    role,
					Produce: produce,
					Listen:  listen,
				})
				return false, err // client-offer flow
			},
		}
		handler.ServeHTTP(w, r)
	})

	s.router.Get("/ws/broadcast/{id}", func(w http.ResponseWriter, r *http.Request) {
		sessionID := chi.URLParam(r, "id")
		token := r.URL.Query().Get("token")

		// Validate the broadcast token against the session.
		sess, err := s.services.Sessions.Get(r.Context(), sessionID)
		if err != nil || token == "" || sess.BroadcastToken != token {
			http.Error(w, "invalid broadcast token", http.StatusForbidden)
			return
		}

		handler := &webrtc.SignalingHandler{
			PeerManager:   pm,
			ServerVersion: "3.0.0",
			OnPeer: func(peer *webrtc.PeerConn, req *http.Request) (bool, error) {
				err := media.Bridge(req.Context(), peer, sessionID, sessionrtc.BridgeOpts{
					Role:        "listener",
					Listen:      []string{"type:broadcast"},
					ServerOffer: true,
				})
				return true, err // server-offer flow (receive-only)
			},
		}
		handler.ServeHTTP(w, r)
	})
}

// splitCSV splits a comma-separated query value, trimming blanks.
func splitCSV(v string) []string {
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// defaultRoutingNames returns produce/listen channel selectors for a role when
// a participant does not specify them. Translators listen to feeds and produce
// into broadcasts; ABC sources produce into feeds.
func defaultRoutingNames(role string) (produce, listen []string) {
	switch role {
	case "abc":
		return []string{"type:feed"}, nil
	case "translator", "admin":
		return []string{"type:broadcast"}, []string{"type:feed"}
	default:
		return []string{"type:broadcast"}, []string{"type:feed"}
	}
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
