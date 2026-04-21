package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	crosstalk "github.com/anthropics/crosstalk/server"
	cthttp "github.com/anthropics/crosstalk/server/http"
	ctpion "github.com/anthropics/crosstalk/server/pion"
	ctws "github.com/anthropics/crosstalk/server/ws"
	"github.com/anthropics/crosstalk/server/sqlite"
	"github.com/pion/ice/v4"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"nhooyr.io/websocket"
)

// testServer sets up a fully-wired ct-server for integration tests. It returns
// the server's base URL and a seed API token that can be used for authenticated
// requests. The server and DB are cleaned up when the test ends.
func testServer(t *testing.T) (baseURL, seedToken string) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Open database (runs migrations).
	db, err := sqlite.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	// Wire services.
	userService := &sqlite.UserService{DB: db.DB}
	tokenService := &sqlite.TokenService{DB: db.DB}
	templateService := &sqlite.SessionTemplateService{DB: db.DB}
	sessionService := &sqlite.SessionService{DB: db.DB}

	// Seed admin and capture the token.
	seedToken, err = seedAdminForTest(userService, tokenService)
	require.NoError(t, err)

	// Build embedded web FS.
	webFS, err := fs.Sub(crosstalk.WebDist, "web/dist")
	require.NoError(t, err)

	// Build handler.

	// Create PeerManager with mDNS disabled for in-process tests.
	var se webrtc.SettingEngine
	se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	api := webrtc.NewAPI(webrtc.WithSettingEngine(se))
	pm := ctpion.NewPeerManagerWithAPI(crosstalk.WebRTCConfig{}, api)
	sigHandler := ctws.SignalingHandler{
		TokenService: tokenService,
		PeerManager:  pm,
	}

	handler := &cthttp.Handler{
		UserService:            userService,
		TokenService:           tokenService,
		SessionTemplateService: templateService,
		SessionService:         sessionService,
		Config:                 crosstalk.DefaultConfig(),
		WebFS:                  webFS,
		SignalingHandler:       &sigHandler,
	}

	// Pick a random available port.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := &http.Server{Handler: handler.Router()}
	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Logf("test server error: %v", err)
		}
	}()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	})

	baseURL = fmt.Sprintf("http://%s", listener.Addr().String())
	return baseURL, seedToken
}

// seedAdminForTest creates an admin user and seed token, returning the
// plaintext API token.
func seedAdminForTest(userService crosstalk.UserService, tokenService crosstalk.TokenService) (string, error) {
	now := time.Now().UTC()
	hash, err := cthttp.HashPassword("test-password")
	if err != nil {
		return "", err
	}

	user := &crosstalk.User{
		ID:           "test-admin-id",
		Username:     "admin",
		PasswordHash: hash,
		CreatedAt:    now,
	}
	if err := userService.CreateUser(user); err != nil {
		return "", err
	}

	plaintext := cthttp.GenerateToken()
	token := &crosstalk.APIToken{
		ID:        "test-token-id",
		Name:      "seed",
		TokenHash: cthttp.HashToken(plaintext),
		UserID:    user.ID,
		CreatedAt: now,
	}
	if err := tokenService.CreateToken(token); err != nil {
		return "", err
	}

	return plaintext, nil
}

func TestServerIntegration_ListTemplates(t *testing.T) {
	baseURL, token := testServer(t)

	req, err := http.NewRequest("GET", baseURL+"/api/templates", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var templates []json.RawMessage
	err = json.NewDecoder(resp.Body).Decode(&templates)
	require.NoError(t, err)
	assert.Equal(t, 0, len(templates), "fresh DB should have no templates")
}

func TestServerIntegration_UnauthenticatedReturns401(t *testing.T) {
	baseURL, _ := testServer(t)

	resp, err := http.Get(baseURL + "/api/templates")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestServerIntegration_LoginWithSeedUser(t *testing.T) {
	baseURL, _ := testServer(t)

	body := `{"username":"admin","password":"test-password"}`
	resp, err := http.Post(baseURL+"/api/auth/login", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]string
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	assert.NotEmpty(t, result["token"], "login should return a token")
}

func TestSeedAdmin_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sqlite.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	userService := &sqlite.UserService{DB: db.DB}
	tokenService := &sqlite.TokenService{DB: db.DB}

	// First seed should create the user.
	err = seedAdmin(userService, tokenService)
	require.NoError(t, err)

	admin, err := userService.FindUserByUsername("admin")
	require.NoError(t, err)
	assert.Equal(t, "admin", admin.Username)

	// Second seed should be a no-op.
	err = seedAdmin(userService, tokenService)
	require.NoError(t, err)

	// Should still be exactly one admin user.
	users, err := userService.ListUsers()
	require.NoError(t, err)

	adminCount := 0
	for _, u := range users {
		if u.Username == "admin" {
			adminCount++
		}
	}
	assert.Equal(t, 1, adminCount, "seed should be idempotent")
}

func TestSeedAdmin_CreatesToken(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sqlite.Open(dbPath)
	require.NoError(t, err)
	defer db.Close()

	userService := &sqlite.UserService{DB: db.DB}
	tokenService := &sqlite.TokenService{DB: db.DB}

	err = seedAdmin(userService, tokenService)
	require.NoError(t, err)

	tokens, err := tokenService.ListTokens()
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	assert.Equal(t, "seed", tokens[0].Name)
}

func TestServerBuild(t *testing.T) {
	// Verify the binary builds successfully. This is a fast sanity check.
	// We just verify it compiles — the binary is produced by `go build` in CI.
	// The existence of this test (which imports main package symbols) validates
	// that all wiring compiles correctly.
	_ = seedAdmin // ensure seedAdmin is reachable
	_ = run       // ensure run is reachable
}

// Verify database is closed properly by checking that the file exists and is
// a valid SQLite database after opening and closing.
func TestDatabaseOpenClose(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sqlite.Open(dbPath)
	require.NoError(t, err)

	// Verify the DB is functional.
	var result int
	err = db.QueryRow("SELECT 1").Scan(&result)
	require.NoError(t, err)
	assert.Equal(t, 1, result)

	err = db.Close()
	require.NoError(t, err)

	// File should exist after close.
	_, err = os.Stat(dbPath)
	require.NoError(t, err)
}

func TestServerIntegration_WebSocketSignaling(t *testing.T) {
	baseURL, token := testServer(t)

	// 1. Connect to the WebSocket signaling endpoint with the seed token.
	wsURL := strings.Replace(baseURL, "http://", "ws://", 1) + "/ws/signaling?token=" + token

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	wsConn, _, err := websocket.Dial(ctx, wsURL, nil)
	require.NoError(t, err, "WebSocket dial should succeed")
	defer wsConn.Close(websocket.StatusNormalClosure, "test done")

	// 2. Create a local Pion PeerConnection as the client.
	var se webrtc.SettingEngine
	se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	clientAPI := webrtc.NewAPI(webrtc.WithSettingEngine(se))

	clientPC, err := clientAPI.NewPeerConnection(webrtc.Configuration{})
	require.NoError(t, err)
	t.Cleanup(func() { clientPC.Close() }) //nolint:errcheck

	// Client needs a data channel for proper SDP generation.
	_, err = clientPC.CreateDataChannel("init", nil)
	require.NoError(t, err)

	// 3. Create offer and gather ICE candidates.
	offer, err := clientPC.CreateOffer(nil)
	require.NoError(t, err)

	gatherDone := webrtc.GatheringCompletePromise(clientPC)
	require.NoError(t, clientPC.SetLocalDescription(offer))
	<-gatherDone

	fullOffer := *clientPC.LocalDescription()

	// 4. Send SDP offer over WebSocket.
	offerMsg, err := json.Marshal(map[string]string{
		"type": "offer",
		"sdp":  fullOffer.SDP,
	})
	require.NoError(t, err)
	err = wsConn.Write(ctx, websocket.MessageText, offerMsg)
	require.NoError(t, err)

	// 5. Read messages until we get the SDP answer.
	var answerSDP string
	deadline := time.After(10 * time.Second)
	for {
		readCtx, readCancel := context.WithTimeout(ctx, 5*time.Second)
		_, data, readErr := wsConn.Read(readCtx)
		readCancel()
		require.NoError(t, readErr, "should read signaling message")

		var msg struct {
			Type string `json:"type"`
			SDP  string `json:"sdp"`
		}
		require.NoError(t, json.Unmarshal(data, &msg))

		if msg.Type == "answer" {
			answerSDP = msg.SDP
			break
		}

		// Might receive ICE candidates before the answer; keep reading.
		select {
		case <-deadline:
			t.Fatal("timed out waiting for SDP answer")
		default:
		}
	}

	require.NotEmpty(t, answerSDP, "should receive a non-empty SDP answer")

	// 6. Set the remote description on the client.
	err = clientPC.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  answerSDP,
	})
	require.NoError(t, err)

	// 7. Verify the WebSocket connection is still open by sending a ping-like
	//    no-op message (unknown type is just logged/ignored, not fatal).
	noopMsg, _ := json.Marshal(map[string]string{"type": "ping"})
	err = wsConn.Write(ctx, websocket.MessageText, noopMsg)
	assert.NoError(t, err, "WebSocket should still be open after signaling")
}

func TestServerIntegration_WebSocketSignaling_NoToken(t *testing.T) {
	baseURL, _ := testServer(t)

	// Attempt without a token — should get HTTP 401 before upgrade.
	wsURL := strings.Replace(baseURL, "http://", "ws://", 1) + "/ws/signaling"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, resp, err := websocket.Dial(ctx, wsURL, nil)
	require.Error(t, err, "should fail without token")
	if resp != nil {
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	}
}

func TestServerIntegration_WebSocketSignaling_InvalidToken(t *testing.T) {
	baseURL, _ := testServer(t)

	// Attempt with a bogus token — should get HTTP 401.
	wsURL := strings.Replace(baseURL, "http://", "ws://", 1) + "/ws/signaling?token=bogus-invalid"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, resp, err := websocket.Dial(ctx, wsURL, nil)
	require.Error(t, err, "should fail with invalid token")
	if resp != nil {
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	}
}
