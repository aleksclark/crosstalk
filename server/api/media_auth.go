package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	crosstalk "github.com/aleksclark/crosstalk/server"
	"github.com/aleksclark/crosstalk/server/mediaticket"
	"github.com/aleksclark/crosstalk/server/ownership"
)

const defaultLeaseTTL = 5 * time.Minute

// mediaAdmission is the result of validating (and optionally consuming) a
// media credential before peer allocation.
type mediaAdmission struct {
	SessionID         string
	Role              string
	Subject           string
	Identity          string
	Label             string
	ProduceChannelIDs []string
	ListenChannelIDs  []string
	OwnerGeneration   uint64
	// ABC is set when admission is via ABC API token (legacy path without ticket).
	ABC *crosstalk.ABC
	// Ticket is set when a one-time media ticket was consumed.
	Ticket *crosstalk.MediaTicket
}

// handleWebRTCToken issues a one-time, session-scoped media ticket. Channel
// IDs and role are derived server-side from the caller's assignments; client
// produce/listen selectors may only narrow.
func (s *Server) handleWebRTCToken(ctx context.Context, input *WebRTCTokenRequest) (*WebRTCTokenResponse, error) {
	if s.services.MediaTickets == nil {
		return nil, huma.Error503ServiceUnavailable("media ticket service not configured")
	}

	sessionID := strings.TrimSpace(input.Body.SessionID)
	if sessionID == "" {
		return nil, huma.Error400BadRequest("session_id is required")
	}

	// Resolve caller identity: user JWT or ABC API token.
	var (
		subject string
		role    string
		label   string
		abc     *crosstalk.ABC
	)
	if claims, err := s.requireAuth(ctx, input.Authorization); err == nil {
		subject = claims.Subject
		role = claims.Role
		if role != "admin" && role != "translator" {
			return nil, huma.Error403Forbidden("insufficient permissions")
		}
		if err := s.authorizeSessionAccess(ctx, claims, sessionID); err != nil {
			return nil, err
		}
		if s.services.Users != nil {
			if u, uerr := s.services.Users.Get(ctx, subject); uerr == nil && u != nil {
				label = u.Username
			}
		}
	} else {
		abc, err = s.lookupABCFromAuthHeader(ctx, input.Authorization)
		if err != nil || abc == nil {
			return nil, huma.Error401Unauthorized("invalid token")
		}
		if abc.SessionID == nil || *abc.SessionID == "" || *abc.SessionID != sessionID {
			return nil, huma.Error403Forbidden("abc not assigned to this session")
		}
		subject = "abc:" + abc.ID
		role = "abc"
		label = abc.Name
	}

	// Optional client role request cannot elevate.
	if reqRole := strings.TrimSpace(input.Body.Role); reqRole != "" {
		if !roleCompatible(role, reqRole) {
			return nil, huma.Error403Forbidden("requested role not permitted")
		}
		// Listeners are mintable only for dedicated broadcast flow; reject
		// elevating a translator/admin to listener here is fine, but a
		// translator requesting "listener" is a downgrade — allow narrow listen-only.
		if reqRole == "listener" && (role == "translator" || role == "admin") {
			role = "listener"
		}
	}

	// Verify session exists.
	if s.services.Sessions == nil {
		return nil, huma.Error500InternalServerError("sessions service not configured")
	}
	if _, err := s.services.Sessions.Get(ctx, sessionID); err != nil {
		return nil, huma.Error404NotFound("session not found")
	}

	lease, err := s.ensureSessionLease(ctx, sessionID)
	if err != nil {
		return nil, huma.Error500InternalServerError(fmt.Sprintf("session lease: %v", err))
	}

	defProduce, defListen := defaultCapabilitySelectors(role)
	// ABC listen capability comes only from persisted monitor assignment.
	if role == "abc" && abc != nil {
		defProduce = []string{"type:feed"}
		defListen = s.abcMonitorSelectors(ctx, abc)
	}

	allowedProduce, err := s.resolveChannelIDs(ctx, sessionID, defProduce)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to resolve produce channels")
	}
	allowedListen, err := s.resolveChannelIDs(ctx, sessionID, defListen)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to resolve listen channels")
	}

	// Client selectors only narrow.
	reqProduce, err := s.resolveChannelIDs(ctx, sessionID, input.Body.Produce)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to resolve requested produce channels")
	}
	reqListen, err := s.resolveChannelIDs(ctx, sessionID, input.Body.Listen)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to resolve requested listen channels")
	}
	produceIDs := intersectIDs(allowedProduce, reqProduce)
	listenIDs := intersectIDs(allowedListen, reqListen)

	// Listener must not produce.
	if role == "listener" {
		produceIDs = []string{}
	}

	issued, err := s.services.MediaTickets.Issue(ctx, mediaticket.IssueRequest{
		SessionID:         sessionID,
		OwnerID:           lease.OwnerID,
		OwnerGeneration:   lease.Generation,
		Subject:           subject,
		Role:              role,
		ProduceChannelIDs: produceIDs,
		ListenChannelIDs:  listenIDs,
		TTL:               mediaticket.MaxTTL,
	})
	if err != nil {
		return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to issue media ticket: %v", err))
	}

	_ = label // reserved for future response metadata

	resp := &WebRTCTokenResponse{}
	resp.Body.Token = issued.Token
	resp.Body.ExpiresAt = issued.Ticket.ExpiresAt
	resp.Body.SessionID = sessionID
	resp.Body.Role = role
	resp.Body.ProduceChannelIDs = produceIDs
	if resp.Body.ProduceChannelIDs == nil {
		resp.Body.ProduceChannelIDs = []string{}
	}
	resp.Body.ListenChannelIDs = listenIDs
	if resp.Body.ListenChannelIDs == nil {
		resp.Body.ListenChannelIDs = []string{}
	}
	resp.Body.OwnerGeneration = lease.Generation
	return resp, nil
}

func roleCompatible(actual, requested string) bool {
	if actual == requested {
		return true
	}
	// Admin may act as translator/listener for media.
	if actual == "admin" {
		return requested == "translator" || requested == "listener" || requested == "admin"
	}
	// Translator may only stay translator or downgrade to listener.
	if actual == "translator" {
		return requested == "translator" || requested == "listener"
	}
	// ABC may only be abc.
	if actual == "abc" {
		return requested == "abc"
	}
	return false
}

// ensureSessionLease returns the current unexpired lease, acquiring one for
// this instance when none is held.
func (s *Server) ensureSessionLease(ctx context.Context, sessionID string) (ownership.Lease, error) {
	if s.services.Leases == nil {
		// Fall back to session row owner fields when lease service is absent
		// (should not happen in production wiring).
		sess, err := s.services.Sessions.Get(ctx, sessionID)
		if err != nil {
			return ownership.Lease{}, err
		}
		ownerID := sess.OwnerID
		if ownerID == "" {
			ownerID = s.instanceID()
		}
		return ownership.Lease{
			SessionID:  sessionID,
			OwnerID:    ownerID,
			Generation: sess.OwnerGeneration,
		}, nil
	}
	cur, err := s.services.Leases.Current(ctx, sessionID)
	if err != nil {
		return ownership.Lease{}, err
	}
	if cur.OwnerID != "" && !cur.ExpiresAt.IsZero() {
		return cur, nil
	}
	return s.services.Leases.Acquire(ctx, sessionID, s.instanceID(), defaultLeaseTTL)
}

func (s *Server) instanceID() string {
	if s.services.InstanceID != "" {
		return s.services.InstanceID
	}
	return "api"
}

// lookupABCFromAuthHeader validates a Bearer ABC API token.
func (s *Server) lookupABCFromAuthHeader(ctx context.Context, authHeader string) (*crosstalk.ABC, error) {
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return nil, errors.New("invalid authorization header")
	}
	abc := s.lookupABC(ctx, parts[1])
	if abc == nil {
		return nil, errors.New("unknown abc token")
	}
	return abc, nil
}

// admitSessionWS validates credentials for /api/sessions/{id}/ws BEFORE any
// peer is allocated. Prefer a one-time media ticket; fall back to JWT/ABC only
// when MediaTickets is not configured (tests without ticket wiring).
func (s *Server) admitSessionWS(w http.ResponseWriter, r *http.Request, sessionID string) (*mediaAdmission, bool) {
	q := r.URL.Query()
	token := q.Get("token")
	if token == "" {
		// Also accept Authorization header for non-browser clients.
		if h := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(h), "bearer ") {
			token = strings.TrimSpace(h[7:])
		}
	}
	if token == "" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return nil, false
	}

	// Prefer media-ticket consumption when the service is wired AND the
	// credential looks like a media ticket (signed media JWT or opaque nonce).
	// User access JWTs also have three segments — do not treat every JWT as a
	// media ticket or translators would never fall through to assignment checks.
	if s.services.MediaTickets != nil && s.looksLikeMediaTicket(token) {
		adm, err := s.consumeMediaTicket(r.Context(), sessionID, token)
		if err == nil {
			// Apply optional query narrowing (intersect only).
			adm.ProduceChannelIDs = s.narrowFromQuery(r.Context(), sessionID, adm.ProduceChannelIDs, q.Get("produce"), q.Has("produce"))
			adm.ListenChannelIDs = s.narrowFromQuery(r.Context(), sessionID, adm.ListenChannelIDs, q.Get("listen"), q.Has("listen"))
			if adm.Role == "listener" {
				adm.ProduceChannelIDs = nil
			}
			return adm, true
		}
		// Media ticket credential failed → hard fail (no JWT fallback that
		// would re-open fail-open paths).
		status := http.StatusUnauthorized
		if errors.Is(err, errTicketSessionMismatch) {
			status = http.StatusForbidden
		}
		http.Error(w, err.Error(), status)
		return nil, false
	}

	// JWT path (access token). Still enforce assignment; do not trust produce/listen.
	if s.auth != nil {
		if claims, err := s.auth.ValidateAccessToken(token); err == nil {
			if err := s.authorizeSessionAccess(r.Context(), claims, sessionID); err != nil {
				http.Error(w, "forbidden", http.StatusForbidden)
				return nil, false
			}
			role := claims.Role
			if role == "" {
				role = "translator"
			}
			defP, defL := defaultCapabilitySelectors(role)
			prod, _ := s.resolveChannelIDs(r.Context(), sessionID, defP)
			listen, _ := s.resolveChannelIDs(r.Context(), sessionID, defL)
			prod = s.narrowFromQuery(r.Context(), sessionID, prod, q.Get("produce"), q.Has("produce"))
			listen = s.narrowFromQuery(r.Context(), sessionID, listen, q.Get("listen"), q.Has("listen"))
			label := ""
			if s.services.Users != nil {
				if u, uerr := s.services.Users.Get(r.Context(), claims.Subject); uerr == nil && u != nil {
					label = u.Username
				}
			}
			return &mediaAdmission{
				SessionID:         sessionID,
				Role:              role,
				Subject:           claims.Subject,
				Identity:          claims.Subject,
				Label:             label,
				ProduceChannelIDs: prod,
				ListenChannelIDs:  listen,
			}, true
		}
	}

	// ABC API token path for session WS (unusual; boards use /ws/signaling).
	if abc := s.lookupABC(r.Context(), token); abc != nil {
		if abc.SessionID == nil || *abc.SessionID != sessionID {
			http.Error(w, "abc not assigned to this session", http.StatusForbidden)
			return nil, false
		}
		prod, _ := s.resolveChannelIDs(r.Context(), sessionID, []string{"type:feed"})
		listenSel := s.abcMonitorSelectors(r.Context(), abc)
		listen, _ := s.resolveChannelIDs(r.Context(), sessionID, listenSel)
		return &mediaAdmission{
			SessionID:         sessionID,
			Role:              "abc",
			Subject:           "abc:" + abc.ID,
			Identity:          "abc:" + abc.ID,
			Label:             abc.Name,
			ProduceChannelIDs: prod,
			ListenChannelIDs:  listen,
			ABC:               abc,
		}, true
	}

	http.Error(w, "unauthorized", http.StatusUnauthorized)
	return nil, false
}

var errTicketSessionMismatch = errors.New("ticket session mismatch")

func (s *Server) consumeMediaTicket(ctx context.Context, sessionID, token string) (*mediaAdmission, error) {
	lease, err := s.currentLeaseGeneration(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	// Validate JWT claims before consume when signed.
	var claims *mediaticket.Claims
	if c, perr := s.services.MediaTickets.ParseToken(token); perr == nil {
		claims = c
		if claims.SessionID != sessionID {
			return nil, errTicketSessionMismatch
		}
		if claims.OwnerGeneration != lease.Generation {
			return nil, crosstalk.ErrStaleGeneration
		}
	}

	ticket, err := s.services.MediaTickets.Consume(ctx, mediaticket.ConsumeRequest{
		Token:           token,
		Nonce:           token, // opaque nonce path when not a JWT
		OwnerGeneration: lease.Generation,
	})
	if err != nil {
		return nil, err
	}
	if ticket.SessionID != sessionID {
		return nil, errTicketSessionMismatch
	}

	role := ticket.Role
	if claims != nil && claims.Role != "" {
		role = claims.Role
	}
	subject := ticket.Subject
	label := ""
	if strings.HasPrefix(subject, "abc:") {
		// ok
	} else if s.services.Users != nil {
		if u, uerr := s.services.Users.Get(ctx, subject); uerr == nil && u != nil {
			label = u.Username
		}
	}

	return &mediaAdmission{
		SessionID:         sessionID,
		Role:              role,
		Subject:           subject,
		Identity:          subject,
		Label:             label,
		ProduceChannelIDs: append([]string(nil), ticket.ProduceChannelIDs...),
		ListenChannelIDs:  append([]string(nil), ticket.ListenChannelIDs...),
		OwnerGeneration:   ticket.OwnerGeneration,
		Ticket:            ticket,
	}, nil
}

func (s *Server) currentLeaseGeneration(ctx context.Context, sessionID string) (ownership.Lease, error) {
	if s.services.Leases != nil {
		return s.services.Leases.Current(ctx, sessionID)
	}
	sess, err := s.services.Sessions.Get(ctx, sessionID)
	if err != nil {
		return ownership.Lease{}, err
	}
	return ownership.Lease{
		SessionID:  sessionID,
		OwnerID:    sess.OwnerID,
		Generation: sess.OwnerGeneration,
	}, nil
}

func (s *Server) narrowFromQuery(ctx context.Context, sessionID string, allowed []string, raw string, present bool) []string {
	if !present {
		return allowed
	}
	requested, err := s.resolveChannelIDs(ctx, sessionID, splitCSV(raw))
	if err != nil {
		return nil
	}
	// Empty present produce/listen means "none" (explicit clear), not default.
	if raw == "" {
		return nil
	}
	return intersectIDs(allowed, requested)
}

// looksLikeMediaTicket reports whether token is a media ticket (JWT with media
// audience that parses with the media signing key, or opaque hex nonce) rather
// than a user access JWT.
func (s *Server) looksLikeMediaTicket(token string) bool {
	if s.services.MediaTickets == nil || token == "" {
		return false
	}
	if _, err := s.services.MediaTickets.ParseToken(token); err == nil {
		return true
	}
	// Opaque nonces from mediaticket are 32 hex chars. Access JWTs contain '.'.
	if strings.Contains(token, ".") {
		return false
	}
	if len(token) == 32 {
		for _, c := range token {
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
				return false
			}
		}
		return true
	}
	return false
}

// admitSignalingWS validates /ws/signaling credentials before peer allocation.
// ABC boards present their long-lived API token; unknown tokens are rejected.
func (s *Server) admitSignalingWS(w http.ResponseWriter, r *http.Request) (*mediaAdmission, bool) {
	token := r.URL.Query().Get("token")
	if token == "" {
		if h := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(h), "bearer ") {
			token = strings.TrimSpace(h[7:])
		}
	}
	if token == "" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return nil, false
	}

	// Media ticket for a session (optional advanced path).
	if s.services.MediaTickets != nil && s.looksLikeMediaTicket(token) {
		// Session must be in ticket claims; without path session id, parse first.
		if claims, err := s.services.MediaTickets.ParseToken(token); err == nil && claims.SessionID != "" {
			adm, err := s.consumeMediaTicket(r.Context(), claims.SessionID, token)
			if err != nil {
				http.Error(w, err.Error(), http.StatusUnauthorized)
				return nil, false
			}
			return adm, true
		}
	}

	abc := s.lookupABC(r.Context(), token)
	if abc == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return nil, false
	}

	adm := &mediaAdmission{
		Role:     "abc",
		Subject:  "abc:" + abc.ID,
		Identity: "abc:" + abc.ID,
		Label:    abc.Name,
		ABC:      abc,
	}
	if abc.SessionID != nil {
		adm.SessionID = *abc.SessionID
		prod, _ := s.resolveChannelIDs(r.Context(), adm.SessionID, []string{"type:feed"})
		listenSel := s.abcMonitorSelectors(r.Context(), abc)
		listen, _ := s.resolveChannelIDs(r.Context(), adm.SessionID, listenSel)
		adm.ProduceChannelIDs = prod
		adm.ListenChannelIDs = listen
	}
	return adm, true
}

// admitBroadcastWS validates the public broadcast token before peer allocation.
func (s *Server) admitBroadcastWS(w http.ResponseWriter, r *http.Request, sessionID string) (*mediaAdmission, bool) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "invalid broadcast token", http.StatusForbidden)
		return nil, false
	}
	if s.services.Sessions == nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return nil, false
	}
	sess, err := s.services.Sessions.Get(r.Context(), sessionID)
	if err != nil || sess.BroadcastToken == "" || sess.BroadcastToken != token {
		http.Error(w, "invalid broadcast token", http.StatusForbidden)
		return nil, false
	}
	listen, _ := s.resolveChannelIDs(r.Context(), sessionID, []string{"type:broadcast"})
	return &mediaAdmission{
		SessionID:        sessionID,
		Role:             "listener",
		Subject:          "listener",
		ListenChannelIDs: listen,
	}, true
}
