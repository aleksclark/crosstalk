package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	crosstalk "github.com/aleksclark/crosstalk/server"
	"github.com/aleksclark/crosstalk/server/auth"
)

// authorizeSessionAccess enforces session visibility:
//   - admin: all sessions
//   - translator: only assigned sessions
//
// Fail-closed on assignment lookup errors. Does not leak whether the session
// exists to unauthorized translators (403).
func (s *Server) authorizeSessionAccess(ctx context.Context, claims *auth.Claims, sessionID string) error {
	if claims == nil {
		return huma.Error401Unauthorized("unauthorized")
	}
	if sessionID == "" {
		return huma.Error400BadRequest("session id required")
	}
	switch claims.Role {
	case "admin":
		return nil
	case "translator":
		if !s.translatorAssignedTo(ctx, claims.Subject, sessionID) {
			return huma.Error403Forbidden("not assigned to this session")
		}
		return nil
	default:
		return huma.Error403Forbidden("insufficient permissions")
	}
}

// authorizeChannelAccess verifies the caller may access the session and that
// the channel belongs to it. Returns the channel on success.
func (s *Server) authorizeChannelAccess(ctx context.Context, claims *auth.Claims, sessionID, channelID string) (*crosstalk.Channel, error) {
	if err := s.authorizeSessionAccess(ctx, claims, sessionID); err != nil {
		return nil, err
	}
	if channelID == "" {
		return nil, huma.Error400BadRequest("channel id required")
	}
	if s.services.Channels == nil {
		return nil, huma.Error500InternalServerError("channels service not configured")
	}
	ch, err := s.services.Channels.Get(ctx, channelID)
	if err != nil || ch == nil {
		return nil, huma.Error404NotFound("channel not found")
	}
	if ch.SessionID != sessionID {
		return nil, huma.Error404NotFound("channel not found")
	}
	return ch, nil
}

// filterSessionsForClaims returns sessions the caller is allowed to see.
// Translators only receive assigned sessions (no metadata leak of others).
func (s *Server) filterSessionsForClaims(ctx context.Context, claims *auth.Claims, sessions []crosstalk.Session) []crosstalk.Session {
	if claims == nil {
		return nil
	}
	if claims.Role == "admin" {
		return sessions
	}
	if claims.Role != "translator" || s.services.Users == nil {
		return nil
	}
	assigned, err := s.services.Users.GetAssignedSessions(ctx, claims.Subject)
	if err != nil {
		return nil
	}
	allow := make(map[string]struct{}, len(assigned))
	for _, id := range assigned {
		allow[id] = struct{}{}
	}
	out := make([]crosstalk.Session, 0, len(assigned))
	for _, sess := range sessions {
		if _, ok := allow[sess.ID]; ok {
			out = append(out, sess)
		}
	}
	return out
}

// sessionToOutForClaims maps a session for API responses. Broadcast tokens are
// admin-only on list/get to avoid leaking share URLs; translators use the
// dedicated broadcast-url endpoint after assignment checks.
func sessionToOutForClaims(sess crosstalk.Session, claims *auth.Claims) SessionOut {
	out := sessionToOut(sess)
	if claims == nil || claims.Role != "admin" {
		out.BroadcastToken = ""
	}
	return out
}

// filterABCsForClaims limits ABC list visibility for translators to ABCs
// assigned to one of their sessions (plus unassigned boards so they can be
// selected as monitors only after an admin assigns them — unassigned are
// hidden to avoid leaking booth inventory). Admins see all.
func (s *Server) filterABCsForClaims(ctx context.Context, claims *auth.Claims, abcs []crosstalk.ABC) []crosstalk.ABC {
	if claims == nil {
		return nil
	}
	if claims.Role == "admin" {
		return abcs
	}
	if claims.Role != "translator" || s.services.Users == nil {
		return nil
	}
	assigned, err := s.services.Users.GetAssignedSessions(ctx, claims.Subject)
	if err != nil {
		return nil
	}
	allow := make(map[string]struct{}, len(assigned))
	for _, id := range assigned {
		allow[id] = struct{}{}
	}
	out := make([]crosstalk.ABC, 0, len(abcs))
	for _, abc := range abcs {
		if abc.SessionID == nil {
			continue
		}
		if _, ok := allow[*abc.SessionID]; ok {
			out = append(out, abc)
		}
	}
	return out
}

// resolveChannelIDs maps produce/listen selectors (channel name, channel ID, or
// type:feed / type:broadcast) to concrete channel IDs within a session.
func (s *Server) resolveChannelIDs(ctx context.Context, sessionID string, selectors []string) ([]string, error) {
	if len(selectors) == 0 {
		return []string{}, nil
	}
	if s.services.Channels == nil {
		return nil, fmt.Errorf("channels service not configured")
	}
	channels, err := s.services.Channels.List(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]crosstalk.Channel, len(channels))
	byName := make(map[string]crosstalk.Channel, len(channels))
	var feeds, broadcasts []string
	for _, ch := range channels {
		byID[ch.ID] = ch
		byName[strings.ToLower(ch.Name)] = ch
		switch ch.Type {
		case crosstalk.ChannelFeed:
			feeds = append(feeds, ch.ID)
		case crosstalk.ChannelBroadcast:
			broadcasts = append(broadcasts, ch.ID)
		}
	}

	seen := make(map[string]struct{})
	out := make([]string, 0, len(selectors))
	add := func(id string) {
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}

	for _, raw := range selectors {
		sel := strings.TrimSpace(raw)
		if sel == "" {
			continue
		}
		switch {
		case strings.EqualFold(sel, "type:feed"):
			for _, id := range feeds {
				add(id)
			}
		case strings.EqualFold(sel, "type:broadcast"):
			for _, id := range broadcasts {
				add(id)
			}
		default:
			if ch, ok := byID[sel]; ok {
				add(ch.ID)
				continue
			}
			if ch, ok := byName[strings.ToLower(sel)]; ok {
				add(ch.ID)
				continue
			}
			// Unknown selector is ignored for expansion; callers that need
			// strictness should intersect against allowed sets.
		}
	}
	return out, nil
}

// intersectIDs returns elements of requested that appear in allowed.
// Order follows requested. Empty requested means "no further narrowing" and
// returns a copy of allowed (server-derived capability stands).
func intersectIDs(allowed, requested []string) []string {
	if len(requested) == 0 {
		out := make([]string, len(allowed))
		copy(out, allowed)
		return out
	}
	allow := make(map[string]struct{}, len(allowed))
	for _, id := range allowed {
		allow[id] = struct{}{}
	}
	out := make([]string, 0, len(requested))
	seen := make(map[string]struct{})
	for _, id := range requested {
		if _, ok := allow[id]; !ok {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// defaultCapabilitySelectors returns role-default produce/listen selectors
// (channel type selectors) used when minting media tickets.
func defaultCapabilitySelectors(role string) (produce, listen []string) {
	switch role {
	case "abc":
		return []string{"type:feed"}, nil
	case "listener":
		return nil, []string{"type:broadcast"}
	case "translator":
		return []string{"type:broadcast"}, []string{"type:feed"}
	case "admin":
		return []string{"type:broadcast"}, []string{"type:feed", "type:broadcast"}
	default:
		return nil, nil
	}
}

// channelNamesFromIDs resolves channel IDs to names for Bridge opts (which
// historically accept name/type selectors).
func (s *Server) channelNamesFromIDs(ctx context.Context, sessionID string, ids []string) []string {
	if len(ids) == 0 || s.services.Channels == nil {
		return nil
	}
	channels, err := s.services.Channels.List(ctx, sessionID)
	if err != nil {
		return append([]string(nil), ids...)
	}
	byID := make(map[string]string, len(channels))
	for _, ch := range channels {
		byID[ch.ID] = ch.Name
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if name, ok := byID[id]; ok {
			out = append(out, name)
		} else {
			out = append(out, id)
		}
	}
	return out
}
