package pion

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	crosstalk "github.com/aleksclark/crosstalk/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthClient_RequestWebRTCToken_Success(t *testing.T) {
	expectedToken := "wrt_test123"
	var receivedAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")

		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/webrtc/token", r.URL.Path)

		resp := crosstalk.WebRTCTokenResponse{Token: expectedToken}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewAuthClient(srv.URL, "my-api-token")
	token, err := client.RequestWebRTCToken()

	require.NoError(t, err)
	assert.Equal(t, expectedToken, token)
	assert.Equal(t, "Bearer my-api-token", receivedAuth)
}

func TestAuthClient_RequestWebRTCToken_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid token"}`))
	}))
	defer srv.Close()

	client := NewAuthClient(srv.URL, "bad-token")
	_, err := client.RequestWebRTCToken()

	require.Error(t, err)
	assert.True(t, IsAuthError(err), "expected AuthError, got %T", err)
	assert.Contains(t, err.Error(), "authentication failed")
	assert.Contains(t, err.Error(), "401")
}

func TestAuthClient_RequestWebRTCToken_Forbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":"forbidden"}`))
	}))
	defer srv.Close()

	client := NewAuthClient(srv.URL, "forbidden-token")
	_, err := client.RequestWebRTCToken()

	require.Error(t, err)
	assert.True(t, IsAuthError(err), "expected AuthError, got %T", err)
	assert.Contains(t, err.Error(), "403")
}

func TestAuthClient_RequestWebRTCToken_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`internal error`))
	}))
	defer srv.Close()

	client := NewAuthClient(srv.URL, "my-token")
	_, err := client.RequestWebRTCToken()

	require.Error(t, err)
	assert.False(t, IsAuthError(err), "should not be an AuthError")
	assert.Contains(t, err.Error(), "500")
}

func TestAuthClient_RequestWebRTCToken_EmptyToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := crosstalk.WebRTCTokenResponse{Token: ""}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewAuthClient(srv.URL, "my-token")
	_, err := client.RequestWebRTCToken()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestAuthClient_RequestWebRTCToken_ConnectionRefused(t *testing.T) {
	client := NewAuthClient("http://127.0.0.1:1", "my-token")
	_, err := client.RequestWebRTCToken()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "requesting webrtc token")
}

func TestIsAuthError(t *testing.T) {
	assert.True(t, IsAuthError(&AuthError{StatusCode: 401, Message: "test"}))
	assert.False(t, IsAuthError(assert.AnError))
}
