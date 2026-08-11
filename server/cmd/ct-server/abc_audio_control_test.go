package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	pionwebrtc "github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"nhooyr.io/websocket"

	crosstalkv2 "github.com/aleksclark/crosstalk/server/proto/v2"
	"github.com/aleksclark/crosstalk/server/webrtc"
)

const audioTestUID = "usb:0d8c:0014:path:platform-xhci-0_1"

// TestIntegrationABCAudioControl_CapabilitiesCommandApplied proves the full
// REST → control-channel command → report → GET applied path.
func TestIntegrationABCAudioControl_CapabilitiesCommandApplied(t *testing.T) {
	env := setupIntegrationServer(t)
	adminToken, abc := provisionAudioABC(t, env, "Audio Booth")

	client, _ := newABCClient(t, env, abc.Token)
	client.sendAudioReport(t, inventoryReport(audioTestUID))
	waitCaps(t, env, adminToken, abc.ID)

	code, putBody := putAudioSettings(t, env, adminToken, abc.ID, ulid.Make().String(), 0, 65, 40, false)
	require.Equal(t, http.StatusAccepted, code, "body=%v", putBody)
	assert.Equal(t, float64(1), putBody["accepted_revision"])
	desired := putBody["desired"].(map[string]any)
	assert.Equal(t, float64(1), desired["revision"])
	cmdID := desired["command_id"].(string)
	require.NotEmpty(t, cmdID)

	cmd := client.waitAudioCommand(t, 8*time.Second)
	require.NotNil(t, cmd)
	assert.Equal(t, uint64(1), cmd.GetDesiredRevision())
	assert.Equal(t, cmdID, cmd.GetCommandId())
	assert.Equal(t, audioTestUID, cmd.GetOutput().GetDeviceUid())
	assert.Equal(t, uint32(65), cmd.GetOutput().GetVolumePercent())
	assert.Equal(t, uint32(40), cmd.GetInput().GetGainPercent())

	client.sendAudioReport(t, appliedReport(cmd, 65, 40, false))

	require.Eventually(t, func() bool {
		st := getAudioSettings(t, env, adminToken, abc.ID)
		return st["overall_state"] == "applied" &&
			st["reported"].(map[string]any)["revision"] == float64(1) &&
			st["reported"].(map[string]any)["output_volume_state"] == "applied"
	}, 5*time.Second, 50*time.Millisecond, "GET should transition pending→applied")
}

// TestIntegrationABCAudioControl_OfflinePutThenConnect: desired is durable while
// offline; connect+Hello/control open pushes the command.
func TestIntegrationABCAudioControl_OfflinePutThenConnect(t *testing.T) {
	env := setupIntegrationServer(t)
	adminToken, abc := provisionAudioABC(t, env, "Offline Booth")

	seed, _ := newABCClient(t, env, abc.Token)
	seed.sendAudioReport(t, inventoryReport(audioTestUID))
	waitCaps(t, env, adminToken, abc.ID)
	_ = seed.pc.Close()
	_ = seed.ws.Close(websocket.StatusNormalClosure, "")
	time.Sleep(200 * time.Millisecond)

	code, body := putAudioSettings(t, env, adminToken, abc.ID, ulid.Make().String(), 0, 70, 30, true)
	require.Equal(t, http.StatusAccepted, code, "body=%v", body)
	assert.Equal(t, float64(1), body["accepted_revision"])
	assert.Equal(t, float64(1), body["desired"].(map[string]any)["revision"])
	// overall may be offline or pending depending on how quickly connected clears;
	// durable desired is what matters for offline queue semantics.
	assert.Contains(t, []string{"offline", "pending"}, body["overall_state"])

	// Reconnect: reconcile on control open must deliver command.
	client, _ := newABCClient(t, env, abc.Token)
	cmd := client.waitAudioCommand(t, 10*time.Second)
	require.Equal(t, uint64(1), cmd.GetDesiredRevision())
	assert.Equal(t, uint32(70), cmd.GetOutput().GetVolumePercent())
	assert.True(t, cmd.GetOutput().GetMuted())
	assert.Equal(t, uint32(30), cmd.GetInput().GetGainPercent())
}

// TestIntegrationABCAudioControl_HeartbeatConvergence: inventory heartbeat
// triggers reconcile when desired is still pending.
func TestIntegrationABCAudioControl_HeartbeatConvergence(t *testing.T) {
	env := setupIntegrationServer(t)
	adminToken, abc := provisionAudioABC(t, env, "Heartbeat Booth")

	client, _ := newABCClient(t, env, abc.Token)
	client.sendAudioReport(t, inventoryReport(audioTestUID))
	waitCaps(t, env, adminToken, abc.ID)
	drainAudioCommands(client, 500*time.Millisecond)

	code, _ := putAudioSettings(t, env, adminToken, abc.ID, ulid.Make().String(), 0, 55, 22, false)
	require.Equal(t, http.StatusAccepted, code)

	// Drop any immediate push, then prove heartbeat inventory re-converges.
	drainAudioCommands(client, 200*time.Millisecond)
	client.sendAudioReport(t, inventoryReport(audioTestUID))
	cmd := client.waitAudioCommand(t, 8*time.Second)
	require.Equal(t, uint64(1), cmd.GetDesiredRevision())
	assert.Equal(t, uint32(55), cmd.GetOutput().GetVolumePercent())
}

// TestIntegrationABCAudioControl_DuplicateRequestID: second identical request_id
// is 200 and does not create a second revision.
func TestIntegrationABCAudioControl_DuplicateRequestID(t *testing.T) {
	env := setupIntegrationServer(t)
	adminToken, abc := provisionAudioABC(t, env, "Dup Booth")

	client, _ := newABCClient(t, env, abc.Token)
	client.sendAudioReport(t, inventoryReport(audioTestUID))
	waitCaps(t, env, adminToken, abc.ID)

	reqID := ulid.Make().String()
	code1, body1 := putAudioSettings(t, env, adminToken, abc.ID, reqID, 0, 60, 25, false)
	require.Equal(t, http.StatusAccepted, code1)
	code2, body2 := putAudioSettings(t, env, adminToken, abc.ID, reqID, 0, 99, 99, true)
	require.Equal(t, http.StatusOK, code2)
	assert.Equal(t, body1["accepted_revision"], body2["accepted_revision"])
	assert.Equal(t, float64(1), body2["desired"].(map[string]any)["revision"])
}

// TestIntegrationABCAudioControl_StaleReportIgnored: older report cannot roll
// back a newer desired/reported revision.
func TestIntegrationABCAudioControl_StaleReportIgnored(t *testing.T) {
	env := setupIntegrationServer(t)
	adminToken, abc := provisionAudioABC(t, env, "Stale Booth")

	client, _ := newABCClient(t, env, abc.Token)
	client.sendAudioReport(t, inventoryReport(audioTestUID))
	waitCaps(t, env, adminToken, abc.ID)

	code, _ := putAudioSettings(t, env, adminToken, abc.ID, ulid.Make().String(), 0, 10, 10, false)
	require.Equal(t, http.StatusAccepted, code)
	cmd1 := client.waitAudioCommand(t, 8*time.Second)
	require.Equal(t, uint64(1), cmd1.GetDesiredRevision())

	code, _ = putAudioSettings(t, env, adminToken, abc.ID, ulid.Make().String(), 1, 80, 50, false)
	require.Equal(t, http.StatusAccepted, code)
	cmd2 := client.waitAudioCommand(t, 8*time.Second)
	require.Equal(t, uint64(2), cmd2.GetDesiredRevision())

	client.sendAudioReport(t, appliedReport(cmd2, 80, 50, false))
	require.Eventually(t, func() bool {
		st := getAudioSettings(t, env, adminToken, abc.ID)
		return st["reported"].(map[string]any)["revision"] == float64(2)
	}, 5*time.Second, 50*time.Millisecond)

	client.sendAudioReport(t, appliedReport(cmd1, 10, 10, false))
	time.Sleep(300 * time.Millisecond)
	st := getAudioSettings(t, env, adminToken, abc.ID)
	assert.Equal(t, float64(2), st["reported"].(map[string]any)["revision"])
	assert.Equal(t, float64(2), st["desired"].(map[string]any)["revision"])
	assert.Equal(t, "applied", st["overall_state"])
}

// TestIntegrationABCAudioControl_WrongABCCannotMutateOther: report on ABC B's
// authenticated channel cannot apply ABC A's desired.
func TestIntegrationABCAudioControl_WrongABCCannotMutateOther(t *testing.T) {
	env := setupIntegrationServer(t)
	adminUser := "admin-" + ulid.Make().String()
	env.createAdminUser(t, adminUser, "admin-pass-123")
	adminToken := env.login(t, adminUser, "admin-pass-123")

	session := createSession(t, env, adminToken, "Audio Multi")
	createChannel(t, env, adminToken, session.ID, "Floor Feed", "feed")

	abcA := createAssignedABC(t, env, adminToken, session.ID, "ABC-A")
	abcB := createAssignedABC(t, env, adminToken, session.ID, "ABC-B")

	clientA, _ := newABCClient(t, env, abcA.Token)
	clientB, _ := newABCClient(t, env, abcB.Token)

	clientA.sendAudioReport(t, inventoryReport(audioTestUID))
	clientB.sendAudioReport(t, inventoryReport(audioTestUID+"-b"))
	waitCaps(t, env, adminToken, abcA.ID)
	waitCaps(t, env, adminToken, abcB.ID)

	code, body := putAudioSettings(t, env, adminToken, abcA.ID, ulid.Make().String(), 0, 33, 44, false)
	require.Equal(t, http.StatusAccepted, code, "body=%v", body)
	cmd := clientA.waitAudioCommand(t, 8*time.Second)

	// B "acks" A's command over B's channel — must not make A applied.
	clientB.sendAudioReport(t, appliedReport(cmd, 33, 44, false))
	time.Sleep(400 * time.Millisecond)

	stA := getAudioSettings(t, env, adminToken, abcA.ID)
	assert.NotEqual(t, "applied", stA["overall_state"])
	assert.Equal(t, float64(1), stA["desired"].(map[string]any)["revision"])
	stB := getAudioSettings(t, env, adminToken, abcB.ID)
	assert.Equal(t, float64(0), stB["desired"].(map[string]any)["revision"])
}

// TestIntegrationABCAudioControl_MalformedAndOversizedRejected: bad reports
// do not mutate durable state.
func TestIntegrationABCAudioControl_MalformedAndOversizedRejected(t *testing.T) {
	env := setupIntegrationServer(t)
	adminToken, abc := provisionAudioABC(t, env, "BadReport Booth")

	client, _ := newABCClient(t, env, abc.Token)
	client.sendAudioReport(t, inventoryReport(audioTestUID))
	waitCaps(t, env, adminToken, abc.ID)
	before := getAudioSettings(t, env, adminToken, abc.ID)

	// Oversized raw frame (control handler rejects before API).
	oversized := make([]byte, webrtc.MaxControlMessageBytes+8)
	require.NotNil(t, client.dc)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if client.dc.ReadyState() == pionwebrtc.DataChannelStateOpen {
			_ = client.dc.Send(oversized)
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = client.dc.Send([]byte{0xde, 0xad, 0xbe, 0xef})

	// Over device limit — rejected by mapper.
	devs := make([]*crosstalkv2.AudioDeviceCapability, 9)
	for i := range devs {
		devs[i] = &crosstalkv2.AudioDeviceCapability{DeviceUid: fmt.Sprintf("usb:d%d", i)}
	}
	client.sendAudioReport(t, &crosstalkv2.AudioControlReport{
		DesiredRevision: 0,
		Devices:         devs,
	})

	time.Sleep(400 * time.Millisecond)
	after := getAudioSettings(t, env, adminToken, abc.ID)
	beforeCaps := before["reported"].(map[string]any)["capabilities"].([]any)
	afterCaps := after["reported"].(map[string]any)["capabilities"].([]any)
	assert.Equal(t, len(beforeCaps), len(afterCaps))
	assert.Equal(t, float64(0), after["desired"].(map[string]any)["revision"])
}

// TestIntegrationABCAudioControl_DisconnectReconnectGenerationRace: old peer
// close must not wipe a newer registration; reconnect still receives command.
func TestIntegrationABCAudioControl_DisconnectReconnectGenerationRace(t *testing.T) {
	env := setupIntegrationServer(t)
	adminToken, abc := provisionAudioABC(t, env, "GenRace Booth")

	c1, _ := newABCClient(t, env, abc.Token)
	c1.sendAudioReport(t, inventoryReport(audioTestUID))
	waitCaps(t, env, adminToken, abc.ID)

	c2, _ := newABCClient(t, env, abc.Token)
	_ = c1.pc.Close()
	time.Sleep(200 * time.Millisecond)

	code, _ := putAudioSettings(t, env, adminToken, abc.ID, ulid.Make().String(), 0, 41, 42, false)
	require.Equal(t, http.StatusAccepted, code)

	cmd := c2.waitAudioCommand(t, 10*time.Second)
	require.Equal(t, uint64(1), cmd.GetDesiredRevision())
	assert.Equal(t, uint32(41), cmd.GetOutput().GetVolumePercent())
}

// --- helpers ----------------------------------------------------------------

type provisionedABC struct {
	ID    string
	Token string
}

func provisionAudioABC(t *testing.T, env *testEnv, abcName string) (string, provisionedABC) {
	t.Helper()
	adminUser := "admin-" + ulid.Make().String()
	env.createAdminUser(t, adminUser, "admin-pass-123")
	token := env.login(t, adminUser, "admin-pass-123")

	session := createSession(t, env, token, "Audio Session "+ulid.Make().String()[:6])
	createChannel(t, env, token, session.ID, "Floor Feed", "feed")

	abc := createAssignedABC(t, env, token, session.ID, abcName)
	return token, abc
}

func createAssignedABC(t *testing.T, env *testEnv, token, sessionID, abcName string) provisionedABC {
	t.Helper()
	resp := env.doRequest(t, http.MethodPost, "/api/abcs", token, fmt.Sprintf(`{"name":%q}`, abcName))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var created struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	resp.Body.Close()
	require.NotEmpty(t, created.Token)

	resp = env.doRequest(t, http.MethodPut, "/api/abcs/"+created.ID, token,
		fmt.Sprintf(`{"name":%q,"session_id":%q}`, abcName, sessionID))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	return provisionedABC{ID: created.ID, Token: created.Token}
}

func inventoryReport(uid string) *crosstalkv2.AudioControlReport {
	return &crosstalkv2.AudioControlReport{
		DesiredRevision: 0,
		Devices: []*crosstalkv2.AudioDeviceCapability{
			{
				DeviceUid:      uid,
				Direction:      "both",
				Backend:        "alsa",
				SupportsVolume: true,
				SupportsMute:   true,
				SupportsGain:   true,
			},
		},
	}
}

func appliedReport(cmd *crosstalkv2.AudioControlCommand, vol, gain int, muted bool) *crosstalkv2.AudioControlReport {
	v := uint32(vol)
	g := uint32(gain)
	m := muted
	outUID := audioTestUID
	inUID := audioTestUID
	if cmd.GetOutput() != nil && cmd.GetOutput().GetDeviceUid() != "" {
		outUID = cmd.GetOutput().GetDeviceUid()
	}
	if cmd.GetInput() != nil && cmd.GetInput().GetDeviceUid() != "" {
		inUID = cmd.GetInput().GetDeviceUid()
	}
	return &crosstalkv2.AudioControlReport{
		CommandId:       cmd.GetCommandId(),
		DesiredRevision: cmd.GetDesiredRevision(),
		Devices: []*crosstalkv2.AudioDeviceCapability{
			{
				DeviceUid:      outUID,
				Direction:      "both",
				Backend:        "alsa",
				SupportsVolume: true,
				SupportsMute:   true,
				SupportsGain:   true,
			},
		},
		Output: &crosstalkv2.AudioOutputObserved{
			DeviceUid:     outUID,
			VolumePercent: &v,
			Muted:         &m,
			VolumeState:   crosstalkv2.AudioApplyState_AUDIO_APPLY_STATE_APPLIED,
			MuteState:     crosstalkv2.AudioApplyState_AUDIO_APPLY_STATE_APPLIED,
		},
		Input: &crosstalkv2.AudioInputObserved{
			DeviceUid:   inUID,
			GainPercent: &g,
			GainState:   crosstalkv2.AudioApplyState_AUDIO_APPLY_STATE_APPLIED,
		},
	}
}

func putAudioSettings(t *testing.T, env *testEnv, token, abcID, requestID string, expectedRev uint64, vol, gain int, muted bool) (int, map[string]any) {
	t.Helper()
	body := fmt.Sprintf(`{
		"request_id": %q,
		"expected_revision": %d,
		"output": {"device_uid": %q, "volume_percent": %d, "muted": %t},
		"input": {"device_uid": %q, "gain_percent": %d}
	}`, requestID, expectedRev, audioTestUID, vol, muted, audioTestUID, gain)
	resp := env.doRequest(t, http.MethodPut, "/api/abcs/"+abcID+"/audio-settings", token, body)
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var out map[string]any
	if len(raw) > 0 {
		require.NoError(t, json.Unmarshal(raw, &out), "body=%s", string(raw))
	}
	return resp.StatusCode, out
}

func getAudioSettings(t *testing.T, env *testEnv, token, abcID string) map[string]any {
	t.Helper()
	resp := env.doRequest(t, http.MethodGet, "/api/abcs/"+abcID+"/audio-settings", token, "")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	return out
}

func waitCaps(t *testing.T, env *testEnv, token, abcID string) {
	t.Helper()
	require.Eventually(t, func() bool {
		st := getAudioSettings(t, env, token, abcID)
		reported, _ := st["reported"].(map[string]any)
		if reported == nil {
			return false
		}
		caps, _ := reported["capabilities"].([]any)
		return len(caps) > 0
	}, 5*time.Second, 50*time.Millisecond)
}

func drainAudioCommands(ac *abcClient, d time.Duration) {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		select {
		case <-ac.cmdWait:
		case <-time.After(20 * time.Millisecond):
		}
	}
}
