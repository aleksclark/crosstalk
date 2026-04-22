package pion

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	crosstalk "github.com/aleksclark/crosstalk/cli"
	climock "github.com/aleksclark/crosstalk/cli/mock"
	crosstalkv1 "github.com/aleksclark/crosstalk/proto/gen/go/crosstalk/v1"
	"google.golang.org/protobuf/proto"
)

func TestClient_HandleBindChannel_Source(t *testing.T) {
	cfg := &crosstalk.Config{
		ServerURL: "http://localhost",
		Token:     "test",
	}
	pwSvc := &climock.PipeWireService{
		DiscoverFn: func() ([]crosstalk.Source, []crosstalk.Sink, error) {
			return nil, nil, nil
		},
	}

	var bindReceived *BindChannelMsg
	client := NewClient(cfg, pwSvc, WithOnBindChannelClient(func(b *BindChannelMsg) {
		bindReceived = b
	}))

	mc := &mockConn{}
	bind := &BindChannelMsg{
		ChannelID: "ch-1",
		LocalName: "mic-1",
		Direction: DirectionSource,
		TrackID:   "trk-1",
	}

	client.handleBindChannel(mc, bind)

	require.NotNil(t, bindReceived)
	assert.Equal(t, "ch-1", bindReceived.ChannelID)
	assert.Equal(t, DirectionSource, bindReceived.Direction)

	mc.mu.Lock()
	statuses := mc.channelStatuses
	mc.mu.Unlock()
	require.Len(t, statuses, 1)
	assert.Equal(t, "ch-1", statuses[0].ChannelID)
	assert.Equal(t, crosstalkv1.ChannelState_CHANNEL_ACTIVE, statuses[0].State)
}

func TestClient_HandleBindChannel_Sink(t *testing.T) {
	cfg := &crosstalk.Config{
		ServerURL: "http://localhost",
		Token:     "test",
	}
	pwSvc := &climock.PipeWireService{
		DiscoverFn: func() ([]crosstalk.Source, []crosstalk.Sink, error) {
			return nil, nil, nil
		},
	}

	client := NewClient(cfg, pwSvc)

	mc := &mockConn{}
	bind := &BindChannelMsg{
		ChannelID: "ch-2",
		LocalName: "speakers",
		Direction: DirectionSink,
		TrackID:   "trk-2",
	}

	client.handleBindChannel(mc, bind)

	mc.mu.Lock()
	statuses := mc.channelStatuses
	mc.mu.Unlock()
	require.Len(t, statuses, 1)
	assert.Equal(t, "ch-2", statuses[0].ChannelID)
	assert.Equal(t, crosstalkv1.ChannelState_CHANNEL_BINDING, statuses[0].State)
}

func TestClient_HandleBindChannel_AddTrackError(t *testing.T) {
	cfg := &crosstalk.Config{
		ServerURL: "http://localhost",
		Token:     "test",
	}
	pwSvc := &climock.PipeWireService{
		DiscoverFn: func() ([]crosstalk.Source, []crosstalk.Sink, error) {
			return nil, nil, nil
		},
	}

	client := NewClient(cfg, pwSvc)

	mc := &mockConn{
		addTrackFn: func(channelID, trackID, localName string, dir Direction) (*BoundTrack, error) {
			return nil, fmt.Errorf("peer connection not established")
		},
	}
	bind := &BindChannelMsg{
		ChannelID: "ch-3",
		LocalName: "mic-1",
		Direction: DirectionSource,
		TrackID:   "trk-3",
	}

	client.handleBindChannel(mc, bind)

	mc.mu.Lock()
	statuses := mc.channelStatuses
	mc.mu.Unlock()
	require.Len(t, statuses, 1)
	assert.Equal(t, "ch-3", statuses[0].ChannelID)
	assert.Equal(t, crosstalkv1.ChannelState_CHANNEL_ERROR, statuses[0].State)
	assert.Contains(t, statuses[0].ErrMsg, "peer connection not established")
}

func TestClient_HandleUnbindChannel(t *testing.T) {
	cfg := &crosstalk.Config{
		ServerURL: "http://localhost",
		Token:     "test",
	}
	pwSvc := &climock.PipeWireService{
		DiscoverFn: func() ([]crosstalk.Source, []crosstalk.Sink, error) {
			return nil, nil, nil
		},
	}

	client := NewClient(cfg, pwSvc)

	var removedChannelID string
	var removeMu sync.Mutex
	mc := &mockConn{
		removeTrackFn: func(channelID string) error {
			removeMu.Lock()
			removedChannelID = channelID
			removeMu.Unlock()
			return nil
		},
	}

	unbind := &UnbindChannelMsg{
		ChannelID: "ch-1",
	}

	client.handleUnbindChannel(mc, unbind)

	removeMu.Lock()
	assert.Equal(t, "ch-1", removedChannelID)
	removeMu.Unlock()

	mc.mu.Lock()
	statuses := mc.channelStatuses
	mc.mu.Unlock()
	require.Len(t, statuses, 1)
	assert.Equal(t, "ch-1", statuses[0].ChannelID)
	assert.Equal(t, crosstalkv1.ChannelState_CHANNEL_IDLE, statuses[0].State)
}

func TestClient_ControlMessageDispatch_BindChannel(t *testing.T) {
	bindPb := &crosstalkv1.ControlMessage{
		Payload: &crosstalkv1.ControlMessage_BindChannel{
			BindChannel: &crosstalkv1.BindChannel{
				ChannelId: "ch-dispatch",
				LocalName: "mic-1",
				Direction: crosstalkv1.Direction_SOURCE,
				TrackId:   "trk-dispatch",
			},
		},
	}

	data, err := proto.Marshal(bindPb)
	require.NoError(t, err)

	parsed, err := ParseControlMessage(data)
	require.NoError(t, err)
	require.NotNil(t, parsed.GetBindChannel())
	assert.Equal(t, "ch-dispatch", parsed.GetBindChannel().GetChannelId())
}
