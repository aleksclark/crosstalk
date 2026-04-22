package pion

import (
	"testing"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	crosstalk "github.com/aleksclark/crosstalk/server"
)

// testWebRTCConfig returns a minimal WebRTCConfig with no external STUN/TURN
// servers — sufficient for in-process peer-to-peer tests.
func testWebRTCConfig() crosstalk.WebRTCConfig {
	return crosstalk.WebRTCConfig{}
}

// testAPI returns a webrtc.API with mDNS disabled so that in-process tests do
// not attempt multicast DNS on the host.
func testAPI(t *testing.T) *webrtc.API {
	t.Helper()
	var se webrtc.SettingEngine
	se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	return webrtc.NewAPI(webrtc.WithSettingEngine(se))
}

// testPeerManager returns a PeerManager wired to the test-only API.
func testPeerManager(t *testing.T) *PeerManager {
	t.Helper()
	return NewPeerManagerWithAPI(testWebRTCConfig(), testAPI(t))
}

// createClientPC creates a "client-side" PeerConnection using the same
// test-only API (mDNS disabled, no STUN/TURN).
func createClientPC(t *testing.T) *webrtc.PeerConnection {
	t.Helper()
	api := testAPI(t)
	pc, err := api.NewPeerConnection(webrtc.Configuration{})
	require.NoError(t, err)
	t.Cleanup(func() { pc.Close() }) //nolint:errcheck
	return pc
}

// signalPair performs a full offer/answer exchange between a client
// PeerConnection and a server PeerConn. Both sides gather ICE candidates
// completely before exchanging SDPs (non-trickle), which is the simplest
// approach for in-process tests.
func signalPair(t *testing.T, client *webrtc.PeerConnection, server *PeerConn) {
	t.Helper()

	// Client creates offer. We must wait for gathering to complete so the SDP
	// contains ICE candidates and credentials.
	offer, err := client.CreateOffer(nil)
	require.NoError(t, err)

	clientGatherDone := webrtc.GatheringCompletePromise(client)
	require.NoError(t, client.SetLocalDescription(offer))
	<-clientGatherDone

	// Use the fully-gathered local description (includes candidates).
	fullOffer := *client.LocalDescription()

	// Server handles the offer: sets remote SDP, creates + sets answer.
	answer, err := server.HandleOffer(fullOffer)
	require.NoError(t, err)
	_ = answer // HandleOffer already sets local description on server side.

	// Wait for server-side gathering to complete, then read the full answer.
	serverGatherDone := webrtc.GatheringCompletePromise(server.pc)
	<-serverGatherDone
	fullAnswer := *server.pc.LocalDescription()

	// Set the fully-gathered answer on the client.
	require.NoError(t, client.SetRemoteDescription(fullAnswer))
}

func TestPeerConnection_OfferAnswer(t *testing.T) {
	pm := testPeerManager(t)
	server, err := pm.CreatePeerConnection()
	require.NoError(t, err)
	t.Cleanup(func() { server.Close() }) //nolint:errcheck

	client := createClientPC(t)

	// The client needs at least one data channel for the offer SDP to contain
	// an application m-line with ICE credentials.
	_, err = client.CreateDataChannel("init", nil)
	require.NoError(t, err)

	// Track ICE connection state on the client side.
	connected := make(chan struct{})
	client.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		if state == webrtc.ICEConnectionStateConnected {
			close(connected)
		}
	})

	signalPair(t, client, server)

	select {
	case <-connected:
		// success
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for ICE connected state")
	}
}

func TestPeerConnection_ICETrickle(t *testing.T) {
	pm := testPeerManager(t)
	server, err := pm.CreatePeerConnection()
	require.NoError(t, err)
	t.Cleanup(func() { server.Close() }) //nolint:errcheck

	client := createClientPC(t)

	// Client needs a data channel for proper SDP generation.
	_, err = client.CreateDataChannel("init", nil)
	require.NoError(t, err)

	// Track that at least one ICE candidate was gathered by each side.
	clientGathered := make(chan struct{}, 1)
	serverGathered := make(chan struct{}, 1)

	client.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c != nil {
			select {
			case clientGathered <- struct{}{}:
			default:
			}
		}
	})
	server.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c != nil {
			select {
			case serverGathered <- struct{}{}:
			default:
			}
		}
	})

	// Create offer and set local description (starts ICE gathering).
	offer, err := client.CreateOffer(nil)
	require.NoError(t, err)

	clientGatherComplete := webrtc.GatheringCompletePromise(client)
	require.NoError(t, client.SetLocalDescription(offer))
	<-clientGatherComplete

	// Pass the full offer to server.
	fullOffer := *client.LocalDescription()
	answer, err := server.HandleOffer(fullOffer)
	require.NoError(t, err)
	_ = answer

	serverGatherComplete := webrtc.GatheringCompletePromise(server.pc)
	<-serverGatherComplete
	fullAnswer := *server.pc.LocalDescription()
	require.NoError(t, client.SetRemoteDescription(fullAnswer))

	// Verify that both sides gathered at least one candidate.
	timeout := time.After(10 * time.Second)
	for clientGathered != nil || serverGathered != nil {
		select {
		case <-clientGathered:
			clientGathered = nil
		case <-serverGathered:
			serverGathered = nil
		case <-timeout:
			t.Fatal("timed out waiting for ICE candidates to be gathered")
		}
	}
}

func TestPeerConnection_DataChannelEcho(t *testing.T) {
	pm := testPeerManager(t)
	server, err := pm.CreatePeerConnection()
	require.NoError(t, err)
	t.Cleanup(func() { server.Close() }) //nolint:errcheck

	client := createClientPC(t)

	// The server creates the "control" data channel during
	// CreatePeerConnection. The client receives it via OnDataChannel.
	echoed := make(chan []byte, 1)
	client.OnDataChannel(func(dc *webrtc.DataChannel) {
		if dc.Label() != "control" {
			return
		}
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			echoed <- msg.Data
		})
		dc.OnOpen(func() {
			require.NoError(t, dc.Send([]byte("ping")))
		})
	})

	// Client needs a data channel for proper SDP generation.
	_, err = client.CreateDataChannel("init", nil)
	require.NoError(t, err)

	// Track connection.
	connected := make(chan struct{})
	client.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		if state == webrtc.ICEConnectionStateConnected {
			close(connected)
		}
	})

	signalPair(t, client, server)

	select {
	case <-connected:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for ICE connected state")
	}

	select {
	case data := <-echoed:
		assert.Equal(t, []byte("ping"), data)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for data channel echo")
	}
}

func TestPeerManager_Registry(t *testing.T) {
	pm := testPeerManager(t)

	require.Equal(t, 0, pm.Count())

	// Create three peer connections.
	p1, err := pm.CreatePeerConnection()
	require.NoError(t, err)
	p2, err := pm.CreatePeerConnection()
	require.NoError(t, err)
	p3, err := pm.CreatePeerConnection()
	require.NoError(t, err)

	assert.Equal(t, 3, pm.Count())

	// Each has a unique ID.
	assert.NotEqual(t, p1.ID, p2.ID)
	assert.NotEqual(t, p2.ID, p3.ID)

	// Remove one.
	pm.RemovePeer(p2.ID)
	assert.Equal(t, 2, pm.Count())

	// Removing the same id again is a no-op.
	pm.RemovePeer(p2.ID)
	assert.Equal(t, 2, pm.Count())

	// Cleanup.
	pm.RemovePeer(p1.ID)
	pm.RemovePeer(p3.ID)
	assert.Equal(t, 0, pm.Count())
}
