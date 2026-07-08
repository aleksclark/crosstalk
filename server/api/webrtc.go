package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	pionwebrtc "github.com/pion/webrtc/v4"

	crosstalk "github.com/aleksclark/crosstalk/server"
	"github.com/aleksclark/crosstalk/server/auth"
	"github.com/aleksclark/crosstalk/server/sessionrtc"
	"github.com/aleksclark/crosstalk/server/webrtc"
)

// lookupABC resolves a raw API token to its ABC record, or nil if the token is
// empty or unknown.
func (s *Server) lookupABC(ctx context.Context, token string) *crosstalk.ABC {
	if token == "" || s.services.ABCs == nil {
		return nil
	}
	abc, err := s.services.ABCs.GetByTokenHash(ctx, auth.HashToken(token))
	if err != nil {
		return nil
	}
	return abc
}

// setABCConnected persists an ABC's live connection state. It re-reads the ABC
// first so a stale in-memory copy (e.g. captured at connect time, before a
// later session/monitor assignment) cannot clobber the current routing fields
// when only the connection flag is being toggled.
func (s *Server) setABCConnected(ctx context.Context, abc *crosstalk.ABC, connected bool) {
	if abc == nil || s.services.ABCs == nil {
		return
	}
	current, err := s.services.ABCs.Get(ctx, abc.ID)
	if err != nil {
		// The ABC may have been deleted; nothing to update.
		return
	}
	current.Connected = connected
	if connected {
		now := time.Now().UTC()
		current.LastSeen = &now
	}
	if err := s.services.ABCs.Update(ctx, current); err != nil {
		s.log.Warn("failed to update abc connection state", "abc", abc.ID, "connected", connected, "error", err)
	}
}

// registerABCPeer records the live signaling peer for an ABC.
func (s *Server) registerABCPeer(abcID, peerID string) {
	s.abcPeersMu.Lock()
	s.abcPeers[abcID] = peerID
	s.abcPeersMu.Unlock()
}

// deregisterABCPeer clears the live peer mapping for an ABC, but only if it
// still points at peerID (a newer connection may have already replaced it).
func (s *Server) deregisterABCPeer(abcID, peerID string) {
	s.abcPeersMu.Lock()
	if s.abcPeers[abcID] == peerID {
		delete(s.abcPeers, abcID)
	}
	s.abcPeersMu.Unlock()
}

// reconnectABC closes an ABC's live signaling peer, if any, so the
// auto-reconnecting board re-establishes its connection and re-bridges with its
// current session/monitor assignment. No-op when the ABC is not connected.
func (s *Server) reconnectABC(abcID string) {
	s.abcPeersMu.Lock()
	peerID := s.abcPeers[abcID]
	s.abcPeersMu.Unlock()
	if peerID == "" || s.services.PeerManager == nil {
		return
	}
	s.log.Info("reconnecting abc after assignment change", "abc", abcID, "peer", peerID)
	s.services.PeerManager.RemovePeer(peerID)
}

// abcMonitorSelectors returns the Listen channel selectors for an ABC. When a
// specific monitor channel is set (and still exists), the ABC listens only to
// that channel by name. When no monitor channel is set, the ABC listens to
// nothing ("None") — it purely produces its mic into the session. This avoids a
// default self-loopback and keeps the booth's single audio m-line free to
// reliably carry its mic upstream.
func (s *Server) abcMonitorSelectors(ctx context.Context, abc *crosstalk.ABC) []string {
	if abc == nil || abc.MonitorChannelID == nil || *abc.MonitorChannelID == "" {
		return nil
	}
	if s.services.Channels != nil {
		if ch, err := s.services.Channels.Get(ctx, *abc.MonitorChannelID); err == nil && ch != nil {
			return []string{ch.Name}
		}
	}
	return nil
}

// mountWebRTC wires the WebRTC signaling endpoints and the admin debug API onto
// the router. It is a no-op when no PeerManager is configured (e.g. unit tests
// that only exercise the REST API).
func (s *Server) mountWebRTC() {
	pm := s.services.PeerManager
	if pm == nil {
		return
	}

	// Generic signaling endpoint. Headless clients (ABC boards) connect here
	// with their API token. Each connection registers a peer in the
	// PeerManager (visible on the admin Debug page). When the token belongs to
	// an ABC assigned to a session, the peer is bridged into that session as an
	// "abc" producer so its microphone reaches the session's feed channel.
	s.router.Get("/ws/signaling", func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		abc := s.lookupABC(r.Context(), token)

		handler := &webrtc.SignalingHandler{
			PeerManager:   pm,
			ServerVersion: "3.0.0",
			OnPeer: func(peer *webrtc.PeerConn, req *http.Request) (bool, error) {
				assigned := ""
				if abc != nil && abc.SessionID != nil {
					assigned = *abc.SessionID
				}

				// Reflect the ABC's live connection state in the store so the
				// admin UI shows the board as online. Headless boards connect
				// on this websocket path (not the HTTP Bearer path), so mark
				// connected here and clear it when the peer closes. Track the
				// live peer so an assignment/monitor change can force a
				// reconnect (the board auto-reconnects and re-bridges).
				if abc != nil {
					s.setABCConnected(req.Context(), abc, true)
					s.registerABCPeer(abc.ID, peer.ID)
					peer.OnClose(func() {
						s.setABCConnected(context.Background(), abc, false)
						s.deregisterABCPeer(abc.ID, peer.ID)
					})
				}

				// Install the control-protocol handler once the client opens
				// its "control" data channel (the server adopts, not creates).
				peer.OnControlChannel(func(*pionwebrtc.DataChannel) {
					ctrl := &webrtc.ControlHandler{
						Peer:              peer,
						ServerVersion:     "3.0.0",
						AssignedSessionID: assigned,
					}
					ctrl.Install()
				})

				// Bridge an assigned ABC as a producer into its session's feed.
				// It listens to its configured monitor channel for booth return
				// audio, falling back to all broadcast channels when unset.
				// The ABC ID is a stable identity so reconnects reuse one source.
				if s.services.SessionMedia != nil && assigned != "" {
					return false, s.services.SessionMedia.Bridge(req.Context(), peer, assigned, sessionrtc.BridgeOpts{
						Role:     "abc",
						Identity: "abc:" + abc.ID,
						Label:    abc.Name,
						Produce:  []string{"type:feed"},
						Listen:   s.abcMonitorSelectors(req.Context(), abc),
					})
				}
				return false, nil
			},
		}
		handler.ServeHTTP(w, r)
	})

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
		identity := ""
		label := ""
		if s.auth != nil {
			if claims, err := s.auth.ValidateAccessToken(q.Get("token")); err == nil {
				if claims.Role != "" {
					role = claims.Role
				}
				// The JWT subject (user ID) is a stable identity across
				// reconnects, so the same user reuses one Source instead of
				// spawning a new one on every connect.
				identity = claims.Subject
				if identity != "" && s.services.Users != nil {
					if u, uerr := s.services.Users.Get(r.Context(), identity); uerr == nil && u.Username != "" {
						label = u.Username
					}
				}
			}
		}

		// Routing defaults apply per direction based on parameter *presence*.
		// An absent produce/listen param falls back to the role default; a
		// param that is present (even if empty) is taken literally. This lets
		// the monitor feature override only the listen side (?listen=<ch>)
		// while the translator keeps producing into its default broadcast, and
		// still lets deep links fully control both directions (e.g.
		// ?produce=<feed>&listen= to produce-only into a feed).
		defProduce, defListen := defaultRoutingNames(role)
		produce := defProduce
		if q.Has("produce") {
			produce = splitCSV(q.Get("produce"))
		}
		listen := defListen
		if q.Has("listen") {
			listen = splitCSV(q.Get("listen"))
		}

		handler := &webrtc.SignalingHandler{
			PeerManager:   pm,
			ServerVersion: "3.0.0",
			OnPeer: func(peer *webrtc.PeerConn, req *http.Request) (bool, error) {
				err := media.Bridge(req.Context(), peer, sessionID, sessionrtc.BridgeOpts{
					Role:     role,
					Identity: identity,
					Label:    label,
					Produce:  produce,
					Listen:   listen,
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
