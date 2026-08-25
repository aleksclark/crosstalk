package abc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	controlv2 "github.com/aleksclark/crosstalk/abc/internal/controlv2"
)

func TestEncodeHello_AdvertisesOfferedOpus(t *testing.T) {
	data, err := encodeHello("abc", "booth", DefaultHelloCapabilities)
	require.NoError(t, err)

	caps, err := helloCapabilitiesFromWire(data)
	require.NoError(t, err)
	require.NotEmpty(t, caps)

	var codecs []string
	for _, cap := range caps {
		codecs = append(codecs, cap.Codec)
		assert.Contains(t, []int32{1, 2}, cap.Channels)
		assert.Equal(t, int32(48000), cap.SampleRate)
	}
	assert.Contains(t, codecs, "opus/48000/2")
	assert.Contains(t, codecs, "opus/48000/1")

	var cm controlv2.ControlMessage
	require.NoError(t, proto.Unmarshal(data, &cm))
	hello := cm.GetHello()
	require.NotNil(t, hello)
	assert.Equal(t, "abc", hello.GetClientType())
	assert.Equal(t, "booth", hello.GetClientName())
}

func TestDecodeControlMessage_Welcome(t *testing.T) {
	data, err := proto.Marshal(&controlv2.ControlMessage{
		Payload: &controlv2.ControlMessage_Welcome{
			Welcome: &controlv2.Welcome{
				PeerId:            "peer-1",
				ServerVersion:     "3.0.0",
				AssignedSessionId: "sess-9",
			},
		},
	})
	require.NoError(t, err)

	msg, err := decodeControlMessage(data)
	require.NoError(t, err)
	require.NotNil(t, msg.Welcome)
	assert.Equal(t, "peer-1", msg.Welcome.PeerID)
	assert.Equal(t, "3.0.0", msg.Welcome.ServerVersion)
	assert.Equal(t, "sess-9", msg.Welcome.AssignedSessionID)
}

func TestDecodeControlMessage_MalformedIsProtocolError(t *testing.T) {
	_, err := decodeControlMessage([]byte{0xff, 0x00, 0x01})
	require.Error(t, err)
	assert.True(t, IsProtocolError(err))
	assert.NotContains(t, err.Error(), sentinelToken)
}

func TestDecodeControlMessage_OversizedIsProtocolError(t *testing.T) {
	_, err := decodeControlMessage(make([]byte, maxControlMessageBytes+1))
	require.Error(t, err)
	assert.True(t, IsProtocolError(err))
}

func TestAudioControlReportRoundTrip(t *testing.T) {
	vol := uint32(40)
	muted := true
	report := AudioControlReport{
		CommandID:       "cmd-1",
		DesiredRevision: 3,
		ErrorCode:       "",
		Output: &AudioOutputObserved{
			DeviceUID:     "usb:dev",
			VolumePercent: &vol,
			Muted:         &muted,
			VolumeState:   AudioApplyApplied,
			MuteState:     AudioApplyApplied,
		},
	}
	data, err := encodeControlReport(report)
	require.NoError(t, err)
	msg, err := decodeControlMessage(data)
	require.NoError(t, err)
	require.NotNil(t, msg.AudioControlReport)
	assert.Equal(t, "cmd-1", msg.AudioControlReport.CommandID)
	require.NotNil(t, msg.AudioControlReport.Output)
	assert.Equal(t, uint32(40), *msg.AudioControlReport.Output.VolumePercent)
}
