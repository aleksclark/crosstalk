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
	"github.com/anthropics/crosstalk/server/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	handler := &cthttp.Handler{
		UserService:            userService,
		TokenService:           tokenService,
		SessionTemplateService: templateService,
		SessionService:         sessionService,
		Config:                 crosstalk.DefaultConfig(),
		WebFS:                  webFS,
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
