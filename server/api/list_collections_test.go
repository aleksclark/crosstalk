package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	crosstalk "github.com/aleksclark/crosstalk/server"
	"github.com/aleksclark/crosstalk/server/auth"
	"github.com/aleksclark/crosstalk/server/postgres"
)

func authGET(t *testing.T, base, path, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, base+path, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func TestListSessions_BoundedPagination(t *testing.T) {
	ts, _, users := setupTestServer(t)
	token := createAdminAndLogin(t, ts, users)
	ctx := context.Background()

	// Seed via store for controlled timestamps.
	// Re-open stores from the same server is awkward; create via API then rely on order.
	// Use direct store from a parallel pgtest would not share DB — create via API.
	for i := 0; i < 5; i++ {
		body := fmt.Sprintf(`{"name":"Sess %02d"}`, i)
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/sessions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		_ = resp.Body.Close()
		time.Sleep(2 * time.Millisecond)
	}

	resp := authGET(t, ts.URL, "/api/sessions?limit=2", token)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var page1 struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
		NextCursor string `json:"next_cursor"`
		Total      *int64 `json:"total"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&page1))
	require.Len(t, page1.Data, 2)
	require.NotEmpty(t, page1.NextCursor)
	require.NotNil(t, page1.Total)
	assert.Equal(t, int64(5), *page1.Total)

	resp2 := authGET(t, ts.URL, "/api/sessions?limit=2&cursor="+url.QueryEscape(page1.NextCursor), token)
	defer resp2.Body.Close()
	require.Equal(t, http.StatusOK, resp2.StatusCode)
	var page2 struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		NextCursor string `json:"next_cursor"`
	}
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&page2))
	require.Len(t, page2.Data, 2)
	assert.NotEqual(t, page1.Data[0].ID, page2.Data[0].ID)

	// Invalid sort rejected.
	resp3 := authGET(t, ts.URL, "/api/sessions?sort=password_hash", token)
	defer resp3.Body.Close()
	assert.Equal(t, http.StatusUnprocessableEntity, resp3.StatusCode) // huma enum validation OR 400

	// Search
	resp4 := authGET(t, ts.URL, "/api/sessions?q=Sess%2001", token)
	defer resp4.Body.Close()
	require.Equal(t, http.StatusOK, resp4.StatusCode)
	var qPage struct {
		Data []struct {
			Name string `json:"name"`
		} `json:"data"`
		Total *int64 `json:"total"`
	}
	require.NoError(t, json.NewDecoder(resp4.Body).Decode(&qPage))
	require.Len(t, qPage.Data, 1)
	assert.Equal(t, "Sess 01", qPage.Data[0].Name)
	_ = ctx
}

func TestListSessions_TranslatorScopeNoLeak(t *testing.T) {
	ts, _, users := setupTestServer(t)
	ctx := context.Background()
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
		var s struct {
			ID string `json:"id"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&s))
		return s.ID
	}
	sidA := createSess("Assigned")
	_ = createSess("Hidden1")
	_ = createSess("Hidden2")

	hash, _ := auth.HashPassword("tr123")
	tr := &crosstalk.User{
		ID: ulid.Make().String(), Username: "scope-tr", PasswordHash: hash, Role: "translator",
	}
	require.NoError(t, users.Create(ctx, tr))
	require.NoError(t, users.AssignSessions(ctx, tr.ID, []string{sidA}))

	resp, err := http.Post(ts.URL+"/api/auth/login", "application/json",
		strings.NewReader(`{"username":"scope-tr","password":"tr123"}`))
	require.NoError(t, err)
	defer resp.Body.Close()
	var login struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&login))

	listResp := authGET(t, ts.URL, "/api/sessions?limit=100", login.AccessToken)
	defer listResp.Body.Close()
	require.Equal(t, http.StatusOK, listResp.StatusCode)
	var list struct {
		Data []struct {
			ID             string `json:"id"`
			BroadcastToken string `json:"broadcast_token"`
		} `json:"data"`
		Total *int64 `json:"total"`
	}
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&list))
	require.Len(t, list.Data, 1)
	assert.Equal(t, sidA, list.Data[0].ID)
	assert.Empty(t, list.Data[0].BroadcastToken)
	require.NotNil(t, list.Total)
	assert.Equal(t, int64(1), *list.Total, "total must not include unassigned sessions")
}

func TestListABCs_SessionNameAndBound(t *testing.T) {
	ts, _, users := setupTestServer(t)
	token := createAdminAndLogin(t, ts, users)
	ctx := context.Background()

	// Create session
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/sessions", strings.NewReader(`{"name":"Named Session"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	var sess struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&sess))
	_ = resp.Body.Close()

	// Create ABCs and assign via PUT
	var abcIDs []string
	for i := 0; i < 4; i++ {
		req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/abcs", strings.NewReader(fmt.Sprintf(`{"name":"Board %d"}`, i)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err = http.DefaultClient.Do(req)
		require.NoError(t, err)
		var created struct {
			ID string `json:"id"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
		_ = resp.Body.Close()
		abcIDs = append(abcIDs, created.ID)

		body := fmt.Sprintf(`{"name":"Board %d","session_id":"%s"}`, i, sess.ID)
		req, _ = http.NewRequest(http.MethodPut, ts.URL+"/api/abcs/"+created.ID, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err = http.DefaultClient.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		_ = resp.Body.Close()
	}

	resp = authGET(t, ts.URL, "/api/abcs?limit=2", token)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var page struct {
		Data []struct {
			ID          string  `json:"id"`
			SessionID   *string `json:"session_id"`
			SessionName string  `json:"session_name"`
		} `json:"data"`
		NextCursor string `json:"next_cursor"`
		Total      *int64 `json:"total"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&page))
	require.Len(t, page.Data, 2)
	require.NotEmpty(t, page.NextCursor)
	require.NotNil(t, page.Total)
	assert.Equal(t, int64(4), *page.Total)
	for _, row := range page.Data {
		assert.Equal(t, "Named Session", row.SessionName)
		require.NotNil(t, row.SessionID)
		assert.Equal(t, sess.ID, *row.SessionID)
	}
	_ = ctx
	_ = abcIDs
}

func TestListTranslators_BoundedWithSessionNames(t *testing.T) {
	ts, _, users := setupTestServer(t)
	token := createAdminAndLogin(t, ts, users)
	ctx := context.Background()

	// Session for names
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/sessions", strings.NewReader(`{"name":"Booth A"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	var sess struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&sess))
	_ = resp.Body.Close()

	for i := 0; i < 4; i++ {
		body := fmt.Sprintf(`{"username":"page-tr-%d","password":"pw"}`, i)
		req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/translators", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err = http.DefaultClient.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var tr struct {
			ID string `json:"id"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&tr))
		_ = resp.Body.Close()

		assign := fmt.Sprintf(`{"session_ids":["%s"]}`, sess.ID)
		req, _ = http.NewRequest(http.MethodPut, ts.URL+"/api/translators/"+tr.ID+"/sessions", strings.NewReader(assign))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err = http.DefaultClient.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		_ = resp.Body.Close()
	}

	resp = authGET(t, ts.URL, "/api/translators?limit=2", token)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var page struct {
		Data []struct {
			Username     string            `json:"username"`
			Sessions     []string          `json:"sessions"`
			SessionNames map[string]string `json:"session_names"`
		} `json:"data"`
		NextCursor string `json:"next_cursor"`
		Total      *int64 `json:"total"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&page))
	require.Len(t, page.Data, 2)
	require.NotEmpty(t, page.NextCursor)
	require.NotNil(t, page.Total)
	assert.Equal(t, int64(4), *page.Total)
	for _, row := range page.Data {
		require.NotEmpty(t, row.Sessions)
		assert.Equal(t, "Booth A", row.SessionNames[row.Sessions[0]])
	}
	_ = ctx
	_ = users
	_ = postgres.UserStore{}
}
