package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

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

	sessions, err := s.services.Sessions.List(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list sessions")
	}

	resp := &ListSessionsResponse{}
	resp.Body.Data = make([]SessionOut, len(sessions))
	for i, sess := range sessions {
		resp.Body.Data[i] = sessionToOut(sess)
	}
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

	sess, err := s.services.Sessions.Get(ctx, input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("session not found")
	}

	resp := &GetSessionResponse{}
	resp.Body = sessionToOut(*sess)
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

	token, err := s.services.Sessions.RegenerateBroadcastToken(ctx, input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("session not found")
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
	if err := s.requireRole(claims, "admin"); err != nil {
		return nil, err
	}

	abcs, err := s.services.ABCs.List(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list ABCs")
	}

	resp := &ListABCsResponse{}
	resp.Body.Data = make([]ABCOut, len(abcs))
	for i, abc := range abcs {
		resp.Body.Data[i] = abcToOut(abc)
	}
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
	if err := s.requireRole(claims, "admin"); err != nil {
		return nil, err
	}

	abc, err := s.services.ABCs.Get(ctx, input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("ABC not found")
	}

	if input.Body.Name != "" {
		abc.Name = input.Body.Name
	}
	if input.Body.SessionID != nil {
		abc.SessionID = input.Body.SessionID
	}

	if err := s.services.ABCs.Update(ctx, abc); err != nil {
		return nil, huma.Error500InternalServerError("failed to update ABC")
	}

	resp := &UpdateABCResponse{}
	resp.Body = abcToOut(*abc)
	return resp, nil
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

	users, err := s.services.Users.ListByRole(ctx, "translator")
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list translators")
	}

	resp := &ListTranslatorsResponse{}
	resp.Body.Data = make([]TranslatorOut, len(users))
	for i, u := range users {
		sessions, _ := s.services.Users.GetAssignedSessions(ctx, u.ID)
		resp.Body.Data[i] = TranslatorOut{
			ID:        u.ID,
			Username:  u.Username,
			Sessions:  sessions,
			CreatedAt: u.CreatedAt,
		}
	}
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

// --- WebRTC Handlers ---

func (s *Server) handleWebRTCToken(ctx context.Context, input *WebRTCTokenRequest) (*WebRTCTokenResponse, error) {
	_, err := s.requireAuth(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	// Generate a short-lived token for WebRTC signaling
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, huma.Error500InternalServerError("failed to generate token")
	}

	resp := &WebRTCTokenResponse{}
	resp.Body.Token = hex.EncodeToString(tokenBytes)
	resp.Body.ExpiresAt = time.Now().Add(30 * time.Second)
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
		ID:        a.ID,
		Name:      a.Name,
		SessionID: a.SessionID,
		Connected: a.Connected,
		LastSeen:  a.LastSeen,
		CreatedAt: a.CreatedAt,
	}
}
