package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"github.com/oklog/ulid/v2"

	crosstalk "github.com/aleksclark/crosstalk/server"
	"github.com/aleksclark/crosstalk/server/auth"
)

// --- Auth Handlers ---

func (s *Server) handleLogin(ctx context.Context, input *LoginRequest) (*LoginResponse, error) {
	pair, err := s.auth.Login(ctx, input.Body.Username, input.Body.Password)
	if err != nil {
		return nil, huma.Error401Unauthorized("invalid credentials")
	}
	resp := &LoginResponse{}
	resp.Body.AccessToken = pair.AccessToken
	resp.Body.RefreshToken = pair.RefreshToken
	return resp, nil
}

func (s *Server) handleRefresh(ctx context.Context, input *RefreshRequest) (*RefreshResponse, error) {
	pair, err := s.auth.Refresh(ctx, input.Body.RefreshToken)
	if err != nil {
		return nil, huma.Error401Unauthorized("invalid refresh token")
	}
	resp := &RefreshResponse{}
	resp.Body.AccessToken = pair.AccessToken
	resp.Body.RefreshToken = pair.RefreshToken
	return resp, nil
}

func (s *Server) handleLogout(ctx context.Context, input *LogoutRequest) (*LogoutResponse, error) {
	_ = s.auth.Logout(ctx, input.Body.RefreshToken)
	resp := &LogoutResponse{}
	resp.Body.OK = true
	return resp, nil
}

// --- Session Handlers ---

func (s *Server) handleListSessions(ctx context.Context, input *ListSessionsRequest) (*ListSessionsResponse, error) {
	claims, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := s.requireRole(claims, "admin", "translator"); err != nil {
		return nil, err
	}

	q := crosstalk.ListQuery{
		Q:         input.Q,
		Sort:      input.Sort,
		Direction: crosstalk.ListDirection(input.Direction),
		Limit:     input.Limit,
		Cursor:    input.Cursor,
	}
	if claims.Role == "translator" {
		assigned, err := s.assignedSessionIDs(ctx, claims.Subject)
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to resolve assignments")
		}
		q.RestrictToIDs = &assigned
	}

	page, err := s.services.Sessions.ListPage(ctx, q)
	if err != nil {
		if errors.Is(err, crosstalk.ErrInvalidListQuery) {
			return nil, huma.Error400BadRequest(err.Error())
		}
		return nil, huma.Error500InternalServerError("failed to list sessions")
	}

	resp := &ListSessionsResponse{}
	resp.Body.Data = make([]SessionOut, len(page.Items))
	for i, sess := range page.Items {
		resp.Body.Data[i] = sessionToOutForClaims(sess, claims)
	}
	resp.Body.NextCursor = page.NextCursor
	resp.Body.Total = page.Total
	return resp, nil
}

func (s *Server) handleCreateSession(ctx context.Context, input *CreateSessionRequest) (*CreateSessionResponse, error) {
	claims, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := s.requireRole(claims, "admin"); err != nil {
		return nil, err
	}

	sess := &crosstalk.Session{
		ID:          ulid.Make().String(),
		Name:        input.Body.Name,
		Description: input.Body.Description,
	}

	if err := s.services.Sessions.Create(ctx, sess); err != nil {
		return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to create session: %v", err))
	}

	resp := &CreateSessionResponse{}
	resp.Body = sessionToOut(*sess)
	return resp, nil
}

func (s *Server) handleGetSession(ctx context.Context, input *GetSessionRequest) (*GetSessionResponse, error) {
	claims, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := s.requireRole(claims, "admin", "translator"); err != nil {
		return nil, err
	}
	if err := s.authorizeSessionAccess(ctx, claims, input.ID); err != nil {
		return nil, err
	}

	sess, err := s.services.Sessions.Get(ctx, input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("session not found")
	}

	resp := &GetSessionResponse{}
	resp.Body = sessionToOutForClaims(*sess, claims)
	return resp, nil
}

func (s *Server) handleUpdateSession(ctx context.Context, input *UpdateSessionRequest) (*UpdateSessionResponse, error) {
	claims, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := s.requireRole(claims, "admin"); err != nil {
		return nil, err
	}

	sess, err := s.services.Sessions.Get(ctx, input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("session not found")
	}

	sess.Name = input.Body.Name
	sess.Description = input.Body.Description

	if err := s.services.Sessions.Update(ctx, sess); err != nil {
		return nil, huma.Error500InternalServerError("failed to update session")
	}

	resp := &UpdateSessionResponse{}
	resp.Body = sessionToOut(*sess)
	return resp, nil
}

func (s *Server) handleDeleteSession(ctx context.Context, input *DeleteSessionRequest) (*DeleteSessionResponse, error) {
	claims, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := s.requireRole(claims, "admin"); err != nil {
		return nil, err
	}

	if err := s.services.Sessions.Delete(ctx, input.ID); err != nil {
		return nil, huma.Error404NotFound("session not found")
	}

	resp := &DeleteSessionResponse{}
	resp.Body.OK = true
	return resp, nil
}

func (s *Server) handleGetBroadcastURL(ctx context.Context, input *GetBroadcastURLRequest) (*GetBroadcastURLResponse, error) {
	claims, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := s.requireRole(claims, "admin", "translator"); err != nil {
		return nil, err
	}
	if err := s.authorizeSessionAccess(ctx, claims, input.ID); err != nil {
		return nil, err
	}

	sess, err := s.services.Sessions.Get(ctx, input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("session not found")
	}

	resp := &GetBroadcastURLResponse{}
	resp.Body.BroadcastToken = sess.BroadcastToken
	resp.Body.URL = fmt.Sprintf("/broadcast/%s", sess.BroadcastToken)
	return resp, nil
}

func (s *Server) handleRegenerateBroadcastURL(ctx context.Context, input *RegenerateBroadcastURLRequest) (*RegenerateBroadcastURLResponse, error) {
	claims, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := s.requireRole(claims, "admin"); err != nil {
		return nil, err
	}

	// Capture old token so we can identify listeners to drop after rotation.
	var oldToken string
	if sess, gerr := s.services.Sessions.Get(ctx, input.ID); gerr == nil && sess != nil {
		oldToken = sess.BroadcastToken
	}

	token, err := s.services.Sessions.RegenerateBroadcastToken(ctx, input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("session not found")
	}

	// Best-effort: drop active peers for this session so rotated tokens cannot
	// keep listening. Full listener tracking is network-lane territory; when
	// PeerManager is present we remove all peers (conservative fail-closed).
	_ = oldToken
	if s.services.PeerManager != nil {
		for _, p := range s.services.PeerManager.ListPeers() {
			s.services.PeerManager.RemovePeer(p.ID)
		}
	}

	resp := &RegenerateBroadcastURLResponse{}
	resp.Body.BroadcastToken = token
	resp.Body.URL = fmt.Sprintf("/broadcast/%s", token)
	return resp, nil
}

// --- Channel Handlers ---

func (s *Server) handleListChannels(ctx context.Context, input *ListChannelsRequest) (*ListChannelsResponse, error) {
	claims, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := s.requireRole(claims, "admin", "translator"); err != nil {
		return nil, err
	}
	if err := s.authorizeSessionAccess(ctx, claims, input.ID); err != nil {
		return nil, err
	}

	channels, err := s.services.Channels.List(ctx, input.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list channels")
	}

	resp := &ListChannelsResponse{}
	resp.Body.Data = make([]ChannelOut, len(channels))
	for i, ch := range channels {
		resp.Body.Data[i] = channelToOut(ch)
	}
	return resp, nil
}

func (s *Server) handleListSources(ctx context.Context, input *ListSourcesRequest) (*ListSourcesResponse, error) {
	claims, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := s.requireRole(claims, "admin", "translator"); err != nil {
		return nil, err
	}
	if err := s.authorizeSessionAccess(ctx, claims, input.ID); err != nil {
		return nil, err
	}

	sources, err := s.services.Sources.List(ctx, input.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list sources")
	}

	resp := &ListSourcesResponse{}
	resp.Body.Data = make([]SourceOut, len(sources))
	for i, src := range sources {
		resp.Body.Data[i] = sourceToOut(src)
	}
	return resp, nil
}

func (s *Server) handleCreateChannel(ctx context.Context, input *CreateChannelRequest) (*CreateChannelResponse, error) {
	claims, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := s.requireRole(claims, "admin"); err != nil {
		return nil, err
	}

	ch := &crosstalk.Channel{
		ID:        ulid.Make().String(),
		SessionID: input.ID,
		Name:      input.Body.Name,
		Type:      crosstalk.ChannelType(input.Body.Type),
	}

	if err := s.services.Channels.Create(ctx, ch); err != nil {
		return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to create channel: %v", err))
	}

	resp := &CreateChannelResponse{}
	resp.Body = channelToOut(*ch)
	return resp, nil
}

func (s *Server) handleUpdateChannel(ctx context.Context, input *UpdateChannelRequest) (*UpdateChannelResponse, error) {
	claims, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := s.requireRole(claims, "admin"); err != nil {
		return nil, err
	}

	ch, err := s.services.Channels.Get(ctx, input.ChID)
	if err != nil {
		return nil, huma.Error404NotFound("channel not found")
	}

	ch.Name = input.Body.Name
	ch.Type = crosstalk.ChannelType(input.Body.Type)

	if err := s.services.Channels.Update(ctx, ch); err != nil {
		return nil, huma.Error500InternalServerError("failed to update channel")
	}

	resp := &UpdateChannelResponse{}
	resp.Body = channelToOut(*ch)
	return resp, nil
}

func (s *Server) handleDeleteChannel(ctx context.Context, input *DeleteChannelRequest) (*DeleteChannelResponse, error) {
	claims, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := s.requireRole(claims, "admin"); err != nil {
		return nil, err
	}

	if err := s.services.Channels.Delete(ctx, input.ChID); err != nil {
		return nil, huma.Error404NotFound("channel not found")
	}

	resp := &DeleteChannelResponse{}
	resp.Body.OK = true
	return resp, nil
}

// --- Mix Handlers ---

func (s *Server) handleGetMix(ctx context.Context, input *GetMixRequest) (*GetMixResponse, error) {
	claims, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := s.requireRole(claims, "admin", "translator"); err != nil {
		return nil, err
	}
	if _, err := s.authorizeChannelAccess(ctx, claims, input.ID, input.ChID); err != nil {
		return nil, err
	}

	entries, err := s.services.Mix.GetMix(ctx, input.ChID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get mix")
	}

	resp := &GetMixResponse{}
	resp.Body.Data = make([]MixEntryOut, len(entries))
	for i, e := range entries {
		resp.Body.Data[i] = mixEntryToOut(e)
	}
	return resp, nil
}

func (s *Server) handleUpdateMix(ctx context.Context, input *UpdateMixRequest) (*UpdateMixResponse, error) {
	claims, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := s.requireRole(claims, "admin", "translator"); err != nil {
		return nil, err
	}
	if _, err := s.authorizeChannelAccess(ctx, claims, input.ID, input.ChID); err != nil {
		return nil, err
	}

	entries := make([]crosstalk.MixEntry, len(input.Body.Entries))
	for i, e := range input.Body.Entries {
		entries[i] = crosstalk.MixEntry{
			ChannelID: input.ChID,
			SourceID:  e.SourceID,
			Muted:     e.Muted,
			Level:     e.Level,
		}
	}

	if err := s.services.Mix.SetMix(ctx, input.ChID, entries); err != nil {
		return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to update mix: %v", err))
	}

	// Return current state
	current, err := s.services.Mix.GetMix(ctx, input.ChID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get mix after update")
	}

	resp := &UpdateMixResponse{}
	resp.Body.Data = make([]MixEntryOut, len(current))
	for i, e := range current {
		resp.Body.Data[i] = mixEntryToOut(e)
	}
	return resp, nil
}

// --- ABC Handlers ---

func (s *Server) handleListABCs(ctx context.Context, input *ListABCsRequest) (*ListABCsResponse, error) {
	claims, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	// Any authenticated user may list ABCs; translators need this to see and
	// choose booth monitors for their assigned sessions.
	if err := s.requireRole(claims, "admin", "translator"); err != nil {
		return nil, err
	}

	q := crosstalk.ListQuery{
		Q:         input.Q,
		Sort:      input.Sort,
		Direction: crosstalk.ListDirection(input.Direction),
		Limit:     input.Limit,
		Cursor:    input.Cursor,
	}
	if claims.Role == "translator" {
		assigned, err := s.assignedSessionIDs(ctx, claims.Subject)
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to resolve assignments")
		}
		// Scope by session_id before pagination; empty set → empty page.
		q.RestrictToIDs = &assigned
	}

	page, err := s.services.ABCs.ListPage(ctx, q)
	if err != nil {
		if errors.Is(err, crosstalk.ErrInvalidListQuery) {
			return nil, huma.Error400BadRequest(err.Error())
		}
		return nil, huma.Error500InternalServerError("failed to list ABCs")
	}

	resp := &ListABCsResponse{}
	resp.Body.Data = make([]ABCOut, len(page.Items))
	for i, item := range page.Items {
		out := abcToOut(item.ABC)
		out.SessionName = item.SessionName
		resp.Body.Data[i] = out
	}
	resp.Body.NextCursor = page.NextCursor
	resp.Body.Total = page.Total
	return resp, nil
}

func (s *Server) handleCreateABC(ctx context.Context, input *CreateABCRequest) (*CreateABCResponse, error) {
	claims, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := s.requireRole(claims, "admin"); err != nil {
		return nil, err
	}

	// Generate a random token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, huma.Error500InternalServerError("failed to generate token")
	}
	token := hex.EncodeToString(tokenBytes)
	tokenHash := auth.HashToken(token)

	abc := &crosstalk.ABC{
		ID:        ulid.Make().String(),
		Name:      input.Body.Name,
		TokenHash: tokenHash,
	}

	if err := s.services.ABCs.Create(ctx, abc); err != nil {
		return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to create ABC: %v", err))
	}

	resp := &CreateABCResponse{}
	resp.Body.ID = abc.ID
	resp.Body.Name = abc.Name
	resp.Body.Token = token
	return resp, nil
}

func (s *Server) handleGetABC(ctx context.Context, input *GetABCRequest) (*GetABCResponse, error) {
	claims, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := s.requireRole(claims, "admin"); err != nil {
		return nil, err
	}

	abc, err := s.services.ABCs.Get(ctx, input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("ABC not found")
	}

	resp := &GetABCResponse{}
	resp.Body = abcToOut(*abc)
	return resp, nil
}

func (s *Server) handleUpdateABC(ctx context.Context, input *UpdateABCRequest) (*UpdateABCResponse, error) {
	claims, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := s.requireRole(claims, "admin", "translator"); err != nil {
		return nil, err
	}

	abc, err := s.services.ABCs.Get(ctx, input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("ABC not found")
	}

	// Translators may only change the monitor channel, and only for ABCs in a
	// session they are assigned to. Admins may change everything.
	isAdmin := claims.Role == "admin"
	if !isAdmin {
		if abc.SessionID == nil || !s.translatorAssignedTo(ctx, claims.Subject, *abc.SessionID) {
			return nil, huma.Error403Forbidden("not assigned to this ABC's session")
		}
		if input.Body.Name != "" || input.Body.SessionID != nil {
			return nil, huma.Error403Forbidden("translators may only change the monitor channel")
		}
	}

	if input.Body.Name != "" {
		abc.Name = input.Body.Name
	}
	// A nil SessionID means "leave assignment unchanged"; an empty string is an
	// explicit request to unassign the ABC from its session.
	routingChanged := false
	if input.Body.SessionID != nil {
		newSession := input.Body.SessionID
		if *input.Body.SessionID == "" {
			newSession = nil
		}
		if !strPtrEqual(abc.SessionID, newSession) {
			routingChanged = true
		}
		abc.SessionID = newSession
	}
	// Same nil/empty-string semantics for the monitor channel selection.
	if input.Body.MonitorChannelID != nil {
		newMonitor := input.Body.MonitorChannelID
		if *input.Body.MonitorChannelID == "" {
			newMonitor = nil
		}
		if !strPtrEqual(abc.MonitorChannelID, newMonitor) {
			routingChanged = true
		}
		abc.MonitorChannelID = newMonitor
	}

	if err := s.services.ABCs.Update(ctx, abc); err != nil {
		return nil, huma.Error500InternalServerError("failed to update ABC")
	}

	// If the ABC's routing (session or monitor channel) changed, force the
	// board to reconnect so it re-bridges with the new configuration. This is
	// what makes a late assignment (board connected before being assigned)
	// actually produce a source.
	if routingChanged {
		s.reconnectABC(abc.ID)
	}

	resp := &UpdateABCResponse{}
	resp.Body = abcToOut(*abc)
	return resp, nil
}

// strPtrEqual reports whether two *string values point to the same string (or
// are both nil).
func strPtrEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// translatorAssignedTo reports whether the given user is assigned to the
// session. Errors are treated as "not assigned" (fail closed).
func (s *Server) translatorAssignedTo(ctx context.Context, userID, sessionID string) bool {
	if s.services.Users == nil {
		return false
	}
	sessions, err := s.services.Users.GetAssignedSessions(ctx, userID)
	if err != nil {
		return false
	}
	for _, sid := range sessions {
		if sid == sessionID {
			return true
		}
	}
	return false
}

func (s *Server) handleDeleteABC(ctx context.Context, input *DeleteABCRequest) (*DeleteABCResponse, error) {
	claims, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := s.requireRole(claims, "admin"); err != nil {
		return nil, err
	}

	if err := s.services.ABCs.Delete(ctx, input.ID); err != nil {
		return nil, huma.Error404NotFound("ABC not found")
	}

	resp := &DeleteABCResponse{}
	resp.Body.OK = true
	return resp, nil
}

func (s *Server) handleRestartABC(ctx context.Context, input *RestartABCRequest) (*RestartABCResponse, error) {
	claims, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := s.requireRole(claims, "admin"); err != nil {
		return nil, err
	}

	// Verify ABC exists
	if _, err := s.services.ABCs.Get(ctx, input.ID); err != nil {
		return nil, huma.Error404NotFound("ABC not found")
	}

	// TODO: Send restart command via control channel (WebRTC data channel)
	s.log.Info("restart command sent", "abc_id", input.ID)

	resp := &RestartABCResponse{}
	resp.Body.OK = true
	return resp, nil
}

// --- Translator Handlers ---

func (s *Server) handleListTranslators(ctx context.Context, input *ListTranslatorsRequest) (*ListTranslatorsResponse, error) {
	claims, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := s.requireRole(claims, "admin"); err != nil {
		return nil, err
	}

	q := crosstalk.ListQuery{
		Q:         input.Q,
		Sort:      input.Sort,
		Direction: crosstalk.ListDirection(input.Direction),
		Limit:     input.Limit,
		Cursor:    input.Cursor,
	}
	page, err := s.services.Users.ListTranslatorsPage(ctx, q)
	if err != nil {
		if errors.Is(err, crosstalk.ErrInvalidListQuery) {
			return nil, huma.Error400BadRequest(err.Error())
		}
		return nil, huma.Error500InternalServerError("failed to list translators")
	}

	resp := &ListTranslatorsResponse{}
	resp.Body.Data = make([]TranslatorOut, len(page.Items))
	for i, item := range page.Items {
		resp.Body.Data[i] = TranslatorOut{
			ID:           item.ID,
			Username:     item.Username,
			Sessions:     item.SessionIDs,
			SessionNames: item.SessionNames,
			CreatedAt:    item.CreatedAt,
		}
	}
	resp.Body.NextCursor = page.NextCursor
	resp.Body.Total = page.Total
	return resp, nil
}

func (s *Server) handleCreateTranslator(ctx context.Context, input *CreateTranslatorRequest) (*CreateTranslatorResponse, error) {
	claims, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := s.requireRole(claims, "admin"); err != nil {
		return nil, err
	}

	hash, err := auth.HashPassword(input.Body.Password)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to hash password")
	}

	user := &crosstalk.User{
		ID:           ulid.Make().String(),
		Username:     input.Body.Username,
		PasswordHash: hash,
		Role:         "translator",
	}

	if err := s.services.Users.Create(ctx, user); err != nil {
		return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to create translator: %v", err))
	}

	resp := &CreateTranslatorResponse{}
	resp.Body = TranslatorOut{
		ID:        user.ID,
		Username:  user.Username,
		CreatedAt: user.CreatedAt,
	}
	return resp, nil
}

func (s *Server) handleUpdateTranslator(ctx context.Context, input *UpdateTranslatorRequest) (*UpdateTranslatorResponse, error) {
	claims, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := s.requireRole(claims, "admin"); err != nil {
		return nil, err
	}

	user, err := s.services.Users.Get(ctx, input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("translator not found")
	}

	if input.Body.Username != "" {
		user.Username = input.Body.Username
	}
	if input.Body.Password != "" {
		hash, err := auth.HashPassword(input.Body.Password)
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to hash password")
		}
		user.PasswordHash = hash
	}

	if err := s.services.Users.Update(ctx, user); err != nil {
		return nil, huma.Error500InternalServerError("failed to update translator")
	}

	sessions, _ := s.services.Users.GetAssignedSessions(ctx, user.ID)
	resp := &UpdateTranslatorResponse{}
	resp.Body = TranslatorOut{
		ID:        user.ID,
		Username:  user.Username,
		Sessions:  sessions,
		CreatedAt: user.CreatedAt,
	}
	return resp, nil
}

func (s *Server) handleDeleteTranslator(ctx context.Context, input *DeleteTranslatorRequest) (*DeleteTranslatorResponse, error) {
	claims, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := s.requireRole(claims, "admin"); err != nil {
		return nil, err
	}

	if err := s.services.Users.Delete(ctx, input.ID); err != nil {
		return nil, huma.Error404NotFound("translator not found")
	}

	resp := &DeleteTranslatorResponse{}
	resp.Body.OK = true
	return resp, nil
}

func (s *Server) handleAssignTranslatorSessions(ctx context.Context, input *AssignTranslatorSessionsRequest) (*AssignTranslatorSessionsResponse, error) {
	claims, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := s.requireRole(claims, "admin"); err != nil {
		return nil, err
	}

	if err := s.services.Users.AssignSessions(ctx, input.ID, input.Body.SessionIDs); err != nil {
		return nil, huma.Error500InternalServerError("failed to assign sessions")
	}

	resp := &AssignTranslatorSessionsResponse{}
	resp.Body.OK = true
	return resp, nil
}

// --- User Handlers ---

func (s *Server) handleListUsers(ctx context.Context, input *ListUsersRequest) (*ListUsersResponse, error) {
	claims, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := s.requireRole(claims, "admin"); err != nil {
		return nil, err
	}

	users, err := s.services.Users.List(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list users")
	}

	resp := &ListUsersResponse{}
	resp.Body.Data = make([]UserOut, len(users))
	for i, u := range users {
		resp.Body.Data[i] = UserOut{
			ID:        u.ID,
			Username:  u.Username,
			Role:      u.Role,
			CreatedAt: u.CreatedAt,
		}
	}
	return resp, nil
}

func (s *Server) handleCreateUser(ctx context.Context, input *CreateUserRequest) (*CreateUserResponse, error) {
	claims, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := s.requireRole(claims, "admin"); err != nil {
		return nil, err
	}

	hash, err := auth.HashPassword(input.Body.Password)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to hash password")
	}

	user := &crosstalk.User{
		ID:           ulid.Make().String(),
		Username:     input.Body.Username,
		PasswordHash: hash,
		Role:         input.Body.Role,
	}

	if err := s.services.Users.Create(ctx, user); err != nil {
		return nil, huma.Error500InternalServerError(fmt.Sprintf("failed to create user: %v", err))
	}

	resp := &CreateUserResponse{}
	resp.Body = UserOut{
		ID:        user.ID,
		Username:  user.Username,
		Role:      user.Role,
		CreatedAt: user.CreatedAt,
	}
	return resp, nil
}

func (s *Server) handleDeleteUser(ctx context.Context, input *DeleteUserRequest) (*DeleteUserResponse, error) {
	claims, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := s.requireRole(claims, "admin"); err != nil {
		return nil, err
	}

	if err := s.services.Users.Delete(ctx, input.ID); err != nil {
		return nil, huma.Error404NotFound("user not found")
	}

	resp := &DeleteUserResponse{}
	resp.Body.OK = true
	return resp, nil
}

// --- Public Handlers ---

func (s *Server) handleGetBroadcastInfo(ctx context.Context, input *GetBroadcastInfoRequest) (*GetBroadcastInfoResponse, error) {
	sess, err := s.services.Sessions.Get(ctx, input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("session not found")
	}

	resp := &GetBroadcastInfoResponse{}
	resp.Body.SessionID = sess.ID
	resp.Body.SessionName = sess.Name
	resp.Body.Active = true // TODO: track active state
	return resp, nil
}

// --- Helpers ---

func sessionToOut(s crosstalk.Session) SessionOut {
	return SessionOut{
		ID:             s.ID,
		Name:           s.Name,
		Description:    s.Description,
		BroadcastToken: s.BroadcastToken,
		CreatedAt:      s.CreatedAt,
		UpdatedAt:      s.UpdatedAt,
	}
}

func channelToOut(c crosstalk.Channel) ChannelOut {
	return ChannelOut{
		ID:        c.ID,
		SessionID: c.SessionID,
		Name:      c.Name,
		Type:      string(c.Type),
		CreatedAt: c.CreatedAt,
	}
}

func sourceToOut(s crosstalk.Source) SourceOut {
	return SourceOut{
		ID:        s.ID,
		SessionID: s.SessionID,
		Name:      s.Name,
		Origin:    string(s.Origin),
		PeerID:    s.PeerID,
		Connected: s.Connected,
		FirstSeen: s.FirstSeen,
		LastSeen:  s.LastSeen,
	}
}

func mixEntryToOut(e crosstalk.MixEntry) MixEntryOut {
	return MixEntryOut{
		ID:        e.ID,
		ChannelID: e.ChannelID,
		SourceID:  e.SourceID,
		Muted:     e.Muted,
		Level:     e.Level,
	}
}

func abcToOut(a crosstalk.ABC) ABCOut {
	return ABCOut{
		ID:               a.ID,
		Name:             a.Name,
		SessionID:        a.SessionID,
		MonitorChannelID: a.MonitorChannelID,
		Connected:        a.Connected,
		LastSeen:         a.LastSeen,
		CreatedAt:        a.CreatedAt,
	}
}

func recordingToOut(r crosstalk.Recording) RecordingOut {
	return RecordingOut{
		ID:        r.ID,
		SessionID: r.SessionID,
		SourceID:  r.SourceID,
		ChannelID: r.ChannelID,
		FilePath:  r.FilePath,
		StartedAt: r.StartedAt,
		EndedAt:   r.EndedAt,
		SizeBytes: r.SizeBytes,
	}
}

// --- Recording Handlers ---

func (s *Server) handleListRecordings(ctx context.Context, input *ListRecordingsRequest) (*ListRecordingsResponse, error) {
	claims, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := s.requireRole(claims, "admin", "translator"); err != nil {
		return nil, err
	}
	if err := s.authorizeSessionAccess(ctx, claims, input.ID); err != nil {
		return nil, err
	}

	if s.services.Recordings == nil {
		return nil, huma.Error500InternalServerError("recordings service not configured")
	}

	recordings, err := s.services.Recordings.FindBySession(ctx, input.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list recordings")
	}

	resp := &ListRecordingsResponse{}
	resp.Body.Data = make([]RecordingOut, len(recordings))
	for i, r := range recordings {
		resp.Body.Data[i] = recordingToOut(r)
	}
	return resp, nil
}

func (s *Server) handleDownloadRecording(ctx context.Context, input *DownloadRecordingRequest) (*DownloadRecordingResponse, error) {
	claims, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}
	if err := s.requireRole(claims, "admin"); err != nil {
		return nil, err
	}

	if s.services.Recordings == nil {
		return nil, huma.Error500InternalServerError("recordings service not configured")
	}

	rec, err := s.services.Recordings.FindByID(ctx, input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("recording not found")
	}

	resp := &DownloadRecordingResponse{}
	resp.Body.FilePath = rec.FilePath
	return resp, nil
}
