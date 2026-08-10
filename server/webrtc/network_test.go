package webrtc

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateICEConfig_AllAbsentOK(t *testing.T) {
	err := ValidateICEConfig(ICEConfig{})
	require.NoError(t, err)
}

func TestValidateICEConfig_STUNOnlyOK(t *testing.T) {
	err := ValidateICEConfig(ICEConfig{
		STUNServers: []string{"stun:stun.l.google.com:19302"},
	})
	require.NoError(t, err)
}

func TestValidateICEConfig_TURNCompleteOK(t *testing.T) {
	err := ValidateICEConfig(ICEConfig{
		TURNServer: "turn:turn.example.com:3478",
		TURNUser:   "user",
		TURNCred:   "secret-credential",
	})
	require.NoError(t, err)
}

func TestValidateICEConfig_TURNPartialRejected(t *testing.T) {
	cases := []struct {
		name string
		cfg  ICEConfig
	}{
		{
			name: "url only",
			cfg:  ICEConfig{TURNServer: "turn:turn.example.com:3478"},
		},
		{
			name: "url+user missing cred",
			cfg: ICEConfig{
				TURNServer: "turn:turn.example.com:3478",
				TURNUser:   "user",
			},
		},
		{
			name: "user+cred missing url",
			cfg: ICEConfig{
				TURNUser: "user",
				TURNCred: "secret",
			},
		},
		{
			name: "url+cred missing user",
			cfg: ICEConfig{
				TURNServer: "turn:turn.example.com:3478",
				TURNCred:   "secret",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateICEConfig(tc.cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "all present or all absent")
		})
	}
}

func TestValidateICEConfig_MalformedAndUnsupported(t *testing.T) {
	cases := []struct {
		name string
		cfg  ICEConfig
	}{
		{
			name: "stun missing scheme",
			cfg:  ICEConfig{STUNServers: []string{"stun.l.google.com:19302"}},
		},
		{
			name: "stun unsupported scheme",
			cfg:  ICEConfig{STUNServers: []string{"http://example.com"}},
		},
		{
			name: "turn with stun scheme",
			cfg: ICEConfig{
				TURNServer: "stun:stun.example.com:3478",
				TURNUser:   "u",
				TURNCred:   "c",
			},
		},
		{
			name: "turn empty host",
			cfg: ICEConfig{
				TURNServer: "turn:",
				TURNUser:   "u",
				TURNCred:   "c",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateICEConfig(tc.cfg)
			require.Error(t, err)
		})
	}
}

func TestBuildICEConfiguration_TURNCredentialsReachPion(t *testing.T) {
	const (
		turnURL  = "turns:turn.example.com:5349?transport=tcp"
		turnUser = "alice"
		turnCred = "super-secret-turn-password"
	)

	cfg := ICEConfig{
		STUNServers: []string{"stun:stun.l.google.com:19302"},
		TURNServer:  turnURL,
		TURNUser:    turnUser,
		TURNCred:    turnCred,
	}

	rtcCfg, err := BuildICEConfiguration(cfg)
	require.NoError(t, err)
	require.Len(t, rtcCfg.ICEServers, 2)

	// STUN server present.
	assert.Equal(t, []string{"stun:stun.l.google.com:19302"}, rtcCfg.ICEServers[0].URLs)
	assert.Empty(t, rtcCfg.ICEServers[0].Username)
	assert.Nil(t, rtcCfg.ICEServers[0].Credential)

	// TURN credentials reach webrtc.Configuration.
	turn := rtcCfg.ICEServers[1]
	assert.Equal(t, []string{turnURL}, turn.URLs)
	assert.Equal(t, turnUser, turn.Username)
	assert.Equal(t, turnCred, turn.Credential)
	assert.Equal(t, webrtc.ICECredentialTypePassword, turn.CredentialType)
}

func TestBuildICEConfiguration_DoesNotLogCredential(t *testing.T) {
	const turnCred = "must-not-appear-in-logs-xyzzy"

	var buf bytes.Buffer
	prev := slog.Default()
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(prev) })

	cfg := ICEConfig{
		STUNServers: []string{"stun:stun.l.google.com:19302"},
		TURNServer:  "turn:turn.example.com:3478",
		TURNUser:    "bob",
		TURNCred:    turnCred,
	}

	rtcCfg, err := BuildICEConfiguration(cfg)
	require.NoError(t, err)
	require.NotEmpty(t, rtcCfg.ICEServers)

	// Building config alone should not log; NewPeerManager may log metadata.
	pm := NewPeerManager(cfg)
	require.NotNil(t, pm)
	// Prove manager configuration carries the credential without logging it.
	got := pm.Configuration()
	require.GreaterOrEqual(t, len(got.ICEServers), 1)
	foundTURN := false
	for _, s := range got.ICEServers {
		if len(s.URLs) > 0 && strings.HasPrefix(s.URLs[0], "turn") {
			foundTURN = true
			assert.Equal(t, turnCred, s.Credential)
		}
	}
	assert.True(t, foundTURN, "expected TURN server in manager configuration")

	logOut := buf.String()
	assert.NotContains(t, logOut, turnCred, "TURN credential must not appear in logs")
}

func TestNewPeerManagerValidated_RejectsPartialTURN(t *testing.T) {
	_, err := NewPeerManagerValidated(ICEConfig{
		TURNServer: "turn:turn.example.com:3478",
		TURNUser:   "u",
		// missing credential
	})
	require.Error(t, err)
}

func TestAdmissionLimits_WithDefaults(t *testing.T) {
	d := AdmissionLimits{}.WithDefaults()
	assert.Equal(t, DefaultMaxSDPBytes, d.MaxSDPBytes)
	assert.Equal(t, DefaultMaxRemoteICECandidates, d.MaxRemoteICECandidates)
	assert.Equal(t, 0, d.MaxPeers)
	assert.Equal(t, DefaultMaxSDPBytes+4<<10, d.MaxSignalingMessageBytes)
}

func TestCheckSDPSize(t *testing.T) {
	require.NoError(t, CheckSDPSize("small", 10))
	err := CheckSDPSize(strings.Repeat("x", 11), 10)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrSDPTooLarge))
}

func TestCheckPeerCount(t *testing.T) {
	require.NoError(t, CheckPeerCount(5, 0)) // unlimited
	require.NoError(t, CheckPeerCount(4, 5))
	err := CheckPeerCount(5, 5)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTooManyPeers))
}

func TestCheckRemoteICECount(t *testing.T) {
	require.NoError(t, CheckRemoteICECount(0, 2))
	require.NoError(t, CheckRemoteICECount(1, 2))
	err := CheckRemoteICECount(2, 2)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTooManyRemoteICE))
}

func TestPeerManager_MaxPeersAdmission(t *testing.T) {
	pm := NewPeerManagerWithAPI(testAPI())
	pm.SetAdmissionLimits(AdmissionLimits{MaxPeers: 1})

	p1, err := pm.CreatePeerConnection()
	require.NoError(t, err)
	defer pm.RemovePeer(p1.ID)

	_, err = pm.CreatePeerConnection()
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTooManyPeers))
	assert.Equal(t, 1, pm.Count())
}

func TestPeerConn_SDPAdmission(t *testing.T) {
	pm := NewPeerManagerWithAPI(testAPI())
	pm.SetAdmissionLimits(AdmissionLimits{MaxSDPBytes: 32})

	peer, err := pm.CreatePeerConnection()
	require.NoError(t, err)
	defer pm.RemovePeer(peer.ID)

	huge := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  strings.Repeat("v=0\r\n", 20),
	}
	_, err = peer.HandleOffer(huge)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrSDPTooLarge))
}

func TestPeerConn_RemoteICEAdmission(t *testing.T) {
	pm := NewPeerManagerWithAPI(testAPI())
	pm.SetAdmissionLimits(AdmissionLimits{MaxRemoteICECandidates: 2})

	peer, err := pm.CreatePeerConnection()
	require.NoError(t, err)
	defer pm.RemovePeer(peer.ID)

	// Without a remote description Pion may error on AddICECandidate; admission
	// must still reject before that once the cap is hit.
	cand := webrtc.ICECandidateInit{
		Candidate: "candidate:1 1 udp 2130706431 192.168.1.1 12345 typ host",
	}

	// First two attempts are admitted (may still fail inside Pion).
	_ = peer.AddICECandidate(cand)
	_ = peer.AddICECandidate(cand)
	assert.Equal(t, 2, peer.RemoteICECount())

	err = peer.AddICECandidate(cand)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTooManyRemoteICE))
	assert.Equal(t, 2, peer.RemoteICECount())
}

func TestPeerManager_CloseAllAndWait(t *testing.T) {
	pm := NewPeerManagerWithAPI(testAPI())

	var closedIDs sync.Map
	pm.OnPeerClosed(func(ev PeerClosedEvent) {
		closedIDs.Store(ev.PeerID, ev.Reason)
	})

	const n = 3
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		p, err := pm.CreatePeerConnection()
		require.NoError(t, err)
		ids = append(ids, p.ID)
	}
	assert.Equal(t, n, pm.Count())

	done := make(chan struct{})
	go func() {
		pm.CloseAllAndWait("shutdown-test")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("CloseAllAndWait did not complete")
	}

	assert.True(t, pm.Closed())
	assert.Equal(t, 0, pm.Count())

	// New peers rejected after close.
	_, err := pm.CreatePeerConnection()
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrPeerManagerClosed))

	for _, id := range ids {
		v, ok := closedIDs.Load(id)
		require.True(t, ok, "missing closed event for %s", id)
		assert.Equal(t, "shutdown-test", v)
	}
}

func TestPeerManager_ObserversCreatedAndClosed(t *testing.T) {
	pm := NewPeerManagerWithAPI(testAPI())

	created := make(chan string, 1)
	closed := make(chan PeerClosedEvent, 1)
	pm.OnPeerCreated(func(p *PeerConn) {
		created <- p.ID
	})
	pm.OnPeerClosed(func(ev PeerClosedEvent) {
		closed <- ev
	})

	peer, err := pm.CreatePeerConnection()
	require.NoError(t, err)

	select {
	case id := <-created:
		assert.Equal(t, peer.ID, id)
	case <-time.After(time.Second):
		t.Fatal("OnPeerCreated not fired")
	}

	pm.RemovePeer(peer.ID)

	select {
	case ev := <-closed:
		assert.Equal(t, peer.ID, ev.PeerID)
		assert.NotNil(t, ev.Peer)
		assert.False(t, ev.At.IsZero())
	case <-time.After(time.Second):
		t.Fatal("OnPeerClosed not fired")
	}

	assert.Equal(t, 0, pm.Count())
}

func TestPeerConn_CloseIdempotent(t *testing.T) {
	pm := NewPeerManagerWithAPI(testAPI())

	var closedCount int
	var mu sync.Mutex
	pm.OnPeerClosed(func(PeerClosedEvent) {
		mu.Lock()
		closedCount++
		mu.Unlock()
	})

	peer, err := pm.CreatePeerConnection()
	require.NoError(t, err)

	require.NoError(t, peer.Close())
	require.NoError(t, peer.Close())
	require.NoError(t, peer.CloseWithReason("again"))

	// Wait for closeWait to drain.
	pm.Wait()

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1, closedCount, "observer must fire once")
}

func TestPeerManager_ICEServersCopy(t *testing.T) {
	pm, err := NewPeerManagerValidated(ICEConfig{
		STUNServers: []string{"stun:stun.l.google.com:19302"},
		TURNServer:  "turn:turn.example.com:3478",
		TURNUser:    "u",
		TURNCred:    "c",
	})
	require.NoError(t, err)

	servers := pm.ICEServers()
	require.Len(t, servers, 2)
	// Mutating returned slice must not affect manager.
	servers[0].URLs[0] = "stun:mutated"
	servers2 := pm.ICEServers()
	assert.Equal(t, "stun:stun.l.google.com:19302", servers2[0].URLs[0])
}
