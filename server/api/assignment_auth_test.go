package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	crosstalk "github.com/aleksclark/crosstalk/server"
	"github.com/aleksclark/crosstalk/server/auth"
)

func TestAssignment_TranslatorSeesOnlyAssignedSessions(t *testing.T) {
	ts, _, users := setupTestServer(t)
	ctx := context.Background()

	adminTok := createAdminAndLogin(t, ts, users)

	hash, _ := auth.HashPassword("tr123")
	tr := &crosstalk.User{
		ID:           ulid.Make().String(),
		Username:     "tr1",
		PasswordHash: hash,
		Role:         "translator",
	}
	require.NoError(t, users.Create(ctx, tr))

	// Create two sessions as admin.
	create := func(name string) string {
		body := `{"name":"` + name + `"}`
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/sessions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+adminTok)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var sess struct {
			ID string `json:"id"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&sess))
		return sess.ID
	}
	sidA := create("Session A")
	sidB := create("Session B")
	require.NoError(t, users.AssignSessions(ctx, tr.ID, []string{sidA}))

	// Login translator.
	resp, err := http.Post(ts.URL+"/api/auth/login", "application/json", strings.NewReader(`{"username":"tr1","password":"tr123"}`))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var login struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&login))
	trTok := login.AccessToken

	// List filters to assigned only.
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/sessions", nil)
	req.Header.Set("Authorization", "Bearer "+trTok)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var list struct {
		Data []struct {
			ID             string `json:"id"`
			BroadcastToken string `json:"broadcast_token"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	require.Len(t, list.Data, 1)
	assert.Equal(t, sidA, list.Data[0].ID)
	assert.Empty(t, list.Data[0].BroadcastToken)

	// Get assigned OK; unassigned forbidden.
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/sessions/"+sidA, nil)
	req.Header.Set("Authorization", "Bearer "+trTok)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var got struct {
		BroadcastToken string `json:"broadcast_token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Empty(t, got.BroadcastToken)

	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/sessions/"+sidB, nil)
	req.Header.Set("Authorization", "Bearer "+trTok)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)

	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/sessions/"+sidB+"/channels", nil)
	req.Header.Set("Authorization", "Bearer "+trTok)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)

	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/sessions/"+sidB+"/sources", nil)
	req.Header.Set("Authorization", "Bearer "+trTok)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)

	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/sessions/"+sidA+"/broadcast-url", nil)
	req.Header.Set("Authorization", "Bearer "+trTok)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/sessions/"+sidB+"/broadcast-url", nil)
	req.Header.Set("Authorization", "Bearer "+trTok)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestAssignment_ChannelBelongsToSession(t *testing.T) {
	ts, _, users := setupTestServer(t)
	adminTok := createAdminAndLogin(t, ts, users)

	createSess := func(name string) string {
		body := `{"name":"` + name + `"}`
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/sessions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+adminTok)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var sess struct {
			ID string `json:"id"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&sess))
		return sess.ID
	}
	createCh := func(sid, name, typ string) string {
		body := `{"name":"` + name + `","type":"` + typ + `"}`
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/sessions/"+sid+"/channels", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+adminTok)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var ch struct {
			ID string `json:"id"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&ch))
		return ch.ID
	}

	sidA := createSess("A")
	sidB := createSess("B")
	chA := createCh(sidA, "Feed A", "feed")
	_ = createCh(sidB, "Feed B", "feed")

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/sessions/"+sidB+"/channels/"+chA+"/mix", nil)
	req.Header.Set("Authorization", "Bearer "+adminTok)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestAssignment_TranslatorCannotAccessOtherSessionMix(t *testing.T) {
	ts, _, users := setupTestServer(t)
	ctx := context.Background()
	adminTok := createAdminAndLogin(t, ts, users)

	hash, _ := auth.HashPassword("tr123")
	tr := &crosstalk.User{
		ID:           ulid.Make().String(),
		Username:     "trmix",
		PasswordHash: hash,
		Role:         "translator",
	}
	require.NoError(t, users.Create(ctx, tr))

	createSess := func(name string) string {
		body := `{"name":"` + name + `"}`
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/sessions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+adminTok)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		var sess struct {
			ID string `json:"id"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&sess))
		return sess.ID
	}
	sidA := createSess("A")
	sidB := createSess("B")
	body := `{"name":"Bcast","type":"broadcast"}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/sessions/"+sidB+"/channels", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminTok)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	var ch struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&ch))
	require.NoError(t, users.AssignSessions(ctx, tr.ID, []string{sidA}))

	resp, err = http.Post(ts.URL+"/api/auth/login", "application/json", strings.NewReader(`{"username":"trmix","password":"tr123"}`))
	require.NoError(t, err)
	defer resp.Body.Close()
	var login struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&login))
	trTok := login.AccessToken

	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/sessions/"+sidB+"/channels/"+ch.ID+"/mix", nil)
	req.Header.Set("Authorization", "Bearer "+trTok)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}
