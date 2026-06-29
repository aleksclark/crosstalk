package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	crosstalk "github.com/aleksclark/crosstalk/server"
	"github.com/aleksclark/crosstalk/server/mixer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockMixStore is a test double for MixStore.
type mockMixStore struct {
	mu      sync.Mutex
	entries map[string][]crosstalk.MixEntry // channelID -> entries
}

func newMockMixStore() *mockMixStore {
	return &mockMixStore{entries: make(map[string][]crosstalk.MixEntry)}
}

func (m *mockMixStore) GetMix(_ context.Context, channelID string) ([]crosstalk.MixEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.entries[channelID], nil
}

func (m *mockMixStore) SetMix(_ context.Context, channelID string, entries []crosstalk.MixEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[channelID] = entries
	return nil
}

// mockChannelStore is a test double for ChannelStore.
type mockChannelStore struct {
	channels []crosstalk.Channel
}

func (m *mockChannelStore) List(_ context.Context, _ string) ([]crosstalk.Channel, error) {
	return m.channels, nil
}

// mockSourceStore is a test double for SourceStore.
type mockSourceStore struct {
	mu      sync.Mutex
	sources map[string]*crosstalk.Source
}

func newMockSourceStore() *mockSourceStore {
	return &mockSourceStore{sources: make(map[string]*crosstalk.Source)}
}

func (m *mockSourceStore) List(_ context.Context, _ string) ([]crosstalk.Source, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []crosstalk.Source
	for _, s := range m.sources {
		result = append(result, *s)
	}
	return result, nil
}

func (m *mockSourceStore) Update(_ context.Context, s *crosstalk.Source) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sources[s.ID] = s
	return nil
}

func setupOrchestrator(t *testing.T, channels []crosstalk.Channel) (*SessionOrchestrator, *mockMixStore, *mockSourceStore) {
	t.Helper()

	mixStore := newMockMixStore()
	channelStore := &mockChannelStore{channels: channels}
	sourceStore := newMockSourceStore()

	orch := New(Config{
		SessionID:    "session-1",
		MixStore:     mixStore,
		ChannelStore: channelStore,
		SourceStore:  sourceStore,
	})

	err := orch.Initialize(context.Background())
	require.NoError(t, err)

	return orch, mixStore, sourceStore
}

func TestOrchestrator_Initialize(t *testing.T) {
	channels := []crosstalk.Channel{
		{ID: "ch1", SessionID: "session-1", Name: "Feed", Type: crosstalk.ChannelFeed},
		{ID: "ch2", SessionID: "session-1", Name: "Broadcast", Type: crosstalk.ChannelBroadcast},
	}

	orch, _, _ := setupOrchestrator(t, channels)
	assert.Equal(t, 2, orch.ChannelCount())
}

func TestOrchestrator_SourceConnect(t *testing.T) {
	channels := []crosstalk.Channel{
		{ID: "ch1", SessionID: "session-1", Name: "Feed", Type: crosstalk.ChannelFeed},
	}

	orch, mixStore, _ := setupOrchestrator(t, channels)

	// Pre-populate mix state
	mixStore.SetMix(context.Background(), "ch1", []crosstalk.MixEntry{
		{ID: "me1", ChannelID: "ch1", SourceID: "src1", Level: 0.8, Muted: false},
	})

	source := crosstalk.Source{
		ID:        "src1",
		SessionID: "session-1",
		Name:      "Microphone 1",
		Origin:    crosstalk.OriginABC,
		Connected: true,
	}

	err := orch.SourceConnect(context.Background(), source)
	require.NoError(t, err)

	assert.Equal(t, 1, orch.ActiveSourceCount())

	state := orch.GetSourceState("src1")
	require.NotNil(t, state)
	assert.True(t, state.Active)
	assert.Equal(t, "src1", state.InputIDs["ch1"])
}

func TestOrchestrator_SourceDisconnect_PreservesState(t *testing.T) {
	channels := []crosstalk.Channel{
		{ID: "ch1", SessionID: "session-1", Name: "Feed", Type: crosstalk.ChannelFeed},
	}

	orch, _, sourceStore := setupOrchestrator(t, channels)

	source := crosstalk.Source{
		ID:        "src1",
		SessionID: "session-1",
		Name:      "Mic 1",
		Origin:    crosstalk.OriginABC,
		Connected: true,
	}

	err := orch.SourceConnect(context.Background(), source)
	require.NoError(t, err)
	assert.Equal(t, 1, orch.ActiveSourceCount())

	// Disconnect
	err = orch.SourceDisconnect(context.Background(), "src1")
	require.NoError(t, err)

	// State preserved but inactive
	assert.Equal(t, 0, orch.ActiveSourceCount())
	state := orch.GetSourceState("src1")
	require.NotNil(t, state)
	assert.False(t, state.Active)

	// Source should be updated in store
	stored := sourceStore.sources["src1"]
	require.NotNil(t, stored)
	assert.False(t, stored.Connected)
}

func TestOrchestrator_SourceReconnect(t *testing.T) {
	channels := []crosstalk.Channel{
		{ID: "ch1", SessionID: "session-1", Name: "Feed", Type: crosstalk.ChannelFeed},
	}

	orch, _, _ := setupOrchestrator(t, channels)

	source := crosstalk.Source{
		ID:        "src1",
		SessionID: "session-1",
		Name:      "Mic 1",
		Connected: true,
	}

	// Connect
	err := orch.SourceConnect(context.Background(), source)
	require.NoError(t, err)

	// Disconnect
	err = orch.SourceDisconnect(context.Background(), "src1")
	require.NoError(t, err)
	assert.Equal(t, 0, orch.ActiveSourceCount())

	// Reconnect
	err = orch.SourceConnect(context.Background(), source)
	require.NoError(t, err)
	assert.Equal(t, 1, orch.ActiveSourceCount())

	state := orch.GetSourceState("src1")
	require.NotNil(t, state)
	assert.True(t, state.Active)
}

func TestOrchestrator_UpdateMix(t *testing.T) {
	channels := []crosstalk.Channel{
		{ID: "ch1", SessionID: "session-1", Name: "Feed", Type: crosstalk.ChannelFeed},
	}

	orch, mixStore, _ := setupOrchestrator(t, channels)

	// Connect a source first
	source := crosstalk.Source{
		ID:        "src1",
		SessionID: "session-1",
		Name:      "Mic 1",
		Connected: true,
	}
	err := orch.SourceConnect(context.Background(), source)
	require.NoError(t, err)

	// Update mix
	newEntries := []crosstalk.MixEntry{
		{ID: "me1", ChannelID: "ch1", SourceID: "src1", Level: 0.5, Muted: true},
	}
	err = orch.UpdateMix(context.Background(), "ch1", newEntries)
	require.NoError(t, err)

	// Verify DB was updated
	stored, err := mixStore.GetMix(context.Background(), "ch1")
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.Equal(t, 0.5, stored[0].Level)
	assert.True(t, stored[0].Muted)

	// Verify mixer input was updated
	cs := orch.GetChannelState("ch1")
	require.NotNil(t, cs)
	inp := cs.Mixer.GetInput("src1")
	require.NotNil(t, inp)
	assert.Equal(t, 0.5, inp.Level)
	assert.True(t, inp.Muted)
}

func TestOrchestrator_ForwardOutput(t *testing.T) {
	channels := []crosstalk.Channel{
		{ID: "ch1", SessionID: "session-1", Name: "Feed", Type: crosstalk.ChannelFeed},
	}

	orch, _, _ := setupOrchestrator(t, channels)

	// Connect source
	source := crosstalk.Source{
		ID:        "src1",
		SessionID: "session-1",
		Name:      "Mic 1",
		Connected: true,
	}
	err := orch.SourceConnect(context.Background(), source)
	require.NoError(t, err)

	// Add sink
	var receivedFrames [][]int16
	var mu sync.Mutex
	orch.ForwardOutput("ch1", "sink1", func(channelID string, frame []int16) {
		mu.Lock()
		defer mu.Unlock()
		cp := make([]int16, len(frame))
		copy(cp, frame)
		receivedFrames = append(receivedFrames, cp)
	})

	// Write audio and run mixer
	samples := make([]int16, mixer.FrameSize)
	for i := range samples {
		samples[i] = 1234
	}
	orch.WriteAudio("src1", samples)

	// Start mixers and wait for output
	orch.StartMixers()
	time.Sleep(50 * time.Millisecond)
	orch.StopMixers()

	mu.Lock()
	defer mu.Unlock()
	require.Greater(t, len(receivedFrames), 0, "should have received at least one frame")

	// First frame should contain our audio
	assert.Equal(t, int16(1234), receivedFrames[0][0])
}

func TestOrchestrator_MultipleSinks(t *testing.T) {
	channels := []crosstalk.Channel{
		{ID: "ch1", SessionID: "session-1", Name: "Feed", Type: crosstalk.ChannelFeed},
	}

	orch, _, _ := setupOrchestrator(t, channels)

	// Connect source
	source := crosstalk.Source{
		ID:        "src1",
		SessionID: "session-1",
		Name:      "Mic 1",
		Connected: true,
	}
	err := orch.SourceConnect(context.Background(), source)
	require.NoError(t, err)

	// Add multiple sinks
	var sink1Count, sink2Count int
	var mu sync.Mutex

	orch.ForwardOutput("ch1", "sink1", func(_ string, _ []int16) {
		mu.Lock()
		sink1Count++
		mu.Unlock()
	})
	orch.ForwardOutput("ch1", "sink2", func(_ string, _ []int16) {
		mu.Lock()
		sink2Count++
		mu.Unlock()
	})

	// Write audio
	samples := make([]int16, mixer.FrameSize)
	orch.WriteAudio("src1", samples)

	orch.StartMixers()
	time.Sleep(50 * time.Millisecond)
	orch.StopMixers()

	mu.Lock()
	defer mu.Unlock()
	assert.Greater(t, sink1Count, 0)
	assert.Greater(t, sink2Count, 0)
}

func TestOrchestrator_RemoveSink(t *testing.T) {
	channels := []crosstalk.Channel{
		{ID: "ch1", SessionID: "session-1", Name: "Feed", Type: crosstalk.ChannelFeed},
	}

	orch, _, _ := setupOrchestrator(t, channels)

	var called bool
	orch.ForwardOutput("ch1", "sink1", func(_ string, _ []int16) {
		called = true
	})

	orch.RemoveSink("ch1", "sink1")

	// After removal, sink should not be called
	cs := orch.GetChannelState("ch1")
	require.NotNil(t, cs)
	cs.mu.RLock()
	assert.Empty(t, cs.Sinks)
	cs.mu.RUnlock()
	assert.False(t, called)
}

func TestOrchestrator_WriteAudioInactiveSource(t *testing.T) {
	channels := []crosstalk.Channel{
		{ID: "ch1", SessionID: "session-1", Name: "Feed", Type: crosstalk.ChannelFeed},
	}

	orch, _, _ := setupOrchestrator(t, channels)

	// Writing audio for nonexistent source should not panic
	samples := make([]int16, mixer.FrameSize)
	orch.WriteAudio("nonexistent", samples) // should be a no-op
}

func TestOrchestrator_DisconnectNonexistentSource(t *testing.T) {
	channels := []crosstalk.Channel{
		{ID: "ch1", SessionID: "session-1", Name: "Feed", Type: crosstalk.ChannelFeed},
	}

	orch, _, _ := setupOrchestrator(t, channels)

	// Should not error
	err := orch.SourceDisconnect(context.Background(), "nonexistent")
	assert.NoError(t, err)
}
