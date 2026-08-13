package api

import (
	"time"
	"io"
	"log/slog"
	"testing"

	"github.com/pion/ice/v4"
	pionwebrtc "github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	crosstalk "github.com/aleksclark/crosstalk/server"
	crosstalkv2 "github.com/aleksclark/crosstalk/server/proto/v2"
	"github.com/aleksclark/crosstalk/server/webrtc"
)

func TestMapAudioControlReport_Applied(t *testing.T) {
	vol := uint32(65)
	muted := false
	gain := uint32(40)
	report := &crosstalkv2.AudioControlReport{
		CommandId:       "abc-audio/abc1/2",
		DesiredRevision: 2,
		Devices: []*crosstalkv2.AudioDeviceCapability{
			{
				DeviceUid:      "usb:0d8c:0014:path:p1",
				Direction:      "both",
				Backend:        "alsa",
				SupportsVolume: true,
				SupportsMute:   true,
				SupportsGain:   true,
			},
		},
		Output: &crosstalkv2.AudioOutputObserved{
			DeviceUid:     "usb:0d8c:0014:path:p1",
			VolumePercent: &vol,
			Muted:         &muted,
			VolumeState:   crosstalkv2.AudioApplyState_AUDIO_APPLY_STATE_APPLIED,
			MuteState:     crosstalkv2.AudioApplyState_AUDIO_APPLY_STATE_APPLIED,
		},
		Input: &crosstalkv2.AudioInputObserved{
			DeviceUid:   "usb:0d8c:0014:path:p1",
			GainPercent: &gain,
			GainState:   crosstalkv2.AudioApplyState_AUDIO_APPLY_STATE_APPLIED,
		},
	}
	obs, ok := mapAudioControlReport(report)
	require.True(t, ok)
	assert.Equal(t, uint64(2), obs.DesiredRevision)
	assert.Equal(t, "abc-audio/abc1/2", obs.CommandID)
	require.NotNil(t, obs.ObservedOutputVolumePercent)
	assert.Equal(t, 65, *obs.ObservedOutputVolumePercent)
	require.NotNil(t, obs.ObservedOutputMuted)
	assert.False(t, *obs.ObservedOutputMuted)
	require.NotNil(t, obs.ObservedInputGainPercent)
	assert.Equal(t, 40, *obs.ObservedInputGainPercent)
	assert.Equal(t, crosstalk.ABCAudioControlApplied, obs.OutputVolumeState)
	assert.Equal(t, crosstalk.ABCAudioControlApplied, obs.OutputMuteState)
	assert.Equal(t, crosstalk.ABCAudioControlApplied, obs.InputGainState)
	require.Len(t, obs.Capabilities, 1)
	assert.Equal(t, "usb:0d8c:0014:path:p1", obs.Capabilities[0].DeviceUID)
}

func TestMapAudioControlReport_InventoryOnly(t *testing.T) {
	report := &crosstalkv2.AudioControlReport{
		DesiredRevision: 0,
		Devices: []*crosstalkv2.AudioDeviceCapability{
			{DeviceUid: "usb:aaaa:bbbb:path:x", Direction: "both", Backend: "alsa"},
		},
	}
	obs, ok := mapAudioControlReport(report)
	require.True(t, ok)
	assert.Equal(t, uint64(0), obs.DesiredRevision)
	assert.Equal(t, crosstalk.ABCAudioControlUnknown, obs.OutputVolumeState)
	assert.Equal(t, crosstalk.ABCAudioControlUnknown, obs.OutputMuteState)
	assert.Equal(t, crosstalk.ABCAudioControlUnknown, obs.InputGainState)
	require.Len(t, obs.Capabilities, 1)
}

func TestMapAudioControlReport_StateMapping(t *testing.T) {
	cases := []struct {
		in   crosstalkv2.AudioApplyState
		want crosstalk.ABCAudioControlState
	}{
		{crosstalkv2.AudioApplyState_AUDIO_APPLY_STATE_UNSPECIFIED, crosstalk.ABCAudioControlUnknown},
		{crosstalkv2.AudioApplyState_AUDIO_APPLY_STATE_APPLIED, crosstalk.ABCAudioControlApplied},
		{crosstalkv2.AudioApplyState_AUDIO_APPLY_STATE_UNSUPPORTED, crosstalk.ABCAudioControlUnsupported},
		{crosstalkv2.AudioApplyState_AUDIO_APPLY_STATE_ERROR, crosstalk.ABCAudioControlError},
		{crosstalkv2.AudioApplyState_AUDIO_APPLY_STATE_DEVICE_MISMATCH, crosstalk.ABCAudioControlDeviceMismatch},
		{crosstalkv2.AudioApplyState_AUDIO_APPLY_STATE_STALE_REVISION, crosstalk.ABCAudioControlUnknown},
	}
	for _, tc := range cases {
		got, ok := mapAudioApplyState(tc.in)
		require.True(t, ok, "state %v", tc.in)
		assert.Equal(t, tc.want, got, "state %v", tc.in)
	}
	_, ok := mapAudioApplyState(crosstalkv2.AudioApplyState(99))
	assert.False(t, ok)
}

func TestMapAudioControlReport_TooManyDevices(t *testing.T) {
	devs := make([]*crosstalkv2.AudioDeviceCapability, MaxABCAudioDevices+1)
	for i := range devs {
		devs[i] = &crosstalkv2.AudioDeviceCapability{DeviceUid: "usb:x"}
	}
	_, ok := mapAudioControlReport(&crosstalkv2.AudioControlReport{Devices: devs})
	assert.False(t, ok)
}

func TestMapAudioControlReport_PercentOutOfRange(t *testing.T) {
	bad := uint32(101)
	_, ok := mapAudioControlReport(&crosstalkv2.AudioControlReport{
		DesiredRevision: 1,
		Output: &crosstalkv2.AudioOutputObserved{
			VolumePercent: &bad,
			VolumeState:   crosstalkv2.AudioApplyState_AUDIO_APPLY_STATE_APPLIED,
			MuteState:     crosstalkv2.AudioApplyState_AUDIO_APPLY_STATE_APPLIED,
		},
		Input: &crosstalkv2.AudioInputObserved{
			GainState: crosstalkv2.AudioApplyState_AUDIO_APPLY_STATE_APPLIED,
		},
	})
	assert.False(t, ok)
}

func TestBuildAudioControlCommand(t *testing.T) {
	vol, gain := 65, 40
	muted := true
	st := &crosstalk.ABCAudioStatus{
		ABCID: "abc1",
		Desired: crosstalk.ABCAudioDesiredView{
			Revision:            3,
			CommandID:           crosstalk.ABCAudioCommandID("abc1", 3),
			OutputDeviceUID:     "usb:out",
			OutputVolumePercent: &vol,
			OutputMuted:         &muted,
			InputDeviceUID:      "usb:in",
			InputGainPercent:    &gain,
		},
	}
	cmd := buildAudioControlCommand(st)
	require.NotNil(t, cmd)
	assert.Equal(t, "abc-audio/abc1/3", cmd.GetCommandId())
	assert.Equal(t, uint64(3), cmd.GetDesiredRevision())
	require.NotNil(t, cmd.GetOutput())
	assert.Equal(t, "usb:out", cmd.GetOutput().GetDeviceUid())
	assert.Equal(t, uint32(65), cmd.GetOutput().GetVolumePercent())
	assert.True(t, cmd.GetOutput().GetMuted())
	require.NotNil(t, cmd.GetInput())
	assert.Equal(t, "usb:in", cmd.GetInput().GetDeviceUid())
	assert.Equal(t, uint32(40), cmd.GetInput().GetGainPercent())
}

func TestABCAudioNeedsCommand(t *testing.T) {
	vol := 50
	st := &crosstalk.ABCAudioStatus{
		Desired: crosstalk.ABCAudioDesiredView{
			Revision:            2,
			OutputDeviceUID:     "usb:d",
			OutputVolumePercent: &vol,
			InputDeviceUID:      "usb:d",
		},
		Reported: crosstalk.ABCAudioReportedView{
			Revision:          1,
			OutputVolumeState: crosstalk.ABCAudioControlApplied,
			OutputMuteState:   crosstalk.ABCAudioControlApplied,
			InputGainState:    crosstalk.ABCAudioControlApplied,
		},
	}
	assert.True(t, abcAudioNeedsCommand(st), "report behind desired")

	st.Reported.Revision = 2
	assert.False(t, abcAudioNeedsCommand(st), "conclusive applied should not resend")

	st.Reported.OutputVolumeState = crosstalk.ABCAudioControlUnknown
	assert.True(t, abcAudioNeedsCommand(st), "non-conclusive needs command")

	st.Reported.OutputVolumeState = crosstalk.ABCAudioControlDeviceMismatch
	st.Reported.OutputMuteState = crosstalk.ABCAudioControlDeviceMismatch
	st.Reported.InputGainState = crosstalk.ABCAudioControlDeviceMismatch
	st.Reported.Capabilities = nil
	assert.False(t, abcAudioNeedsCommand(st), "mismatch without device return")

	st.Reported.Capabilities = []crosstalk.ABCAudioCapability{{DeviceUID: "usb:d"}}
	assert.True(t, abcAudioNeedsCommand(st), "mismatch after device return")

	st.Desired.Revision = 0
	assert.False(t, abcAudioNeedsCommand(st), "unconfigured")
}

func TestABCPeerRegistry_GenerationRace(t *testing.T) {
	s := &Server{
		abcPeers: make(map[string]abcPeerEntry),
		log:      testLogger(),
	}
	pm := webrtc.NewPeerManagerWithAPI(webrtcTestAPI())
	p1, err := pm.CreatePeerConnection()
	require.NoError(t, err)
	p2, err := pm.CreatePeerConnection()
	require.NoError(t, err)
	defer pm.RemovePeer(p1.ID)
	defer pm.RemovePeer(p2.ID)

	g1 := s.registerABCPeer("abc-a", p1)
	require.Equal(t, uint64(1), g1)
	e, ok := s.lookupABCPeer("abc-a")
	require.True(t, ok)
	assert.Equal(t, p1.ID, e.PeerID)
	assert.Equal(t, g1, e.Generation)

	g2 := s.registerABCPeer("abc-a", p2)
	require.Equal(t, uint64(2), g2)
	// Stale close of gen1 must not remove gen2.
	s.deregisterABCPeer("abc-a", p1.ID, g1)
	e, ok = s.lookupABCPeer("abc-a")
	require.True(t, ok)
	assert.Equal(t, p2.ID, e.PeerID)
	assert.Equal(t, g2, e.Generation)

	s.deregisterABCPeer("abc-a", p2.ID, g2)
	_, ok = s.lookupABCPeer("abc-a")
	assert.False(t, ok)
}

func TestRestartABC_RemovesLivePeer(t *testing.T) {
	pm := webrtc.NewPeerManagerWithAPI(webrtcTestAPI())
	peer, err := pm.CreatePeerConnection()
	require.NoError(t, err)

	s := &Server{
		services: Services{PeerManager: pm},
		abcPeers: make(map[string]abcPeerEntry),
		log:      testLogger(),
	}
	s.registerABCPeer("abc-a", peer)

	require.True(t, s.restartABC("abc-a"))
	// RemovePeer runs asynchronously so the HTTP handler cannot block on Pion Close.
	require.Eventually(t, func() bool {
		return pm.FindPeer(peer.ID) == nil
	}, time.Second, 10*time.Millisecond)
	// Registry still has the entry until OnClose deregisters; offline check uses FindPeer.
	assert.False(t, s.restartABC("abc-a"), "peer already removed must not report a restart")
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func webrtcTestAPI() *pionwebrtc.API {
	var se pionwebrtc.SettingEngine
	se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	return pionwebrtc.NewAPI(pionwebrtc.WithSettingEngine(se))
}
