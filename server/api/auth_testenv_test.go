package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
	"nhooyr.io/websocket"

	crosstalk "github.com/aleksclark/crosstalk/server"
	"github.com/aleksclark/crosstalk/server/api"
	"github.com/aleksclark/crosstalk/server/auth"
	"github.com/aleksclark/crosstalk/server/mediaticket"
	"github.com/aleksclark/crosstalk/server/ownership"
	"github.com/aleksclark/crosstalk/server/pgtest"
	"github.com/aleksclark/crosstalk/server/postgres"
	"github.com/aleksclark/crosstalk/server/sessionrtc"
	"github.com/aleksclark/crosstalk/server/webrtc"
)

// authEnv is a fully-wired API test server with leases + media tickets + peers.
type authEnv struct {
	ts       *httptest.Server
	auth     *auth.Service
	users    crosstalk.UserService
	sessions crosstalk.SessionService
	channels crosstalk.ChannelService
	sources  crosstalk.SourceService
	abcs     crosstalk.ABCService
	pm       *webrtc.PeerManager
	tickets  *mediaticket.Service
	leases   ownership.Service
	db       *postgres.DB
}

func setupAuthServer(t *testing.T) *authEnv {
	t.Helper()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	db := pgtest.New(t)

	sessionStore := postgres.NewSessionStore(db)
	channelStore := postgres.NewChannelStore(db)
	sourceStore := postgres.NewSourceStore(db)
	mixStore := postgres.NewMixStore(db)
	abcStore := postgres.NewABCStore(db)
	userStore := postgres.NewUserStore(db)
	refreshTokenStore := postgres.NewRefreshTokenStore(db)
	ticketStore := postgres.NewMediaTicketStore(db)
	leases := ownership.NewStore(db.DB)
	tickets := mediaticket.NewService(ticketStore, []byte("test-media-secret"))

	authCfg := auth.Config{
		Secret:          "test-secret",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
	}
	authService := auth.NewService(authCfg, userStore, refreshTokenStore)

	pm := webrtc.NewPeerManager(webrtc.ICEConfig{})
	media := sessionrtc.NewManager(sessionrtc.Stores{
		Channels: channelStore,
		Sources:  sourceStore,
		Mix:      mixStore,
	}, log)

	svc := api.Services{
		Sessions:      sessionStore,
		Channels:      channelStore,
		Sources:       sourceStore,
		Mix:           mixStore,
		ABCs:          abcStore,
		Users:         userStore,
		RefreshTokens: refreshTokenStore,
		Auth:          authService,
		PeerManager:   pm,
		SessionMedia:  media,
		MediaTickets:  tickets,
		Leases:        leases,
		InstanceID:    "test-api",
	}

	cfg := api.Config{Addr: ":0", JWTSecret: "test-secret"}
	srv := api.NewServer(cfg, svc, log)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	return &authEnv{
		ts:       ts,
		auth:     authService,
		users:    userStore,
		sessions: sessionStore,
		channels: channelStore,
		sources:  sourceStore,
		abcs:     abcStore,
		pm:       pm,
		tickets:  tickets,
		leases:   leases,
		db:       db,
	}
}

func (e *authEnv) createUser(t *testing.T, username, password, role string) *crosstalk.User {
	t.Helper()
	hash, err := auth.HashPassword(password)
	require.NoError(t, err)
	u := &crosstalk.User{
		ID:           ulid.Make().String(),
		Username:     username,
		PasswordHash: hash,
		Role:         role,
	}
	require.NoError(t, e.users.Create(context.Background(), u))
	return u
}

func (e *authEnv) login(t *testing.T, username, password string) string {
	t.Helper()
	body := fmt.Sprintf(`{"username":%q,"password":%q}`, username, password)
	resp, err := http.Post(e.ts.URL+"/api/auth/login", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var result struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	require.NotEmpty(t, result.AccessToken)
	return result.AccessToken
}

func (e *authEnv) doJSON(t *testing.T, method, path, token string, body any) (*http.Response, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, e.ts.URL+path, rdr)
	require.NoError(t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	_ = resp.Body.Close()
	return resp, data
}

func (e *authEnv) createSession(t *testing.T, adminToken, name string) (id, broadcastToken string) {
	t.Helper()
	resp, data := e.doJSON(t, http.MethodPost, "/api/sessions", adminToken, map[string]string{
		"name": name,
	})
	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))
	var sess struct {
		ID             string `json:"id"`
		BroadcastToken string `json:"broadcast_token"`
	}
	require.NoError(t, json.Unmarshal(data, &sess))
	return sess.ID, sess.BroadcastToken
}

func (e *authEnv) createChannel(t *testing.T, adminToken, sessionID, name, typ string) string {
	t.Helper()
	resp, data := e.doJSON(t, http.MethodPost, "/api/sessions/"+sessionID+"/channels", adminToken, map[string]string{
		"name": name,
		"type": typ,
	})
	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))
	var ch struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(data, &ch))
	return ch.ID
}

func (e *authEnv) issueTicket(t *testing.T, bearer, sessionID string, produce, listen []string) (token string, body map[string]any) {
	t.Helper()
	payload := map[string]any{"session_id": sessionID}
	if produce != nil {
		payload["produce"] = produce
	}
	if listen != nil {
		payload["listen"] = listen
	}
	resp, data := e.doJSON(t, http.MethodPost, "/api/webrtc/token", bearer, payload)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(data))
	require.NoError(t, json.Unmarshal(data, &body))
	tok, _ := body["token"].(string)
	require.NotEmpty(t, tok)
	return tok, body
}

func (e *authEnv) peerCount() int {
	return e.pm.Count()
}

func (e *authEnv) sourceCount(t *testing.T, sessionID string) int {
	t.Helper()
	srcs, err := e.sources.List(context.Background(), sessionID)
	require.NoError(t, err)
	return len(srcs)
}

func (e *authEnv) dialWS(t *testing.T, path string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(cancel)
	wsURL := "ws" + strings.TrimPrefix(e.ts.URL, "http") + path
	return websocket.Dial(ctx, wsURL, nil)
}


func toStrings(v any) []string {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case []string:
		return append([]string(nil), t...)
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
