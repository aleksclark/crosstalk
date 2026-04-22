package pion

import (
	"testing"

	crosstalkv1 "github.com/aleksclark/crosstalk/proto/gen/go/crosstalk/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestParseControlMessage_BindChannel(t *testing.T) {
	msg := &crosstalkv1.ControlMessage{
		Payload: &crosstalkv1.ControlMessage_BindChannel{
			BindChannel: &crosstalkv1.BindChannel{
				ChannelId: "ch-1",
				LocalName: "mic-1",
				Direction: crosstalkv1.Direction_SOURCE,
				TrackId:   "trk-1",
			},
		},
	}
	data, err := proto.Marshal(msg)
	require.NoError(t, err)

	parsed, err := ParseControlMessage(data)
	require.NoError(t, err)
	bc := parsed.GetBindChannel()
	require.NotNil(t, bc)
	assert.Equal(t, "ch-1", bc.GetChannelId())
	assert.Equal(t, "mic-1", bc.GetLocalName())
	assert.Equal(t, crosstalkv1.Direction_SOURCE, bc.GetDirection())
	assert.Equal(t, "trk-1", bc.GetTrackId())
}

func TestParseControlMessage_UnbindChannel(t *testing.T) {
	msg := &crosstalkv1.ControlMessage{
		Payload: &crosstalkv1.ControlMessage_UnbindChannel{
			UnbindChannel: &crosstalkv1.UnbindChannel{
				ChannelId: "ch-1",
			},
		},
	}
	data, err := proto.Marshal(msg)
	require.NoError(t, err)

	parsed, err := ParseControlMessage(data)
	require.NoError(t, err)
	uc := parsed.GetUnbindChannel()
	require.NotNil(t, uc)
	assert.Equal(t, "ch-1", uc.GetChannelId())
}

func TestParseControlMessage_SessionEvent(t *testing.T) {
	msg := &crosstalkv1.ControlMessage{
		Payload: &crosstalkv1.ControlMessage_SessionEvent{
			SessionEvent: &crosstalkv1.SessionEvent{
				Type:      crosstalkv1.SessionEventType_SESSION_CLIENT_JOINED,
				Message:   "client joined",
				SessionId: "sess-1",
			},
		},
	}
	data, err := proto.Marshal(msg)
	require.NoError(t, err)

	parsed, err := ParseControlMessage(data)
	require.NoError(t, err)
	se := parsed.GetSessionEvent()
	require.NotNil(t, se)
	assert.Equal(t, crosstalkv1.SessionEventType_SESSION_CLIENT_JOINED, se.GetType())
	assert.Equal(t, "sess-1", se.GetSessionId())
}

func TestParseControlMessage_ChannelStatus(t *testing.T) {
	msg := &crosstalkv1.ControlMessage{
		Payload: &crosstalkv1.ControlMessage_ChannelStatus{
			ChannelStatus: &crosstalkv1.ChannelStatus{
				ChannelId: "ch-1",
				State:     crosstalkv1.ChannelState_CHANNEL_ACTIVE,
			},
		},
	}
	data, err := proto.Marshal(msg)
	require.NoError(t, err)

	parsed, err := ParseControlMessage(data)
	require.NoError(t, err)
	cs := parsed.GetChannelStatus()
	require.NotNil(t, cs)
	assert.Equal(t, "ch-1", cs.GetChannelId())
	assert.Equal(t, crosstalkv1.ChannelState_CHANNEL_ACTIVE, cs.GetState())
}

func TestMarshalBindChannelRoundTrip(t *testing.T) {
	msg := &crosstalkv1.ControlMessage{
		Payload: &crosstalkv1.ControlMessage_BindChannel{
			BindChannel: &crosstalkv1.BindChannel{
				ChannelId: "ch-2",
				LocalName: "speakers",
				Direction: crosstalkv1.Direction_SINK,
				TrackId:   "trk-2",
			},
		},
	}

	data, err := proto.Marshal(msg)
	require.NoError(t, err)

	parsed, err := ParseControlMessage(data)
	require.NoError(t, err)
	bc := parsed.GetBindChannel()
	require.NotNil(t, bc)
	assert.Equal(t, "ch-2", bc.GetChannelId())
	assert.Equal(t, crosstalkv1.Direction_SINK, bc.GetDirection())
}

func TestMarshalChannelStatusRoundTrip(t *testing.T) {
	msg := &crosstalkv1.ControlMessage{
		Payload: &crosstalkv1.ControlMessage_ChannelStatus{
			ChannelStatus: &crosstalkv1.ChannelStatus{
				ChannelId:    "ch-1",
				State:        crosstalkv1.ChannelState_CHANNEL_ERROR,
				ErrorMessage: "track creation failed",
			},
		},
	}

	data, err := proto.Marshal(msg)
	require.NoError(t, err)

	parsed, err := ParseControlMessage(data)
	require.NoError(t, err)
	cs := parsed.GetChannelStatus()
	require.NotNil(t, cs)
	assert.Equal(t, "ch-1", cs.GetChannelId())
	assert.Equal(t, crosstalkv1.ChannelState_CHANNEL_ERROR, cs.GetState())
	assert.Equal(t, "track creation failed", cs.GetErrorMessage())
}
