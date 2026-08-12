package protov2_test

import (
	"encoding/hex"
	"testing"

	"github.com/aleksclark/crosstalk/cli/protov2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Golden bytes produced by server/proto/v2 (protoc-gen-go) marshaling.
// Regenerated when regenerating control.pb.go — keep wire-compatible.
const (
	// volume=0, muted=false, gain=100, revision=1
	goldenAudioCmd0MuteFalseGain100 = "724d0a0d6162632d617564696f2f782f3110011a1d0a177573623a306438633a303031343a73657269616c3a533110001800221b0a177573623a306438633a303031343a73657269616c3a53311064"
	// volume=100, muted=true, revision=9, no input
	goldenAudioCmd100MuteTrue = "72220a02633210091a1a0a147573623a646561643a626565663a706174683a7810641801"
)

func TestConformance_ParseGeneratedAudioControlCommand(t *testing.T) {
	raw, err := hex.DecodeString(goldenAudioCmd0MuteFalseGain100)
	require.NoError(t, err)
	msg, err := protov2.ParseControlMessageV2(raw)
	require.NoError(t, err)
	assert.Equal(t, protov2.PayloadAudioControlCommand, msg.Type)
	require.NotNil(t, msg.AudioControlCommand)
	cmd := msg.AudioControlCommand
	assert.Equal(t, "abc-audio/x/1", cmd.CommandID)
	assert.Equal(t, uint64(1), cmd.DesiredRevision)
	require.NotNil(t, cmd.Output)
	require.NotNil(t, cmd.Output.VolumePercent)
	assert.Equal(t, uint32(0), *cmd.Output.VolumePercent)
	require.NotNil(t, cmd.Output.Muted)
	assert.False(t, *cmd.Output.Muted)
	require.NotNil(t, cmd.Input)
	require.NotNil(t, cmd.Input.GainPercent)
	assert.Equal(t, uint32(100), *cmd.Input.GainPercent)

	raw2, err := hex.DecodeString(goldenAudioCmd100MuteTrue)
	require.NoError(t, err)
	msg2, err := protov2.ParseControlMessageV2(raw2)
	require.NoError(t, err)
	require.NotNil(t, msg2.AudioControlCommand)
	require.NotNil(t, msg2.AudioControlCommand.Output.VolumePercent)
	assert.Equal(t, uint32(100), *msg2.AudioControlCommand.Output.VolumePercent)
	require.NotNil(t, msg2.AudioControlCommand.Output.Muted)
	assert.True(t, *msg2.AudioControlCommand.Output.Muted)
	assert.Nil(t, msg2.AudioControlCommand.Input)
}

func TestConformance_BuildReportWireShape(t *testing.T) {
	// Build report and ensure it starts with field 3 (audio_control_report) tag 0x1a.
	vol := uint32(0)
	muted := false
	gain := uint32(100)
	data, err := protov2.BuildAudioControlReport(protov2.AudioControlReport{
		CommandID:       "abc-audio/x/2",
		DesiredRevision: 2,
		Devices: []protov2.AudioDeviceCapability{{
			DeviceUID:      "usb:0d8c:0014:serial:S1",
			Backend:        "alsa",
			SupportsVolume: true,
			SupportsMute:   true,
			SupportsGain:   true,
		}},
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
	})
	require.NoError(t, err)
	require.NotEmpty(t, data)
	// field 3, wire type 2 → tag (3<<3)|2 = 26 = 0x1a
	assert.Equal(t, byte(0x1a), data[0])
	// Must stay within limits
	assert.LessOrEqual(t, len(data), protov2.MaxControlMessageBytes)
}

func TestConformance_TruncatedAndMalformed(t *testing.T) {
	raw, err := hex.DecodeString(goldenAudioCmd0MuteFalseGain100)
	require.NoError(t, err)
	// Truncate mid-message
	_, err = protov2.ParseControlMessageV2(raw[:len(raw)/2])
	require.Error(t, err)
	// Wrong wire type garbage after valid tag for field 14 with bad length
	_, err = protov2.ParseControlMessageV2([]byte{0x72, 0xff, 0xff, 0xff, 0xff, 0x0f})
	require.Error(t, err)
}
