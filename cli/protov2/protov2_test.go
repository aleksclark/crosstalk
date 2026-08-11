package protov2_test

import (
	"testing"

	"github.com/aleksclark/crosstalk/cli/protov2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildAndParseHelloV2(t *testing.T) {
	data := protov2.BuildHelloV2("abc", "Booth-A")
	require.NotEmpty(t, data)

	msg, err := protov2.ParseControlMessageV2(data)
	require.NoError(t, err)
	assert.Equal(t, protov2.PayloadHello, msg.Type)
}

func TestBuildAndParseWelcomeV2(t *testing.T) {
	welcome := encodeTestWelcome("peer-123", "1.0.0", "session-456")

	msg, err := protov2.ParseControlMessageV2(welcome)
	require.NoError(t, err)
	assert.Equal(t, protov2.PayloadWelcome, msg.Type)
	require.NotNil(t, msg.Welcome)
	assert.Equal(t, "peer-123", msg.Welcome.PeerID)
	assert.Equal(t, "1.0.0", msg.Welcome.ServerVersion)
	assert.Equal(t, "session-456", msg.Welcome.AssignedSessionID)
}

func TestBuildAndParseRestartV2(t *testing.T) {
	restart := encodeTestRestart("session reassigned")

	msg, err := protov2.ParseControlMessageV2(restart)
	require.NoError(t, err)
	assert.Equal(t, protov2.PayloadRestart, msg.Type)
	require.NotNil(t, msg.Restart)
	assert.Equal(t, "session reassigned", msg.Restart.Reason)
}

func TestBuildAndParseSessionAssignmentV2(t *testing.T) {
	sa := encodeTestSessionAssignment("session-789", "abc")

	msg, err := protov2.ParseControlMessageV2(sa)
	require.NoError(t, err)
	assert.Equal(t, protov2.PayloadSessionAssignment, msg.Type)
	require.NotNil(t, msg.SessionAssignment)
	assert.Equal(t, "session-789", msg.SessionAssignment.SessionID)
	assert.Equal(t, "abc", msg.SessionAssignment.Role)
}

func TestParseEmptyMessage(t *testing.T) {
	msg, err := protov2.ParseControlMessageV2([]byte{})
	require.NoError(t, err)
	assert.Equal(t, protov2.PayloadUnknown, msg.Type)
}

func TestParseInvalidData(t *testing.T) {
	_, err := protov2.ParseControlMessageV2([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
	require.Error(t, err)
}

func TestBuildSourceStatusV2(t *testing.T) {
	data := protov2.BuildSourceStatusV2("mic1", true, 0.85)
	require.NotEmpty(t, data)

	msg, err := protov2.ParseControlMessageV2(data)
	require.NoError(t, err)
	assert.Equal(t, protov2.PayloadSourceStatus, msg.Type)
}

func TestParseAudioControlCommand_Boundaries(t *testing.T) {
	// volume 0, mute false, gain 100
	vol0 := uint32(0)
	muted := false
	gain100 := uint32(100)
	inner := encodeAudioCommand("abc-audio/id/1", 7,
		"usb:0d8c:0014:serial:S1", &vol0, &muted,
		"usb:0d8c:0014:serial:S1", &gain100,
	)
	msg, err := protov2.ParseControlMessageV2(wrapField(14, inner))
	require.NoError(t, err)
	assert.Equal(t, protov2.PayloadAudioControlCommand, msg.Type)
	require.NotNil(t, msg.AudioControlCommand)
	cmd := msg.AudioControlCommand
	assert.Equal(t, "abc-audio/id/1", cmd.CommandID)
	assert.Equal(t, uint64(7), cmd.DesiredRevision)
	require.NotNil(t, cmd.Output)
	require.NotNil(t, cmd.Output.VolumePercent)
	assert.Equal(t, uint32(0), *cmd.Output.VolumePercent)
	require.NotNil(t, cmd.Output.Muted)
	assert.False(t, *cmd.Output.Muted)
	require.NotNil(t, cmd.Input)
	require.NotNil(t, cmd.Input.GainPercent)
	assert.Equal(t, uint32(100), *cmd.Input.GainPercent)
}

func TestParseAudioControlCommand_UnknownFieldsSkipped(t *testing.T) {
	var inner []byte
	inner = appendTestString(inner, 1, "cmd-1")
	inner = appendTestVarintField(inner, 2, 3)
	// unknown field 99 string
	inner = appendTestString(inner, 99, "future")
	// output with unknown nested field
	var out []byte
	out = appendTestString(out, 1, "usb:0d8c:0014:path:x")
	out = appendTestVarintField(out, 2, 50)
	out = appendTestString(out, 77, "ignore-me")
	inner = appendTestBytes(inner, 3, out)

	msg, err := protov2.ParseControlMessageV2(wrapField(14, inner))
	require.NoError(t, err)
	require.NotNil(t, msg.AudioControlCommand)
	assert.Equal(t, uint64(3), msg.AudioControlCommand.DesiredRevision)
	require.NotNil(t, msg.AudioControlCommand.Output)
	assert.Equal(t, uint32(50), *msg.AudioControlCommand.Output.VolumePercent)
}

func TestParseOversizedMessageRejected(t *testing.T) {
	big := make([]byte, protov2.MaxControlMessageBytes+1)
	_, err := protov2.ParseControlMessageV2(big)
	require.Error(t, err)
}

func TestBuildAudioControlReport_Limits(t *testing.T) {
	vol := uint32(0)
	muted := false
	gain := uint32(100)
	rep := protov2.AudioControlReport{
		CommandID:       "abc-audio/x/1",
		DesiredRevision: 1,
		Devices: []protov2.AudioDeviceCapability{
			{
				DeviceUID:      "usb:0d8c:0014:serial:S1",
				Direction:      "both",
				Backend:        "alsa",
				SupportsVolume: true,
				SupportsMute:   true,
				SupportsGain:   true,
			},
		},
		Output: &protov2.AudioOutputObserved{
			DeviceUID:     "usb:0d8c:0014:serial:S1",
			VolumePercent: &vol,
			Muted:         &muted,
			VolumeState:   protov2.AudioApplyApplied,
			MuteState:     protov2.AudioApplyApplied,
		},
		Input: &protov2.AudioInputObserved{
			DeviceUID:   "usb:0d8c:0014:serial:S1",
			GainPercent: &gain,
			GainState:   protov2.AudioApplyApplied,
		},
	}
	data, err := protov2.BuildAudioControlReport(rep)
	require.NoError(t, err)
	require.NotEmpty(t, data)
	// field 3 payload type when parsed (we only parse command path; type is set to last field)
	msg, err := protov2.ParseControlMessageV2(data)
	require.NoError(t, err)
	assert.Equal(t, protov2.PayloadAudioControlReport, msg.Type)

	// Oversized device list
	tooMany := protov2.AudioControlReport{Devices: make([]protov2.AudioDeviceCapability, protov2.MaxDevices+1)}
	for i := range tooMany.Devices {
		tooMany.Devices[i].DeviceUID = "x"
	}
	_, err = protov2.BuildAudioControlReport(tooMany)
	require.Error(t, err)
}

func TestBuildAudioControlReport_ErrorDetailCap(t *testing.T) {
	detail := string(make([]byte, protov2.MaxErrorDetailBytes+1))
	for i := range detail {
		detail = detail[:i] + "a" + detail[i+1:]
	}
	_, err := protov2.BuildAudioControlReport(protov2.AudioControlReport{
		ErrorDetail: detail,
	})
	require.Error(t, err)
}

// --- Test helpers ---

func encodeTestWelcome(peerID, serverVersion, assignedSession string) []byte {
	var inner []byte
	inner = appendTestString(inner, 1, peerID)
	inner = appendTestString(inner, 2, serverVersion)
	if assignedSession != "" {
		inner = appendTestString(inner, 3, assignedSession)
	}
	return wrapField(10, inner)
}

func encodeTestRestart(reason string) []byte {
	var inner []byte
	inner = appendTestString(inner, 1, reason)
	return wrapField(12, inner)
}

func encodeTestSessionAssignment(sessionID, role string) []byte {
	var inner []byte
	inner = appendTestString(inner, 1, sessionID)
	inner = appendTestString(inner, 2, role)
	return wrapField(13, inner)
}

func encodeAudioCommand(cmdID string, rev uint64, outUID string, vol *uint32, muted *bool, inUID string, gain *uint32) []byte {
	var inner []byte
	inner = appendTestString(inner, 1, cmdID)
	inner = appendTestVarintField(inner, 2, rev)
	if outUID != "" {
		var out []byte
		out = appendTestString(out, 1, outUID)
		if vol != nil {
			out = appendTestVarintField(out, 2, uint64(*vol))
		}
		if muted != nil {
			if *muted {
				out = appendTestVarintField(out, 3, 1)
			} else {
				out = appendTestVarintField(out, 3, 0)
			}
		}
		inner = appendTestBytes(inner, 3, out)
	}
	if inUID != "" {
		var in []byte
		in = appendTestString(in, 1, inUID)
		if gain != nil {
			in = appendTestVarintField(in, 2, uint64(*gain))
		}
		inner = appendTestBytes(inner, 4, in)
	}
	return inner
}

func wrapField(fieldNum int, inner []byte) []byte {
	var msg []byte
	msg = appendTestBytes(msg, fieldNum, inner)
	return msg
}

func appendTestString(data []byte, fieldNum int, s string) []byte {
	return appendTestBytes(data, fieldNum, []byte(s))
}

func appendTestBytes(data []byte, fieldNum int, value []byte) []byte {
	tag := uint64(fieldNum<<3) | 2
	data = appendTestVarint(data, tag)
	data = appendTestVarint(data, uint64(len(value)))
	data = append(data, value...)
	return data
}

func appendTestVarintField(data []byte, fieldNum int, v uint64) []byte {
	tag := uint64(fieldNum << 3)
	data = appendTestVarint(data, tag)
	data = appendTestVarint(data, v)
	return data
}

func appendTestVarint(data []byte, v uint64) []byte {
	for v >= 0x80 {
		data = append(data, byte(v)|0x80)
		v >>= 7
	}
	data = append(data, byte(v))
	return data
}
