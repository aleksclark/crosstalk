package portal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	crosstalk "github.com/aleksclark/crosstalk/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleRootServesUI(t *testing.T) {
	s := New(":0")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.handleRoot(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "CrossTalk Box Setup")
	assert.Contains(t, rec.Header().Get("Content-Type"), "text/html")
}

func TestHandleRootRedirectsProbes(t *testing.T) {
	s := New(":0")

	for _, path := range []string{"/generate_204", "/hotspot-detect.html", "/ncsi.txt"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		s.handleRoot(rec, req)
		assert.Equal(t, http.StatusFound, rec.Code, path)
		assert.Equal(t, "/", rec.Header().Get("Location"), path)
	}
}

func TestHandleScanReturnsCachedNetworks(t *testing.T) {
	s := New(":0")
	s.SetNetworks([]crosstalk.WiFiNetwork{
		{SSID: "HomeNet", Signal: 80, Secured: true},
	})

	req := httptest.NewRequest(http.MethodGet, "/scan", nil)
	rec := httptest.NewRecorder()
	s.handleScan(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Networks []crosstalk.WiFiNetwork `json:"networks"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Networks, 1)
	assert.Equal(t, "HomeNet", body.Networks[0].SSID)
}

func TestHandleScanEmptyWhenNoNetworksCached(t *testing.T) {
	s := New(":0")

	req := httptest.NewRequest(http.MethodGet, "/scan", nil)
	rec := httptest.NewRecorder()
	s.handleScan(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Networks []crosstalk.WiFiNetwork `json:"networks"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Empty(t, body.Networks)
}

func TestHandleConnectSignalsCredentials(t *testing.T) {
	s := New(":0")

	body := strings.NewReader(`{"ssid":"HomeNet","passphrase":"secret"}`)
	req := httptest.NewRequest(http.MethodPost, "/connect", body)
	rec := httptest.NewRecorder()
	s.handleConnect(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	select {
	case creds := <-s.provisioned:
		assert.Equal(t, "HomeNet", creds.SSID)
		assert.Equal(t, "secret", creds.Passphrase)
	case <-time.After(2 * time.Second):
		t.Fatal("credentials were not signalled")
	}
}

func TestHandleConnectDuplicateSubmitIdempotent(t *testing.T) {
	s := New(":0")

	body1 := strings.NewReader(`{"ssid":"HomeNet","passphrase":"first"}`)
	req1 := httptest.NewRequest(http.MethodPost, "/connect", body1)
	rec1 := httptest.NewRecorder()
	s.handleConnect(rec1, req1)
	assert.Equal(t, http.StatusOK, rec1.Code)

	// Second submit with different passphrase must not overwrite.
	body2 := strings.NewReader(`{"ssid":"Other","passphrase":"second"}`)
	req2 := httptest.NewRequest(http.MethodPost, "/connect", body2)
	rec2 := httptest.NewRecorder()
	s.handleConnect(rec2, req2)
	assert.Equal(t, http.StatusOK, rec2.Code)

	var resp struct {
		Status string `json:"status"`
		SSID   string `json:"ssid"`
	}
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp))
	assert.Equal(t, "connecting", resp.Status)
	assert.Equal(t, "HomeNet", resp.SSID) // first wins

	select {
	case creds := <-s.provisioned:
		assert.Equal(t, "HomeNet", creds.SSID)
		assert.Equal(t, "first", creds.Passphrase)
	case <-time.After(2 * time.Second):
		t.Fatal("credentials were not signalled")
	}

	// Channel must be empty (no second value).
	select {
	case extra := <-s.provisioned:
		t.Fatalf("duplicate submit queued extra credentials: %+v", extra)
	default:
	}
}

func TestHandleConnectRejectsMissingSSID(t *testing.T) {
	s := New(":0")
	req := httptest.NewRequest(http.MethodPost, "/connect", strings.NewReader(`{"ssid":""}`))
	rec := httptest.NewRecorder()
	s.handleConnect(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleConnectRejectsGET(t *testing.T) {
	s := New(":0")
	req := httptest.NewRequest(http.MethodGet, "/connect", nil)
	rec := httptest.NewRecorder()
	s.handleConnect(rec, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}
