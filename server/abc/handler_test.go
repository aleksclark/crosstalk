package abc_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	crosstalk "github.com/aleksclark/crosstalk/server"
	"github.com/aleksclark/crosstalk/server/abc"
	crosstalkv2 "github.com/aleksclark/crosstalk/server/proto/v2"
	"github.com/aleksclark/crosstalk/server/webrtc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockABCService implements crosstalk.ABCService for testing.
type mockABCService struct {
	mu   sync.Mutex
	abcs map[string]*crosstalk.ABC
}

func newMockABCService() *mockABCService {
	return &mockABCService{abcs: make(map[string]*crosstalk.ABC)}
}

func (m *mockABCService) List(_ context.Context) ([]crosstalk.ABC, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []crosstalk.ABC
	for _, a := range m.abcs {
		result = append(result, *a)
	}
	return result, nil
}

func (m *mockABCService) ListPage(_ context.Context, _ crosstalk.ListQuery) (crosstalk.ABCPage, error) {
	list, err := m.List(context.Background())
	if err != nil {
		return crosstalk.ABCPage{}, err
	}
	items := make([]crosstalk.ABCListItem, 0, len(list))
	for _, a := range list {
		items = append(items, crosstalk.ABCListItem{ABC: a})
	}
	return crosstalk.ABCPage{Items: items}, nil
}

func (m *mockABCService) Get(_ context.Context, id string) (*crosstalk.ABC, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.abcs[id]
	if !ok {
		return nil, fmt.Errorf("abc not found: %s", id)
	}
	return a, nil
}

func (m *mockABCService) Create(_ context.Context, a *crosstalk.ABC) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.abcs[a.ID] = a
	return nil
}

func (m *mockABCService) Update(_ context.Context, a *crosstalk.ABC) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.abcs[a.ID]; !ok {
		return fmt.Errorf("abc not found: %s", a.ID)
	}
	m.abcs[a.ID] = a
	return nil
}

func (m *mockABCService) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.abcs, id)
	return nil
}

func (m *mockABCService) GetByTokenHash(_ context.Context, tokenHash string) (*crosstalk.ABC, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.abcs {
		if a.TokenHash == tokenHash {
			return a, nil
		}
	}
	return nil, fmt.Errorf("abc not found for token")
}

// mockSessionService implements crosstalk.SessionService for testing.
type mockSessionService struct {
	sessions map[string]*crosstalk.Session
}

func newMockSessionService() *mockSessionService {
	return &mockSessionService{sessions: make(map[string]*crosstalk.Session)}
}

func (m *mockSessionService) List(_ context.Context) ([]crosstalk.Session, error) {
	return nil, nil
}

func (m *mockSessionService) ListPage(_ context.Context, _ crosstalk.ListQuery) (crosstalk.SessionPage, error) {
	return crosstalk.SessionPage{Items: []crosstalk.Session{}}, nil
}

func (m *mockSessionService) Get(_ context.Context, id string) (*crosstalk.Session, error) {
	s, ok := m.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", id)
	}
	return s, nil
}

func (m *mockSessionService) Create(_ context.Context, s *crosstalk.Session) error {
	m.sessions[s.ID] = s
	return nil
}

func (m *mockSessionService) Update(_ context.Context, s *crosstalk.Session) error {
	m.sessions[s.ID] = s
	return nil
}

func (m *mockSessionService) Delete(_ context.Context, id string) error {
	delete(m.sessions, id)
	return nil
}

func (m *mockSessionService) GetByBroadcastToken(_ context.Context, _ string) (*crosstalk.Session, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockSessionService) RegenerateBroadcastToken(_ context.Context, _ string) (string, error) {
	return "", fmt.Errorf("not implemented")
}

func (m *mockSessionService) TransitionState(_ context.Context, id string, to crosstalk.SessionState, _ *uint64) error {
	s, ok := m.sessions[id]
	if !ok {
		return fmt.Errorf("session not found: %s", id)
	}
	s.State = to
	return nil
}

func TestAuthenticate_ValidToken(t *testing.T) {
	abcSvc := newMockABCService()
	sessionSvc := newMockSessionService()
	pm := webrtc.NewPeerManagerWithAPI(nil)

	// Create an ABC with a known token hash.
	token := "test-abc-token-12345"
	hash := abc.HashToken(token)
	testABC := &crosstalk.ABC{
		ID:        "abc-001",
		Name:      "Booth A",
		TokenHash: hash,
		CreatedAt: time.Now(),
	}
	_ = abcSvc.Create(context.Background(), testABC)

	handler := abc.NewHandler(abcSvc, sessionSvc, pm, "1.0.0")

	result, err := handler.Authenticate(context.Background(), token)
	require.NoError(t, err)
	assert.Equal(t, "abc-001", result.ID)
	assert.Equal(t, "Booth A", result.Name)
}

func TestAuthenticate_InvalidToken(t *testing.T) {
	abcSvc := newMockABCService()
	sessionSvc := newMockSessionService()
	pm := webrtc.NewPeerManagerWithAPI(nil)

	handler := abc.NewHandler(abcSvc, sessionSvc, pm, "1.0.0")

	_, err := handler.Authenticate(context.Background(), "bad-token")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}

func TestAuthenticate_EmptyToken(t *testing.T) {
	abcSvc := newMockABCService()
	sessionSvc := newMockSessionService()
	pm := webrtc.NewPeerManagerWithAPI(nil)

	handler := abc.NewHandler(abcSvc, sessionSvc, pm, "1.0.0")

	_, err := handler.Authenticate(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty token")
}

func TestOnConnect_UpdatesState(t *testing.T) {
	abcSvc := newMockABCService()
	sessionSvc := newMockSessionService()
	pm := webrtc.NewPeerManagerWithAPI(nil)

	testABC := &crosstalk.ABC{
		ID:        "abc-001",
		Name:      "Booth A",
		TokenHash: "hash123",
		Connected: false,
		CreatedAt: time.Now(),
	}
	_ = abcSvc.Create(context.Background(), testABC)

	handler := abc.NewHandler(abcSvc, sessionSvc, pm, "1.0.0")

	// Create a mock peer (just need ID for tracking).
	peer := &webrtc.PeerConn{
		ID:        "peer-001",
		CreatedAt: time.Now(),
	}

	err := handler.OnConnect(context.Background(), peer, testABC)
	require.NoError(t, err)

	// Verify connection state was updated.
	updated, err := abcSvc.Get(context.Background(), "abc-001")
	require.NoError(t, err)
	assert.True(t, updated.Connected)
	assert.NotNil(t, updated.LastSeen)
}

func TestOnConnect_WithSessionAssignment(t *testing.T) {
	abcSvc := newMockABCService()
	sessionSvc := newMockSessionService()
	pm := webrtc.NewPeerManagerWithAPI(nil)

	sessionID := "session-001"
	testABC := &crosstalk.ABC{
		ID:        "abc-002",
		Name:      "Booth B",
		TokenHash: "hash456",
		SessionID: &sessionID,
		Connected: false,
		CreatedAt: time.Now(),
	}
	_ = abcSvc.Create(context.Background(), testABC)
	_ = sessionSvc.Create(context.Background(), &crosstalk.Session{
		ID:   sessionID,
		Name: "Test Session",
	})

	handler := abc.NewHandler(abcSvc, sessionSvc, pm, "1.0.0")

	// Create a mock peer - use a real PeerConn without a real PC.
	// The SendControlMessage will fail because there's no data channel,
	// but it shouldn't panic.
	peer := &webrtc.PeerConn{
		ID:        "peer-002",
		CreatedAt: time.Now(),
	}

	// OnConnect with a session assignment - it will attempt to send but
	// won't error because SendControlMessage on a nil data channel just returns nil.
	err := handler.OnConnect(context.Background(), peer, testABC)
	require.NoError(t, err)

	// Verify the ABC is tracked.
	abc2, ok := handler.GetABCForPeer("peer-002")
	assert.True(t, ok)
	assert.Equal(t, "abc-002", abc2.ID)
}

func TestOnDisconnect_UpdatesState(t *testing.T) {
	abcSvc := newMockABCService()
	sessionSvc := newMockSessionService()
	pm := webrtc.NewPeerManagerWithAPI(nil)

	testABC := &crosstalk.ABC{
		ID:        "abc-003",
		Name:      "Booth C",
		TokenHash: "hash789",
		Connected: false,
		CreatedAt: time.Now(),
	}
	_ = abcSvc.Create(context.Background(), testABC)

	handler := abc.NewHandler(abcSvc, sessionSvc, pm, "1.0.0")

	peer := &webrtc.PeerConn{
		ID:        "peer-003",
		CreatedAt: time.Now(),
	}

	// Connect first.
	err := handler.OnConnect(context.Background(), peer, testABC)
	require.NoError(t, err)

	// Disconnect.
	handler.OnDisconnect(context.Background(), "peer-003")

	// Verify disconnection state.
	updated, err := abcSvc.Get(context.Background(), "abc-003")
	require.NoError(t, err)
	assert.False(t, updated.Connected)
}

func TestSendRestart_NotConnected(t *testing.T) {
	abcSvc := newMockABCService()
	sessionSvc := newMockSessionService()
	pm := webrtc.NewPeerManagerWithAPI(nil)

	handler := abc.NewHandler(abcSvc, sessionSvc, pm, "1.0.0")

	err := handler.SendRestart(context.Background(), "abc-nonexistent", "test restart")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestOnSourceStatus_UpdatesLastSeen(t *testing.T) {
	abcSvc := newMockABCService()
	sessionSvc := newMockSessionService()
	pm := webrtc.NewPeerManagerWithAPI(nil)

	testABC := &crosstalk.ABC{
		ID:        "abc-004",
		Name:      "Booth D",
		TokenHash: "hashxyz",
		Connected: false,
		CreatedAt: time.Now(),
	}
	_ = abcSvc.Create(context.Background(), testABC)

	handler := abc.NewHandler(abcSvc, sessionSvc, pm, "1.0.0")

	peer := &webrtc.PeerConn{
		ID:        "peer-004",
		CreatedAt: time.Now(),
	}

	// Connect.
	err := handler.OnConnect(context.Background(), peer, testABC)
	require.NoError(t, err)

	// Send source status.
	status := &crosstalkv2.SourceStatus{
		SourceName: "mic1",
		Active:     true,
		PeakLevel:  0.75,
	}
	handler.OnSourceStatus(context.Background(), "peer-004", status)

	// Verify LastSeen was updated.
	updated, err := abcSvc.Get(context.Background(), "abc-004")
	require.NoError(t, err)
	assert.NotNil(t, updated.LastSeen)
}

func TestHashToken(t *testing.T) {
	token := "my-secret-token"
	hash := abc.HashToken(token)
	assert.Len(t, hash, 64) // SHA-256 hex = 64 chars

	// Same input produces same hash.
	hash2 := abc.HashToken(token)
	assert.Equal(t, hash, hash2)

	// Different input produces different hash.
	hash3 := abc.HashToken("different-token")
	assert.NotEqual(t, hash, hash3)
}
