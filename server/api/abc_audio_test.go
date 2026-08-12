package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	crosstalk "github.com/aleksclark/crosstalk/server"
	"github.com/aleksclark/crosstalk/server/auth"
)

const (
	testOutUID = "usb:0d8c:0014:path:platform-xhci-0_1"
	testInUID  = "usb:0d8c:0014:path:platform-xhci-0_1"
	testOutUID2 = "usb:0d8c:0014:path:platform-xhci-0_2"
)

type abcAudioEnv struct {
	ts     *httptest.Server
	users  crosstalk.UserService
	abcs   crosstalk.ABCService
	audio  crosstalk.ABCAudioService
	admin  string // access token
	adminU *crosstalk.User
}

func setupABCAudioEnv(t *testing.T) *abcAudioEnv {
	t.Helper()
	ts, _, users, abcs, audio := setupTestServerFull(t)

	ctx := context.Background()
	hash, err := auth.HashPassword("admin123")
	require.NoError(t, err)
	admin := &crosstalk.User{
		ID:           ulid.Make().String(),
		Username:     "audio-admin-" + ulid.Make().String()[:8],
		PasswordHash: hash,
		Role:         "admin",
	}
	require.NoError(t, users.Create(ctx, admin))

	body := fmt.Sprintf(`{"username":%q,"password":"admin123"}`, admin.Username)
	resp, err := http.Post(ts.URL+"/api/auth/login", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var login struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&login))

	return &abcAudioEnv{
		ts:     ts,
		users:  users,
		abcs:   abcs,
		audio:  audio,
		admin:  login.AccessToken,
		adminU: admin,
	}
}

func (e *abcAudioEnv) createABC(t *testing.T) *crosstalk.ABC {
	t.Helper()
	abc := &crosstalk.ABC{
		ID:        ulid.Make().String(),
		Name:      "Audio Booth " + ulid.Make().String()[:6],
		TokenHash: "tok-" + ulid.Make().String(),
	}
	require.NoError(t, e.abcs.Create(context.Background(), abc))
	return abc
}

func (e *abcAudioEnv) seedCapabilities(t *testing.T, abcID string, caps []crosstalk.ABCAudioCapability) {
	t.Helper()
	outUID, inUID := "", ""
	for _, c := range caps {
		switch c.Direction {
		case "output":
			outUID = c.DeviceUID
		case "input":
			inUID = c.DeviceUID
		case "both", "":
			if outUID == "" {
				outUID = c.DeviceUID
			}
			if inUID == "" {
				inUID = c.DeviceUID
			}
		}
	}
	_, err := e.audio.RecordReport(context.Background(), abcID, crosstalk.ABCAudioObservation{
		DesiredRevision:   0,
		OutputDeviceUID:   outUID,
		InputDeviceUID:    inUID,
		OutputVolumeState: crosstalk.ABCAudioControlUnknown,
		OutputMuteState:   crosstalk.ABCAudioControlUnknown,
		InputGainState:    crosstalk.ABCAudioControlUnknown,
		Capabilities:      caps,
		ReportedAt:        time.Now().UTC(),
	})
	require.NoError(t, err)
}

func defaultCaps() []crosstalk.ABCAudioCapability {
	return []crosstalk.ABCAudioCapability{
		{
			DeviceUID:      testOutUID,
			Direction:      "both",
			Backend:        "alsa",
			SupportsVolume: true,
			SupportsMute:   true,
			SupportsGain:   true,
		},
	}
}

func putBody(requestID string, expectedRev uint64, vol, gain int, muted bool, outUID, inUID string) string {
	return fmt.Sprintf(`{
		"request_id": %q,
		"expected_revision": %d,
		"output": {
			"device_uid": %q,
			"volume_percent": %d,
			"muted": %t
		},
		"input": {
			"device_uid": %q,
			"gain_percent": %d
		}
	}`, requestID, expectedRev, outUID, vol, muted, inUID, gain)
}

func (e *abcAudioEnv) do(t *testing.T, method, path, token, body string) (int, map[string]any) {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, e.ts.URL+path, rdr)
	require.NoError(t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var out map[string]any
	if len(raw) > 0 && (raw[0] == '{' || raw[0] == '[') {
		require.NoError(t, json.Unmarshal(raw, &out), "body=%s", string(raw))
	}
	return resp.StatusCode, out
}

func (e *abcAudioEnv) translatorToken(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	hash, err := auth.HashPassword("tr123")
	require.NoError(t, err)
	tr := &crosstalk.User{
		ID:           ulid.Make().String(),
		Username:     "audio-tr-" + ulid.Make().String()[:8],
		PasswordHash: hash,
		Role:         "translator",
	}
	require.NoError(t, e.users.Create(ctx, tr))
	body := fmt.Sprintf(`{"username":%q,"password":"tr123"}`, tr.Username)
	resp, err := http.Post(e.ts.URL+"/api/auth/login", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var login struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&login))
	return login.AccessToken
}

func TestABCAudio_OpenAPIHasAudioSettings(t *testing.T) {
	e := setupABCAudioEnv(t)
	status, spec := e.do(t, http.MethodGet, "/api/openapi.json", "", "")
	require.Equal(t, http.StatusOK, status)
	paths, ok := spec["paths"].(map[string]any)
	require.True(t, ok)
	p, ok := paths["/api/abcs/{id}/audio-settings"].(map[string]any)
	require.True(t, ok, "path /api/abcs/{id}/audio-settings missing")
	_, hasGet := p["get"]
	_, hasPut := p["put"]
	assert.True(t, hasGet)
	assert.True(t, hasPut)

	// Schema validation bounds appear in components.
	comps, _ := spec["components"].(map[string]any)
	schemas, _ := comps["schemas"].(map[string]any)
	require.NotEmpty(t, schemas)
	// Find put body schema by walking put operation requestBody if needed;
	// ensure at least one schema mentions volume_percent or similar via dump.
	raw, _ := json.Marshal(spec)
	assert.Contains(t, string(raw), "volume_percent")
	assert.Contains(t, string(raw), "gain_percent")
	assert.Contains(t, string(raw), "request_id")
	assert.Contains(t, string(raw), "expected_revision")
	assert.Contains(t, string(raw), "overall_state")
	assert.Contains(t, string(raw), "get-abc-audio-settings")
	assert.Contains(t, string(raw), "put-abc-audio-settings")
}

func TestABCAudio_GetUnconfiguredAdmin(t *testing.T) {
	e := setupABCAudioEnv(t)
	abc := e.createABC(t)

	status, body := e.do(t, http.MethodGet, "/api/abcs/"+abc.ID+"/audio-settings", e.admin, "")
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, abc.ID, body["abc_id"])
	assert.Equal(t, false, body["connected"])
	assert.Equal(t, "unconfigured", body["overall_state"])
	desired, ok := body["desired"].(map[string]any)
	require.True(t, ok)
	assert.EqualValues(t, 0, desired["revision"])
	assert.EqualValues(t, 0, body["accepted_revision"])
}

func TestABCAudio_GetUnknownABC(t *testing.T) {
	e := setupABCAudioEnv(t)
	status, _ := e.do(t, http.MethodGet, "/api/abcs/01NONEXISTENT00000000000000/audio-settings", e.admin, "")
	assert.Equal(t, http.StatusNotFound, status)
}

func TestABCAudio_AuthMissingInvalidTranslator(t *testing.T) {
	e := setupABCAudioEnv(t)
	abc := e.createABC(t)
	path := "/api/abcs/" + abc.ID + "/audio-settings"
	body := putBody(ulid.Make().String(), 0, 50, 40, false, testOutUID, testInUID)

	status, _ := e.do(t, http.MethodGet, path, "", "")
	assert.Equal(t, http.StatusUnauthorized, status)

	status, _ = e.do(t, http.MethodGet, path, "not-a-jwt", "")
	assert.Equal(t, http.StatusUnauthorized, status)

	status, _ = e.do(t, http.MethodPut, path, "", body)
	assert.Equal(t, http.StatusUnauthorized, status)

	trTok := e.translatorToken(t)
	status, _ = e.do(t, http.MethodGet, path, trTok, "")
	assert.Equal(t, http.StatusForbidden, status)

	status, _ = e.do(t, http.MethodPut, path, trTok, body)
	assert.Equal(t, http.StatusForbidden, status)
}

func TestABCAudio_SchemaValidation(t *testing.T) {
	e := setupABCAudioEnv(t)
	abc := e.createABC(t)
	e.seedCapabilities(t, abc.ID, defaultCaps())
	path := "/api/abcs/" + abc.ID + "/audio-settings"

	// volume out of range
	badVol := putBody(ulid.Make().String(), 0, 101, 40, false, testOutUID, testInUID)
	status, _ := e.do(t, http.MethodPut, path, e.admin, badVol)
	assert.Equal(t, http.StatusUnprocessableEntity, status)

	// negative gain
	badGain := putBody(ulid.Make().String(), 0, 50, -1, false, testOutUID, testInUID)
	status, _ = e.do(t, http.MethodPut, path, e.admin, badGain)
	assert.Equal(t, http.StatusUnprocessableEntity, status)

	// empty request_id
	emptyReq := `{
		"request_id": "",
		"expected_revision": 0,
		"output": {"device_uid": "` + testOutUID + `", "volume_percent": 50, "muted": false},
		"input": {"device_uid": "` + testInUID + `", "gain_percent": 40}
	}`
	status, _ = e.do(t, http.MethodPut, path, e.admin, emptyReq)
	assert.Equal(t, http.StatusUnprocessableEntity, status)

	// empty device uid
	emptyUID := putBody(ulid.Make().String(), 0, 50, 40, false, "", testInUID)
	status, _ = e.do(t, http.MethodPut, path, e.admin, emptyUID)
	assert.Equal(t, http.StatusUnprocessableEntity, status)

	// missing required nested object
	missingOut := `{
		"request_id": "` + ulid.Make().String() + `",
		"expected_revision": 0,
		"input": {"device_uid": "` + testInUID + `", "gain_percent": 40}
	}`
	status, _ = e.do(t, http.MethodPut, path, e.admin, missingOut)
	assert.Equal(t, http.StatusUnprocessableEntity, status)
}

func TestABCAudio_PutNewRevision202(t *testing.T) {
	e := setupABCAudioEnv(t)
	abc := e.createABC(t)
	e.seedCapabilities(t, abc.ID, defaultCaps())
	path := "/api/abcs/" + abc.ID + "/audio-settings"
	reqID := ulid.Make().String()

	status, body := e.do(t, http.MethodPut, path, e.admin, putBody(reqID, 0, 65, 40, false, testOutUID, testInUID))
	require.Equal(t, http.StatusAccepted, status, "body=%v", body)
	assert.Equal(t, abc.ID, body["abc_id"])
	assert.EqualValues(t, 1, body["accepted_revision"])
	desired := body["desired"].(map[string]any)
	assert.EqualValues(t, 1, desired["revision"])
	assert.Equal(t, crosstalk.ABCAudioCommandID(abc.ID, 1), desired["command_id"])
	assert.EqualValues(t, 65, desired["output_volume_percent"])
	assert.Equal(t, false, desired["output_muted"])
	assert.EqualValues(t, 40, desired["input_gain_percent"])
	assert.Equal(t, testOutUID, desired["output_device_uid"])
	assert.Equal(t, "offline", body["overall_state"]) // not connected
	reported := body["reported"].(map[string]any)
	assert.Equal(t, "pending", reported["output_volume_state"])
}

func TestABCAudio_PutNoOpAndDuplicate200(t *testing.T) {
	e := setupABCAudioEnv(t)
	abc := e.createABC(t)
	e.seedCapabilities(t, abc.ID, defaultCaps())
	path := "/api/abcs/" + abc.ID + "/audio-settings"

	req1 := ulid.Make().String()
	status, body := e.do(t, http.MethodPut, path, e.admin, putBody(req1, 0, 50, 30, true, testOutUID, testInUID))
	require.Equal(t, http.StatusAccepted, status)
	require.EqualValues(t, 1, body["accepted_revision"])

	// Duplicate same request_id → 200, same revision
	status, body = e.do(t, http.MethodPut, path, e.admin, putBody(req1, 0, 50, 30, true, testOutUID, testInUID))
	require.Equal(t, http.StatusOK, status, "body=%v", body)
	assert.EqualValues(t, 1, body["accepted_revision"])
	assert.EqualValues(t, 1, body["desired"].(map[string]any)["revision"])

	// Byte-equal no-op with new request_id → 200, no revision bump
	req2 := ulid.Make().String()
	status, body = e.do(t, http.MethodPut, path, e.admin, putBody(req2, 1, 50, 30, true, testOutUID, testInUID))
	require.Equal(t, http.StatusOK, status, "body=%v", body)
	assert.EqualValues(t, 1, body["accepted_revision"])
	assert.EqualValues(t, 1, body["desired"].(map[string]any)["revision"])
}

func TestABCAudio_PutRevisionConflict409(t *testing.T) {
	e := setupABCAudioEnv(t)
	abc := e.createABC(t)
	e.seedCapabilities(t, abc.ID, defaultCaps())
	path := "/api/abcs/" + abc.ID + "/audio-settings"

	status, _ := e.do(t, http.MethodPut, path, e.admin, putBody(ulid.Make().String(), 0, 50, 30, false, testOutUID, testInUID))
	require.Equal(t, http.StatusAccepted, status)

	status, body := e.do(t, http.MethodPut, path, e.admin, putBody(ulid.Make().String(), 0, 60, 30, false, testOutUID, testInUID))
	assert.Equal(t, http.StatusConflict, status, "body=%v", body)

	status, body = e.do(t, http.MethodPut, path, e.admin, putBody(ulid.Make().String(), 99, 60, 30, false, testOutUID, testInUID))
	assert.Equal(t, http.StatusConflict, status, "body=%v", body)
}

func TestABCAudio_InitialCapabilityBindingRequired(t *testing.T) {
	e := setupABCAudioEnv(t)
	abc := e.createABC(t)
	// No inventory/capabilities seeded.
	path := "/api/abcs/" + abc.ID + "/audio-settings"

	status, body := e.do(t, http.MethodPut, path, e.admin, putBody(ulid.Make().String(), 0, 50, 40, false, testOutUID, testInUID))
	assert.Equal(t, http.StatusConflict, status, "body=%v", body)

	// Inventory without matching UIDs still conflicts.
	e.seedCapabilities(t, abc.ID, []crosstalk.ABCAudioCapability{
		{DeviceUID: "usb:ffff:ffff:path:other", Direction: "both", Backend: "alsa", SupportsVolume: true, SupportsMute: true, SupportsGain: true},
	})
	status, body = e.do(t, http.MethodPut, path, e.admin, putBody(ulid.Make().String(), 0, 50, 40, false, testOutUID, testInUID))
	assert.Equal(t, http.StatusConflict, status, "body=%v", body)

	// Matching caps allow first save.
	e.seedCapabilities(t, abc.ID, defaultCaps())
	status, body = e.do(t, http.MethodPut, path, e.admin, putBody(ulid.Make().String(), 0, 50, 40, false, testOutUID, testInUID))
	require.Equal(t, http.StatusAccepted, status, "body=%v", body)
}

func TestABCAudio_OfflineSameDeviceUpdate(t *testing.T) {
	e := setupABCAudioEnv(t)
	abc := e.createABC(t)
	e.seedCapabilities(t, abc.ID, defaultCaps())
	path := "/api/abcs/" + abc.ID + "/audio-settings"

	status, body := e.do(t, http.MethodPut, path, e.admin, putBody(ulid.Make().String(), 0, 50, 40, false, testOutUID, testInUID))
	require.Equal(t, http.StatusAccepted, status, "body=%v", body)

	// Clear live capability evidence? Offline edits may target already-bound UIDs
	// even without fresh inventory matching — simulate by overwriting caps with empty
	// via a report that has empty capabilities but keeps device UIDs.
	_, err := e.audio.RecordReport(context.Background(), abc.ID, crosstalk.ABCAudioObservation{
		DesiredRevision:   0,
		OutputDeviceUID:   testOutUID,
		InputDeviceUID:    testInUID,
		OutputVolumeState: crosstalk.ABCAudioControlUnknown,
		OutputMuteState:   crosstalk.ABCAudioControlUnknown,
		InputGainState:    crosstalk.ABCAudioControlUnknown,
		Capabilities:      nil,
		ReportedAt:        time.Now().UTC(),
	})
	require.NoError(t, err)

	status, body = e.do(t, http.MethodPut, path, e.admin, putBody(ulid.Make().String(), 1, 70, 45, true, testOutUID, testInUID))
	require.Equal(t, http.StatusAccepted, status, "body=%v", body)
	assert.EqualValues(t, 2, body["accepted_revision"])
	desired := body["desired"].(map[string]any)
	assert.EqualValues(t, 70, desired["output_volume_percent"])
	assert.Equal(t, true, desired["output_muted"])
}

func TestABCAudio_RebindRequiresCapabilityEvidence(t *testing.T) {
	e := setupABCAudioEnv(t)
	abc := e.createABC(t)
	e.seedCapabilities(t, abc.ID, defaultCaps())
	path := "/api/abcs/" + abc.ID + "/audio-settings"

	status, _ := e.do(t, http.MethodPut, path, e.admin, putBody(ulid.Make().String(), 0, 50, 40, false, testOutUID, testInUID))
	require.Equal(t, http.StatusAccepted, status)

	// Attempt rebind to unknown UID without capability evidence → 409
	status, body := e.do(t, http.MethodPut, path, e.admin, putBody(ulid.Make().String(), 1, 50, 40, false, testOutUID2, testInUID))
	assert.Equal(t, http.StatusConflict, status, "body=%v", body)

	// Report new device capability, then rebind succeeds.
	e.seedCapabilities(t, abc.ID, []crosstalk.ABCAudioCapability{
		{DeviceUID: testOutUID, Direction: "both", Backend: "alsa", SupportsVolume: true, SupportsMute: true, SupportsGain: true},
		{DeviceUID: testOutUID2, Direction: "both", Backend: "alsa", SupportsVolume: true, SupportsMute: true, SupportsGain: true},
	})
	status, body = e.do(t, http.MethodPut, path, e.admin, putBody(ulid.Make().String(), 1, 55, 42, false, testOutUID2, testOutUID2))
	require.Equal(t, http.StatusAccepted, status, "body=%v", body)
	assert.Equal(t, testOutUID2, body["desired"].(map[string]any)["output_device_uid"])
}

func TestABCAudio_AuditActorFromJWTOnly(t *testing.T) {
	e := setupABCAudioEnv(t)
	abc := e.createABC(t)
	e.seedCapabilities(t, abc.ID, defaultCaps())
	path := "/api/abcs/" + abc.ID + "/audio-settings"

	// Unknown smuggled fields (actor/command/argv/mixer/binary) are rejected by schema.
	smuggle := fmt.Sprintf(`{
		"request_id": %q,
		"expected_revision": 0,
		"actor_user_id": "forged-user",
		"actor_role": "superadmin",
		"abc_id": "forged-abc",
		"command": "amixer",
		"argv": ["sset", "Mic", "100%%"],
		"output": {"device_uid": %q, "volume_percent": 50, "muted": false, "mixer_name": "Speaker"},
		"input": {"device_uid": %q, "gain_percent": 40, "binary": "/bin/sh"}
	}`, ulid.Make().String(), testOutUID, testInUID)
	status, _ := e.do(t, http.MethodPut, path, e.admin, smuggle)
	assert.Equal(t, http.StatusUnprocessableEntity, status)

	// Valid body: audit actor is JWT subject/role only.
	reqID := ulid.Make().String()
	status, body := e.do(t, http.MethodPut, path, e.admin, putBody(reqID, 0, 50, 40, false, testOutUID, testInUID))
	require.Equal(t, http.StatusAccepted, status, "body=%v", body)

	events, err := e.audio.ListAudit(context.Background(), abc.ID, 10)
	require.NoError(t, err)
	require.NotEmpty(t, events)
	ev := events[0]
	assert.Equal(t, reqID, ev.RequestID)
	assert.Equal(t, e.adminU.ID, ev.ActorUserID)
	assert.Equal(t, "admin", ev.ActorRole)
	assert.NotEqual(t, "forged-user", ev.ActorUserID)
	assert.Equal(t, crosstalk.ABCAudioAuditAccepted, ev.Outcome)
}

func TestABCAudio_GetAfterPut(t *testing.T) {
	e := setupABCAudioEnv(t)
	abc := e.createABC(t)
	e.seedCapabilities(t, abc.ID, defaultCaps())
	path := "/api/abcs/" + abc.ID + "/audio-settings"

	status, _ := e.do(t, http.MethodPut, path, e.admin, putBody(ulid.Make().String(), 0, 80, 25, true, testOutUID, testInUID))
	require.Equal(t, http.StatusAccepted, status)

	status, body := e.do(t, http.MethodGet, path, e.admin, "")
	require.Equal(t, http.StatusOK, status)
	desired := body["desired"].(map[string]any)
	assert.EqualValues(t, 1, desired["revision"])
	assert.EqualValues(t, 80, desired["output_volume_percent"])
	assert.Equal(t, true, desired["output_muted"])
	assert.EqualValues(t, 25, desired["input_gain_percent"])
	assert.Contains(t, body, "stale")
	assert.Contains(t, body, "reported")
	assert.Contains(t, body, "overall_state")
}

func TestABCAudio_PutUnknownABC(t *testing.T) {
	e := setupABCAudioEnv(t)
	path := "/api/abcs/01NONEXISTENT00000000000000/audio-settings"
	status, _ := e.do(t, http.MethodPut, path, e.admin, putBody(ulid.Make().String(), 0, 50, 40, false, testOutUID, testInUID))
	assert.Equal(t, http.StatusNotFound, status)
}

func TestABCAudio_TranslatorCannotWriteEvenAssigned(t *testing.T) {
	e := setupABCAudioEnv(t)
	ctx := context.Background()
	// Create session + assign translator + abc on session — still 403 for audio.
	sessStore := e.users // only need create via HTTP
	_ = sessStore
	status, sessBody := e.do(t, http.MethodPost, "/api/sessions", e.admin, `{"name":"audio-sess"}`)
	require.Equal(t, http.StatusOK, status)
	sid, _ := sessBody["id"].(string)
	require.NotEmpty(t, sid)

	abc := e.createABC(t)
	// assign abc to session via admin update
	upd := fmt.Sprintf(`{"session_id":%q}`, sid)
	status, _ = e.do(t, http.MethodPut, "/api/abcs/"+abc.ID, e.admin, upd)
	require.Equal(t, http.StatusOK, status)

	trTok := e.translatorToken(t)
	// find translator id from login not needed — assign by creating user again
	// Re-create translator and assign properly:
	hash, _ := auth.HashPassword("trX")
	tr := &crosstalk.User{
		ID:           ulid.Make().String(),
		Username:     "audio-tr-asg-" + ulid.Make().String()[:6],
		PasswordHash: hash,
		Role:         "translator",
	}
	require.NoError(t, e.users.Create(ctx, tr))
	require.NoError(t, e.users.AssignSessions(ctx, tr.ID, []string{sid}))
	loginBody := fmt.Sprintf(`{"username":%q,"password":"trX"}`, tr.Username)
	resp, err := http.Post(e.ts.URL+"/api/auth/login", "application/json", strings.NewReader(loginBody))
	require.NoError(t, err)
	defer resp.Body.Close()
	var login struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&login))
	_ = trTok

	e.seedCapabilities(t, abc.ID, defaultCaps())
	path := "/api/abcs/" + abc.ID + "/audio-settings"
	status, _ = e.do(t, http.MethodPut, path, login.AccessToken, putBody(ulid.Make().String(), 0, 50, 40, false, testOutUID, testInUID))
	assert.Equal(t, http.StatusForbidden, status)
	status, _ = e.do(t, http.MethodGet, path, login.AccessToken, "")
	assert.Equal(t, http.StatusForbidden, status)
}
