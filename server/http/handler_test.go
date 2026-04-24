package http_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	crosstalk "github.com/aleksclark/crosstalk/server"
	cthttp "github.com/aleksclark/crosstalk/server/http"
	"github.com/aleksclark/crosstalk/server/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// authToken sets up a valid Bearer token for authenticated requests and returns
// the plaintext token string to set on Authorization headers.
func authToken(t *testing.T, ts *mock.TokenService) string {
	t.Helper()
	token := cthttp.GenerateToken()
	hash := cthttp.HashToken(token)

	apiToken := &crosstalk.APIToken{
		ID:        "auth-tok-1",
		Name:      "test",
		TokenHash: hash,
		UserID:    "user-1",
		CreatedAt: time.Now(),
	}

	ts.FindTokenByHashFn = func(h string) (*crosstalk.APIToken, error) {
		if h == hash {
			return apiToken, nil
		}
		return nil, sql.ErrNoRows
	}

	return token
}

func TestCreateTemplate(t *testing.T) {
	h, _, ts, tmplSvc, _ := newTestHandler(t)
	token := authToken(t, ts)

	tmplSvc.CreateTemplateFn = func(tmpl *crosstalk.SessionTemplate) error {
		return nil
	}

	body := `{"name":"Interview","roles":[{"name":"host","multi_client":false}],"mappings":[],"is_default":false}`
	req := httptest.NewRequest("POST", "/api/templates", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	h.Router().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "Interview", resp["name"])
	assert.NotEmpty(t, resp["id"])
	assert.True(t, tmplSvc.CreateTemplateInvoked)
}

func TestListTemplates(t *testing.T) {
	h, _, ts, tmplSvc, _ := newTestHandler(t)
	token := authToken(t, ts)

	now := time.Now().UTC()
	tmplSvc.ListTemplatesFn = func() ([]crosstalk.SessionTemplate, error) {
		return []crosstalk.SessionTemplate{
			{
				ID:        "tmpl-1",
				Name:      "Default",
				IsDefault: true,
				Roles:     []crosstalk.Role{{Name: "host"}},
				Mappings:  []crosstalk.Mapping{},
				CreatedAt: now,
				UpdatedAt: now,
			},
		}, nil
	}

	req := httptest.NewRequest("GET", "/api/templates", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	h.Router().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp []map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.Len(t, resp, 1)
	assert.Equal(t, "Default", resp[0]["name"])
}

func TestCreateSession(t *testing.T) {
	h, _, ts, tmplSvc, sessSvc := newTestHandler(t)
	token := authToken(t, ts)

	tmplSvc.FindTemplateByIDFn = func(id string) (*crosstalk.SessionTemplate, error) {
		if id == "tmpl-1" {
			return &crosstalk.SessionTemplate{ID: "tmpl-1", Name: "Default"}, nil
		}
		return nil, sql.ErrNoRows
	}
	sessSvc.CreateSessionFn = func(s *crosstalk.Session) error {
		return nil
	}

	body := `{"template_id":"tmpl-1","name":"Test Session"}`
	req := httptest.NewRequest("POST", "/api/sessions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	h.Router().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "Test Session", resp["name"])
	assert.Equal(t, "tmpl-1", resp["template_id"])
	assert.Equal(t, "waiting", resp["status"])
	assert.NotEmpty(t, resp["id"])
}

func TestGetSession(t *testing.T) {
	h, _, ts, tmplSvc, sessSvc := newTestHandler(t)
	token := authToken(t, ts)

	now := time.Now().UTC()
	sessSvc.FindSessionByIDFn = func(id string) (*crosstalk.Session, error) {
		if id == "sess-1" {
			return &crosstalk.Session{
				ID:         "sess-1",
				TemplateID: "tmpl-1",
				Name:       "My Session",
				Status:     crosstalk.SessionWaiting,
				CreatedAt:  now,
			}, nil
		}
		return nil, sql.ErrNoRows
	}
	tmplSvc.FindTemplateByIDFn = func(id string) (*crosstalk.SessionTemplate, error) {
		if id == "tmpl-1" {
			return &crosstalk.SessionTemplate{
				ID:    "tmpl-1",
				Name:  "Default",
				Roles: []crosstalk.Role{{Name: "host"}},
			}, nil
		}
		return nil, sql.ErrNoRows
	}

	req := httptest.NewRequest("GET", "/api/sessions/sess-1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	h.Router().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "sess-1", resp["id"])
	assert.Equal(t, "My Session", resp["name"])
}

// mockOrchestrator implements crosstalk.SessionOrchestrator for handler tests.
type mockOrchestrator struct {
	endSessionFn      func(string)
	recordingStatusFn func(string) *crosstalk.RecordingInfo
	assignSessionFn   func(string, string, string) error
}

func (m *mockOrchestrator) EndSession(sessionID string) {
	if m.endSessionFn != nil {
		m.endSessionFn(sessionID)
	}
}

func (m *mockOrchestrator) RecordingStatus(sessionID string) *crosstalk.RecordingInfo {
	if m.recordingStatusFn != nil {
		return m.recordingStatusFn(sessionID)
	}
	return nil
}

func (m *mockOrchestrator) AssignSession(peerID, sessionID, role string) error {
	if m.assignSessionFn != nil {
		return m.assignSessionFn(peerID, sessionID, role)
	}
	return nil
}

func TestGetSession_WithRecordingStatus(t *testing.T) {
	h, _, ts, tmplSvc, sessSvc := newTestHandler(t)
	token := authToken(t, ts)

	orch := &mockOrchestrator{
		recordingStatusFn: func(id string) *crosstalk.RecordingInfo {
			if id == "sess-rec" {
				return &crosstalk.RecordingInfo{
					Active:     true,
					FileCount:  2,
					TotalBytes: 4096,
				}
			}
			return nil
		},
	}
	h.Orchestrator = orch

	tmplSvc.FindTemplateByIDFn = func(id string) (*crosstalk.SessionTemplate, error) {
		if id == "tmpl-1" {
			return &crosstalk.SessionTemplate{
				ID:    "tmpl-1",
				Name:  "Default",
				Roles: []crosstalk.Role{{Name: "host"}},
			}, nil
		}
		return nil, sql.ErrNoRows
	}

	now := time.Now().UTC()
	sessSvc.FindSessionByIDFn = func(id string) (*crosstalk.Session, error) {
		if id == "sess-rec" {
			return &crosstalk.Session{
				ID:         "sess-rec",
				TemplateID: "tmpl-1",
				Name:       "Recording Session",
				Status:     crosstalk.SessionActive,
				CreatedAt:  now,
			}, nil
		}
		return nil, sql.ErrNoRows
	}

	req := httptest.NewRequest("GET", "/api/sessions/sess-rec", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	h.Router().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "sess-rec", resp["id"])

	recording, ok := resp["recording"].(map[string]any)
	require.True(t, ok, "response should include recording info")
	assert.Equal(t, true, recording["active"])
	assert.Equal(t, float64(2), recording["file_count"])
	assert.Equal(t, float64(4096), recording["total_bytes"])
}

func TestAuthRequired_NoHeader(t *testing.T) {
	h, _, ts, _, _ := newTestHandler(t)
	// Set up FindTokenByHashFn so it doesn't panic if called.
	ts.FindTokenByHashFn = func(string) (*crosstalk.APIToken, error) {
		return nil, sql.ErrNoRows
	}

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/templates"},
		{"POST", "/api/templates"},
		{"GET", "/api/sessions"},
		{"POST", "/api/sessions"},
		{"GET", "/api/users"},
		{"GET", "/api/tokens"},
		{"POST", "/api/auth/logout"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			rec := httptest.NewRecorder()
			h.Router().ServeHTTP(rec, req)

			assert.Equal(t, http.StatusUnauthorized, rec.Code)

			var resp map[string]any
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			errObj, ok := resp["error"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, float64(401), errObj["status"])
		})
	}
}

func TestLoginDoesNotRequireAuth(t *testing.T) {
	h, us, ts, _, _ := newTestHandler(t)
	// Login should not require auth, even though the body may be invalid.
	// It should return 400 (bad request), not 401 (unauthorized).
	ts.FindTokenByHashFn = func(string) (*crosstalk.APIToken, error) {
		return nil, sql.ErrNoRows
	}
	us.FindUserByUsernameFn = func(string) (*crosstalk.User, error) {
		return nil, sql.ErrNoRows
	}

	body := `{"username":"nobody","password":"nope"}`
	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)

	// Should get 401 "invalid credentials" (user not found), not middleware 401.
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	errObj := resp["error"].(map[string]any)
	assert.Equal(t, "invalid credentials", errObj["message"])
}

func TestWebRTCToken_ReturnsAPITokenInfo(t *testing.T) {
	h, _, ts, _, _ := newTestHandler(t)
	token := authToken(t, ts)

	req := httptest.NewRequest("POST", "/api/webrtc/token", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "use_api_token", resp["token"])
	assert.Contains(t, resp["note"], "API token directly")
	assert.Equal(t, "user-1", resp["user_id"])
}

func TestOpenAPI(t *testing.T) {
	h, _, ts, _, _ := newTestHandler(t)
	token := authToken(t, ts)

	req := httptest.NewRequest("GET", "/api/openapi.json", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "3.1.0", resp["openapi"])

	paths, ok := resp["paths"].(map[string]any)
	require.True(t, ok, "paths should be an object")
	assert.NotEmpty(t, paths, "paths should not be empty")
	assert.Contains(t, paths, "/api/auth/login")
	assert.Contains(t, paths, "/api/users")
	assert.Contains(t, paths, "/api/tokens")
	assert.Contains(t, paths, "/api/templates")
	assert.Contains(t, paths, "/api/sessions")
	assert.Contains(t, paths, "/api/clients")
}

func TestListClients_ReturnsEmptyArray(t *testing.T) {
	h, _, ts, _, _ := newTestHandler(t)
	token := authToken(t, ts)

	req := httptest.NewRequest("GET", "/api/clients", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp []any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Empty(t, resp)
}

func TestGetClient_Returns404(t *testing.T) {
	h, _, ts, _, _ := newTestHandler(t)
	token := authToken(t, ts)

	req := httptest.NewRequest("GET", "/api/clients/some-id", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}
