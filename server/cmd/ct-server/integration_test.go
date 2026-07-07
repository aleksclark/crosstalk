package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	crosstalk "github.com/aleksclark/crosstalk/server"
	"github.com/aleksclark/crosstalk/server/api"
	"github.com/aleksclark/crosstalk/server/auth"
	"github.com/aleksclark/crosstalk/server/pgtest"
	"github.com/aleksclark/crosstalk/server/postgres"
	"github.com/aleksclark/crosstalk/server/sessionrtc"
	"github.com/aleksclark/crosstalk/server/webrtc"

	"github.com/pion/ice/v4"
	pionwebrtc "github.com/pion/webrtc/v4"
)

// testEnv holds a fully wired integration test environment.
type testEnv struct {
	server  *httptest.Server
	db      *postgres.DB
	users   crosstalk.UserService
	abcs    crosstalk.ABCService
	sources crosstalk.SourceService
}

func setupIntegrationServer(t *testing.T) *testEnv {
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
	recordingStore := postgres.NewRecordingStore(db)

	authCfg := auth.Config{
		Secret:          "integration-test-secret",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
	}
	authService := auth.NewService(authCfg, userStore, refreshTokenStore)

	// WebRTC peer manager with mDNS disabled so localhost ICE candidates are
	// plain host IPs (no .local names) — required for reliable in-test peers.
	var se pionwebrtc.SettingEngine
	se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	peerManager := webrtc.NewPeerManagerWithAPI(pionwebrtc.NewAPI(pionwebrtc.WithSettingEngine(se)))

	sessionMedia := sessionrtc.NewManager(sessionrtc.Stores{
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
		Recordings:    recordingStore,
		Auth:          authService,
		PeerManager:   peerManager,
		SessionMedia:  sessionMedia,
	}

	cfg := api.Config{Addr: ":0", JWTSecret: "integration-test-secret"}
	srv := api.NewServer(cfg, svc, log)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	// Drain any live peers (flushing their source-disconnect DB writes) before
	// pgtest drops the database. Registered after pgtest.New so it runs first
	// (t.Cleanup is LIFO), while the DB is still open.
	t.Cleanup(func() {
		for _, p := range peerManager.ListPeers() {
			peerManager.RemovePeer(p.ID)
		}
		time.Sleep(50 * time.Millisecond)
	})

	return &testEnv{
		server:  ts,
		db:      db,
		users:   userStore,
		abcs:    abcStore,
		sources: sourceStore,
	}
}

// createAdminUser creates an admin user directly in the store and returns credentials.
func (e *testEnv) createAdminUser(t *testing.T, username, password string) {
	t.Helper()
	hash, err := auth.HashPassword(password)
	require.NoError(t, err)
	admin := &crosstalk.User{
		ID:           ulid.Make().String(),
		Username:     username,
		PasswordHash: hash,
		Role:         "admin",
	}
	require.NoError(t, e.users.Create(context.Background(), admin))
}

// login logs in and returns the access token.
func (e *testEnv) login(t *testing.T, username, password string) string {
	t.Helper()
	body := fmt.Sprintf(`{"username":%q,"password":%q}`, username, password)
	resp, err := http.Post(e.server.URL+"/api/auth/login", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "login failed for %s", username)

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	require.NotEmpty(t, result.AccessToken)
	return result.AccessToken
}

// doRequest performs an authenticated HTTP request and returns the response.
func (e *testEnv) doRequest(t *testing.T, method, path, token string, body string) *http.Response {
	t.Helper()
	var bodyReader *strings.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	} else {
		bodyReader = strings.NewReader("")
	}
	req, err := http.NewRequest(method, e.server.URL+path, bodyReader)
	require.NoError(t, err)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// TestIntegrationFullSessionLifecycle tests creating a user, logging in,
// creating a session with channels, and verifying the full response chain.
func TestIntegrationFullSessionLifecycle(t *testing.T) {
	env := setupIntegrationServer(t)

	// 1. Create admin user and login
	env.createAdminUser(t, "admin", "admin-pass-123")
	token := env.login(t, "admin", "admin-pass-123")

	// 2. Create a session
	resp := env.doRequest(t, http.MethodPost, "/api/sessions", token,
		`{"name":"Sunday Service","description":"Weekly gathering"}`)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var session struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Description    string `json:"description"`
		BroadcastToken string `json:"broadcast_token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&session))
	assert.Equal(t, "Sunday Service", session.Name)
	assert.Equal(t, "Weekly gathering", session.Description)
	assert.NotEmpty(t, session.ID)
	assert.NotEmpty(t, session.BroadcastToken)

	// 3. Create channels within the session
	resp = env.doRequest(t, http.MethodPost, "/api/sessions/"+session.ID+"/channels", token,
		`{"name":"Floor Feed","type":"feed"}`)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var feedChannel struct {
		ID        string `json:"id"`
		SessionID string `json:"session_id"`
		Name      string `json:"name"`
		Type      string `json:"type"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&feedChannel))
	assert.Equal(t, "Floor Feed", feedChannel.Name)
	assert.Equal(t, "feed", feedChannel.Type)
	assert.Equal(t, session.ID, feedChannel.SessionID)

	resp = env.doRequest(t, http.MethodPost, "/api/sessions/"+session.ID+"/channels", token,
		`{"name":"English Broadcast","type":"broadcast"}`)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var broadcastChannel struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&broadcastChannel))
	assert.Equal(t, "English Broadcast", broadcastChannel.Name)
	assert.Equal(t, "broadcast", broadcastChannel.Type)

	// 4. List channels and verify both exist
	resp = env.doRequest(t, http.MethodGet, "/api/sessions/"+session.ID+"/channels", token, "")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var channelList struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&channelList))
	assert.Len(t, channelList.Data, 2)

	// 5. Get the session directly
	resp = env.doRequest(t, http.MethodGet, "/api/sessions/"+session.ID, token, "")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var getSession struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&getSession))
	assert.Equal(t, session.ID, getSession.ID)
	assert.Equal(t, "Sunday Service", getSession.Name)

	// 6. Update session
	resp = env.doRequest(t, http.MethodPut, "/api/sessions/"+session.ID, token,
		`{"name":"Updated Service","description":"Updated desc"}`)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var updatedSession struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&updatedSession))
	assert.Equal(t, "Updated Service", updatedSession.Name)

	// 7. List sessions
	resp = env.doRequest(t, http.MethodGet, "/api/sessions", token, "")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var sessionList struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&sessionList))
	assert.Len(t, sessionList.Data, 1)
	assert.Equal(t, "Updated Service", sessionList.Data[0].Name)
}

// TestIntegrationABCAuthentication tests creating an ABC and verifying
// its token can be used for authentication lookups.
func TestIntegrationABCAuthentication(t *testing.T) {
	env := setupIntegrationServer(t)

	// 1. Create admin and login
	env.createAdminUser(t, "admin", "admin-pass-123")
	token := env.login(t, "admin", "admin-pass-123")

	// 2. Create an ABC via the API
	resp := env.doRequest(t, http.MethodPost, "/api/abcs", token,
		`{"name":"Booth A"}`)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var abcResult struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Token string `json:"token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&abcResult))
	assert.Equal(t, "Booth A", abcResult.Name)
	assert.NotEmpty(t, abcResult.ID)
	assert.NotEmpty(t, abcResult.Token, "ABC token should be returned on creation")

	// 3. Verify the ABC appears in the list
	resp = env.doRequest(t, http.MethodGet, "/api/abcs", token, "")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var abcList struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&abcList))
	assert.Len(t, abcList.Data, 1)
	assert.Equal(t, "Booth A", abcList.Data[0].Name)

	// 4. Verify the ABC token hash resolves correctly in the store
	tokenHash := auth.HashToken(abcResult.Token)
	abc, err := env.abcs.GetByTokenHash(context.Background(), tokenHash)
	require.NoError(t, err)
	assert.Equal(t, abcResult.ID, abc.ID)
	assert.Equal(t, "Booth A", abc.Name)

	// 5. Get the ABC by ID
	resp = env.doRequest(t, http.MethodGet, "/api/abcs/"+abcResult.ID, token, "")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var getABC struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&getABC))
	assert.Equal(t, abcResult.ID, getABC.ID)

	// 6. Create a session and assign it to the ABC
	resp = env.doRequest(t, http.MethodPost, "/api/sessions", token,
		`{"name":"Session for ABC"}`)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var sess struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&sess))

	resp = env.doRequest(t, http.MethodPut, "/api/abcs/"+abcResult.ID, token,
		fmt.Sprintf(`{"name":"Booth A","session_id":"%s"}`, sess.ID))
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var updatedABC struct {
		ID        string  `json:"id"`
		SessionID *string `json:"session_id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&updatedABC))
	require.NotNil(t, updatedABC.SessionID)
	assert.Equal(t, sess.ID, *updatedABC.SessionID)

	// 7. The ABC's API token authenticates against /api/webrtc/token — this is
	// how the headless CLI (KickPi board) obtains a signaling token. A JWT is
	// NOT required for ABCs.
	resp = env.doRequest(t, http.MethodPost, "/api/webrtc/token", abcResult.Token, "")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "ABC token should authenticate for webrtc token")
	var wrtc struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&wrtc))
	assert.NotEmpty(t, wrtc.Token)

	// A bogus token is rejected.
	bad := env.doRequest(t, http.MethodPost, "/api/webrtc/token", "not-a-real-token", "")
	defer bad.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, bad.StatusCode)
}

// TestIntegrationRBACEnforcement tests that translator role users cannot
// access admin-only endpoints.
func TestIntegrationRBACEnforcement(t *testing.T) {
	env := setupIntegrationServer(t)

	// 1. Create admin and a translator user
	env.createAdminUser(t, "admin", "admin-pass-123")
	adminToken := env.login(t, "admin", "admin-pass-123")

	// Create translator via API
	resp := env.doRequest(t, http.MethodPost, "/api/translators", adminToken,
		`{"username":"translator1","password":"trans-pass-123"}`)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// 2. Login as translator
	translatorToken := env.login(t, "translator1", "trans-pass-123")

	// 3. Translator CAN list sessions (allowed for admin + translator)
	resp = env.doRequest(t, http.MethodGet, "/api/sessions", translatorToken, "")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// 4. Translator CANNOT create sessions (admin only)
	resp = env.doRequest(t, http.MethodPost, "/api/sessions", translatorToken,
		`{"name":"Test","description":"test"}`)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)

	// 5. Translator CAN list ABCs (read-only, needed to choose booth monitors
	//    for their assigned sessions).
	resp = env.doRequest(t, http.MethodGet, "/api/abcs", translatorToken, "")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// 6. Translator CANNOT create ABCs (admin only)
	resp = env.doRequest(t, http.MethodPost, "/api/abcs", translatorToken,
		`{"name":"Hack Booth"}`)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)

	// 7. Translator CANNOT list users (admin only)
	resp = env.doRequest(t, http.MethodGet, "/api/users", translatorToken, "")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)

	// 8. Translator CANNOT create users (admin only)
	resp = env.doRequest(t, http.MethodPost, "/api/users", translatorToken,
		`{"username":"hacker","password":"hack","role":"admin"}`)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)

	// 9. Verify no auth = 401
	resp = env.doRequest(t, http.MethodGet, "/api/sessions", "", "")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// TestIntegrationMixOperations tests creating a session, adding sources,
// updating the mix, and verifying the state.
func TestIntegrationMixOperations(t *testing.T) {
	env := setupIntegrationServer(t)

	// 1. Setup: create admin, login, create session + channel
	env.createAdminUser(t, "admin", "admin-pass-123")
	token := env.login(t, "admin", "admin-pass-123")

	// Create session
	resp := env.doRequest(t, http.MethodPost, "/api/sessions", token,
		`{"name":"Mix Test Session"}`)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var session struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&session))

	// Create a broadcast channel
	resp = env.doRequest(t, http.MethodPost, "/api/sessions/"+session.ID+"/channels", token,
		`{"name":"English Output","type":"broadcast"}`)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var channel struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&channel))

	// 2. Create sources directly in the store (simulating connected clients)
	// Sources are created when WebRTC peers connect; there's no public create-source API.
	src1 := &crosstalk.Source{
		ID:        ulid.Make().String(),
		SessionID: session.ID,
		Name:      "Floor Mic",
		Origin:    crosstalk.OriginABC,
		Connected: true,
		FirstSeen: time.Now(),
		LastSeen:  time.Now(),
	}
	src2 := &crosstalk.Source{
		ID:        ulid.Make().String(),
		SessionID: session.ID,
		Name:      "Translator Mic",
		Origin:    crosstalk.OriginTranslator,
		Connected: true,
		FirstSeen: time.Now(),
		LastSeen:  time.Now(),
	}
	require.NoError(t, env.sources.Create(context.Background(), src1))
	require.NoError(t, env.sources.Create(context.Background(), src2))

	// 3. Set mix state for the channel
	mixBody := fmt.Sprintf(`{"entries":[{"source_id":"%s","muted":false,"level":1.0},{"source_id":"%s","muted":true,"level":0.5}]}`,
		src1.ID, src2.ID)
	resp = env.doRequest(t, http.MethodPut, "/api/sessions/"+session.ID+"/channels/"+channel.ID+"/mix", token, mixBody)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var mixResult struct {
		Data []struct {
			SourceID string  `json:"source_id"`
			Muted    bool    `json:"muted"`
			Level    float64 `json:"level"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&mixResult))
	assert.Len(t, mixResult.Data, 2)

	// 4. Verify mix state via GET
	resp = env.doRequest(t, http.MethodGet, "/api/sessions/"+session.ID+"/channels/"+channel.ID+"/mix", token, "")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var getMixResult struct {
		Data []struct {
			SourceID string  `json:"source_id"`
			Muted    bool    `json:"muted"`
			Level    float64 `json:"level"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&getMixResult))
	assert.Len(t, getMixResult.Data, 2)

	// Find entries by source ID
	var floorEntry, translatorEntry struct {
		SourceID string
		Muted    bool
		Level    float64
	}
	for _, e := range getMixResult.Data {
		if e.SourceID == src1.ID {
			floorEntry.SourceID = e.SourceID
			floorEntry.Muted = e.Muted
			floorEntry.Level = e.Level
		} else if e.SourceID == src2.ID {
			translatorEntry.SourceID = e.SourceID
			translatorEntry.Muted = e.Muted
			translatorEntry.Level = e.Level
		}
	}

	assert.Equal(t, src1.ID, floorEntry.SourceID)
	assert.False(t, floorEntry.Muted)
	assert.Equal(t, 1.0, floorEntry.Level)

	assert.Equal(t, src2.ID, translatorEntry.SourceID)
	assert.True(t, translatorEntry.Muted)
	assert.Equal(t, 0.5, translatorEntry.Level)

	// 5. Update mix: unmute translator, adjust levels
	mixBody = fmt.Sprintf(`{"entries":[{"source_id":"%s","muted":false,"level":0.8},{"source_id":"%s","muted":false,"level":1.2}]}`,
		src1.ID, src2.ID)
	resp = env.doRequest(t, http.MethodPut, "/api/sessions/"+session.ID+"/channels/"+channel.ID+"/mix", token, mixBody)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	require.NoError(t, json.NewDecoder(resp.Body).Decode(&mixResult))
	assert.Len(t, mixResult.Data, 2)

	// Verify updated state
	for _, e := range mixResult.Data {
		if e.SourceID == src1.ID {
			assert.False(t, e.Muted)
			assert.Equal(t, 0.8, e.Level)
		} else if e.SourceID == src2.ID {
			assert.False(t, e.Muted)
			assert.Equal(t, 1.2, e.Level)
		}
	}
}

// TestIntegrationDebugAPI verifies the admin debug API is mounted, requires an
// admin token, and returns live (initially empty) peer state rather than stubs.
func TestIntegrationDebugAPI(t *testing.T) {
	env := setupIntegrationServer(t)
	env.createAdminUser(t, "admin", "admin-pass-123")
	token := env.login(t, "admin", "admin-pass-123")

	// Unauthenticated request is rejected.
	unauth := env.doRequest(t, http.MethodGet, "/api/debug/peers", "", "")
	defer unauth.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, unauth.StatusCode)

	// Authenticated admin gets a real (empty) peer list.
	resp := env.doRequest(t, http.MethodGet, "/api/debug/peers", token, "")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var peers struct {
		Count int              `json:"count"`
		Peers []map[string]any `json:"peers"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&peers))
	assert.Equal(t, 0, peers.Count)
	assert.Empty(t, peers.Peers)

	// Events for a non-existent peer return 404.
	missing := env.doRequest(t, http.MethodGet, "/api/debug/peers/nope/events", token, "")
	defer missing.Body.Close()
	assert.Equal(t, http.StatusNotFound, missing.StatusCode)
}
