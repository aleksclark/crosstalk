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

func TestHashToken_Consistent(t *testing.T) {
	plain := "ct_abc123"
	h1 := cthttp.HashToken(plain)
	h2 := cthttp.HashToken(plain)
	assert.Equal(t, h1, h2, "HashToken should produce consistent results")
	assert.Len(t, h1, 64, "SHA-256 hex should be 64 characters")
}

func TestHashToken_DifferentInputs(t *testing.T) {
	h1 := cthttp.HashToken("token_a")
	h2 := cthttp.HashToken("token_b")
	assert.NotEqual(t, h1, h2, "different inputs should produce different hashes")
}

func TestHashPassword_RoundTrip(t *testing.T) {
	password := "supersecret"
	hash, err := cthttp.HashPassword(password)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.NotEqual(t, password, hash)
	assert.True(t, cthttp.CheckPassword(hash, password), "CheckPassword should match the original password")
}

func TestCheckPassword_WrongPassword(t *testing.T) {
	hash, err := cthttp.HashPassword("correct")
	require.NoError(t, err)
	assert.False(t, cthttp.CheckPassword(hash, "wrong"), "CheckPassword should reject wrong password")
}

func TestGenerateToken(t *testing.T) {
	tok := cthttp.GenerateToken()
	assert.True(t, len(tok) > 3, "token should have content")
	assert.Equal(t, "ct_", tok[:3], "token should start with ct_ prefix")
	// 32 random bytes = 64 hex chars + 3 prefix = 67
	assert.Len(t, tok, 67, "token should be ct_ + 64 hex chars")
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	token := cthttp.GenerateToken()
	hash := cthttp.HashToken(token)

	apiToken := &crosstalk.APIToken{
		ID:        "tok-1",
		Name:      "test",
		TokenHash: hash,
		UserID:    "user-1",
		CreatedAt: time.Now(),
	}

	ts := &mock.TokenService{
		FindTokenByHashFn: func(h string) (*crosstalk.APIToken, error) {
			if h == hash {
				return apiToken, nil
			}
			return nil, sql.ErrNoRows
		},
	}

	handler := cthttp.AuthMiddleware(ts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := cthttp.TokenFromContext(r.Context())
		require.NotNil(t, tok)
		assert.Equal(t, "tok-1", tok.ID)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAuthMiddleware_MissingHeader(t *testing.T) {
	ts := &mock.TokenService{
		FindTokenByHashFn: func(string) (*crosstalk.APIToken, error) {
			return nil, sql.ErrNoRows
		},
	}

	handler := cthttp.AuthMiddleware(ts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	errObj, ok := resp["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(401), errObj["status"])
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	ts := &mock.TokenService{
		FindTokenByHashFn: func(string) (*crosstalk.APIToken, error) {
			return nil, sql.ErrNoRows
		},
	}

	handler := cthttp.AuthMiddleware(ts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer invalid_token_here")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// --- Login handler tests ---

func newTestHandler(t *testing.T) (*cthttp.Handler, *mock.UserService, *mock.TokenService, *mock.SessionTemplateService, *mock.SessionService) {
	t.Helper()
	us := &mock.UserService{}
	ts := &mock.TokenService{}
	tmplSvc := &mock.SessionTemplateService{}
	sessSvc := &mock.SessionService{}

	h := &cthttp.Handler{
		UserService:            us,
		TokenService:           ts,
		SessionTemplateService: tmplSvc,
		SessionService:         sessSvc,
		Config: crosstalk.Config{
			Auth: crosstalk.AuthConfig{
				SessionSecret:       "test-secret",
				WebRTCTokenLifetime: "1h",
			},
		},
	}
	return h, us, ts, tmplSvc, sessSvc
}

func TestLogin_CorrectPassword(t *testing.T) {
	h, us, ts, _, _ := newTestHandler(t)

	hash, err := cthttp.HashPassword("goodpass")
	require.NoError(t, err)

	us.FindUserByUsernameFn = func(username string) (*crosstalk.User, error) {
		if username == "admin" {
			return &crosstalk.User{
				ID:           "user-1",
				Username:     "admin",
				PasswordHash: hash,
				CreatedAt:    time.Now(),
			}, nil
		}
		return nil, sql.ErrNoRows
	}
	ts.CreateTokenFn = func(tok *crosstalk.APIToken) error {
		return nil
	}

	body := `{"username":"admin","password":"goodpass"}`
	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Router().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Contains(t, resp["token"], "ct_")
	assert.NotNil(t, resp["user"])
}

func TestLogin_WrongPassword(t *testing.T) {
	h, us, _, _, _ := newTestHandler(t)

	hash, err := cthttp.HashPassword("goodpass")
	require.NoError(t, err)

	us.FindUserByUsernameFn = func(username string) (*crosstalk.User, error) {
		return &crosstalk.User{
			ID:           "user-1",
			Username:     "admin",
			PasswordHash: hash,
			CreatedAt:    time.Now(),
		}, nil
	}

	body := `{"username":"admin","password":"wrongpass"}`
	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Router().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	errObj, ok := resp["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "invalid credentials", errObj["message"])
}
