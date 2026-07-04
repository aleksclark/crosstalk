package api_test

import (
	"context"
	"encoding/json"
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
)

func setupTestServer(t *testing.T) (*httptest.Server, *auth.Service, crosstalk.UserService) {
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

	authCfg := auth.Config{
		Secret:          "test-secret",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
	}
	authService := auth.NewService(authCfg, userStore, refreshTokenStore)

	svc := api.Services{
		Sessions:      sessionStore,
		Channels:      channelStore,
		Sources:       sourceStore,
		Mix:           mixStore,
		ABCs:          abcStore,
		Users:         userStore,
		RefreshTokens: refreshTokenStore,
		Auth:          authService,
	}

	cfg := api.Config{Addr: ":0", JWTSecret: "test-secret"}
	srv := api.NewServer(cfg, svc, log)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	return ts, authService, userStore
}

func createAdminAndLogin(t *testing.T, ts *httptest.Server, users crosstalk.UserService) string {
	t.Helper()
	ctx := context.Background()
	hash, _ := auth.HashPassword("admin123")
	admin := &crosstalk.User{
		ID:           ulid.Make().String(),
		Username:     "admin",
		PasswordHash: hash,
		Role:         "admin",
	}
	require.NoError(t, users.Create(ctx, admin))

	// Login
	body := `{"username":"admin","password":"admin123"}`
	resp, err := http.Post(ts.URL+"/api/auth/login", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	return result.AccessToken
}

func TestOpenAPISpec(t *testing.T) {
	ts, _, _ := setupTestServer(t)

	resp, err := http.Get(ts.URL + "/api/openapi.json")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var spec map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&spec))
	assert.Equal(t, "3.1.0", spec["openapi"])

	info, ok := spec["info"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "CrossTalk", info["title"])
}

func TestLoginEndpoint(t *testing.T) {
	ts, _, users := setupTestServer(t)
	ctx := context.Background()

	hash, _ := auth.HashPassword("password123")
	user := &crosstalk.User{
		ID:           ulid.Make().String(),
		Username:     "testuser",
		PasswordHash: hash,
		Role:         "admin",
	}
	require.NoError(t, users.Create(ctx, user))

	// Success
	body := `{"username":"testuser","password":"password123"}`
	resp, err := http.Post(ts.URL+"/api/auth/login", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	assert.NotEmpty(t, result.AccessToken)
	assert.NotEmpty(t, result.RefreshToken)

	// Failure
	body = `{"username":"testuser","password":"wrong"}`
	resp2, err := http.Post(ts.URL+"/api/auth/login", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp2.StatusCode)
}

func TestSessionsCRUD(t *testing.T) {
	ts, _, users := setupTestServer(t)
	token := createAdminAndLogin(t, ts, users)

	// Create session
	body := `{"name":"Sunday Service","description":"Weekly gathering"}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/sessions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var session struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Description    string `json:"description"`
		BroadcastToken string `json:"broadcast_token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&session))
	assert.Equal(t, "Sunday Service", session.Name)
	assert.NotEmpty(t, session.ID)
	assert.NotEmpty(t, session.BroadcastToken)

	// List sessions
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/sessions", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var listResp struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listResp))
	assert.Len(t, listResp.Data, 1)

	// Get session
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/sessions/"+session.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Update session
	body = `{"name":"Updated Service","description":"Updated desc"}`
	req, _ = http.NewRequest(http.MethodPut, ts.URL+"/api/sessions/"+session.ID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Delete session
	req, _ = http.NewRequest(http.MethodDelete, ts.URL+"/api/sessions/"+session.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestUnauthorizedAccess(t *testing.T) {
	ts, _, _ := setupTestServer(t)

	// No auth header
	resp, err := http.Get(ts.URL + "/api/sessions")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// Invalid token
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/sessions", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestTranslatorRoleRestrictions(t *testing.T) {
	ts, _, users := setupTestServer(t)
	ctx := context.Background()

	// Create translator
	hash, _ := auth.HashPassword("trans123")
	translator := &crosstalk.User{
		ID:           ulid.Make().String(),
		Username:     "translator1",
		PasswordHash: hash,
		Role:         "translator",
	}
	require.NoError(t, users.Create(ctx, translator))

	// Login as translator
	body := `{"username":"translator1","password":"trans123"}`
	resp, err := http.Post(ts.URL+"/api/auth/login", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	token := result.AccessToken

	// Translator CAN list sessions
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/sessions", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Translator CANNOT create sessions (admin only)
	body = `{"name":"Test","description":"test"}`
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/sessions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)

	// Translator CANNOT list ABCs (admin only)
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/abcs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestPublicBroadcastEndpoint(t *testing.T) {
	ts, _, users := setupTestServer(t)
	token := createAdminAndLogin(t, ts, users)

	// Create a session first
	body := `{"name":"Public Test"}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/sessions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var session struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&session))

	// Public broadcast endpoint - no auth needed
	resp, err = http.Get(ts.URL + "/api/sessions/" + session.ID + "/broadcast")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var broadcast struct {
		SessionID   string `json:"session_id"`
		SessionName string `json:"session_name"`
		Active      bool   `json:"active"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&broadcast))
	assert.Equal(t, session.ID, broadcast.SessionID)
	assert.Equal(t, "Public Test", broadcast.SessionName)
}
