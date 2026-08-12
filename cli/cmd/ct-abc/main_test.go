package main

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	crosstalk "github.com/aleksclark/crosstalk/cli"
	"github.com/aleksclark/crosstalk/cli/audioctl"
	"github.com/aleksclark/crosstalk/cli/protov2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	// Create a temp config file.
	dir := t.TempDir()
	path := filepath.Join(dir, "test-config.json")
	content := `{
		"server_url": "wss://example.com/ws",
		"token": "abc-token-123",
		"source_name": "my-mic",
		"sink_name": "my-speaker",
		"log_level": "debug"
	}`
	err := os.WriteFile(path, []byte(content), 0644)
	require.NoError(t, err)

	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, "wss://example.com/ws", cfg.ServerURL)
	assert.Equal(t, "abc-token-123", cfg.Token)
	assert.Equal(t, "my-mic", cfg.SourceName)
	assert.Equal(t, "my-speaker", cfg.SinkName)
	assert.Equal(t, "debug", cfg.LogLevel)
}

func TestLoadConfig_DefaultLogLevel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "minimal.json")
	content := `{"server_url": "ws://localhost:8080", "token": "tok"}`
	err := os.WriteFile(path, []byte(content), 0644)
	require.NoError(t, err)

	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, "info", cfg.LogLevel)
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading config file")
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	err := os.WriteFile(path, []byte("not json"), 0644)
	require.NoError(t, err)

	_, err = LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing config file")
}

func TestABCConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ABCConfig
		wantErr string
	}{
		{
			name:    "valid",
			cfg:     ABCConfig{ServerURL: "ws://localhost", Token: "tok"},
			wantErr: "",
		},
		{
			name:    "missing server_url",
			cfg:     ABCConfig{Token: "tok"},
			wantErr: "server_url",
		},
		{
			name:    "missing token",
			cfg:     ABCConfig{ServerURL: "ws://localhost"},
			wantErr: "token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestABCConfig_ToCLIConfig(t *testing.T) {
	cfg := &ABCConfig{
		ServerURL:  "ws://localhost:8080",
		Token:      "test-token",
		SourceName: "mic1",
		SinkName:   "speaker1",
		LogLevel:   "debug",
	}

	cliCfg := cfg.ToCLIConfig()
	assert.Equal(t, cfg.ServerURL, cliCfg.ServerURL)
	assert.Equal(t, cfg.Token, cliCfg.Token)
	assert.Equal(t, cfg.SourceName, cliCfg.SourceName)
	assert.Equal(t, cfg.SinkName, cliCfg.SinkName)
	assert.Equal(t, cfg.LogLevel, cliCfg.LogLevel)
}

func TestCalculateBackoff(t *testing.T) {
	initial := 1 * time.Second
	max := 60 * time.Second
	factor := 2.0

	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 1 * time.Second},  // 1 * 2^0 = 1
		{2, 2 * time.Second},  // 1 * 2^1 = 2
		{3, 4 * time.Second},  // 1 * 2^2 = 4
		{4, 8 * time.Second},  // 1 * 2^3 = 8
		{7, 60 * time.Second}, // 1 * 2^6 = 64, capped at 60
	}

	for _, tt := range tests {
		got := CalculateBackoff(tt.attempt, initial, max, factor)
		assert.Equal(t, tt.want, got, "attempt=%d", tt.attempt)
	}
}

func TestNewABCClient(t *testing.T) {
	cfg := &ABCConfig{
		ServerURL: "ws://localhost:8080",
		Token:     "test",
	}

	client := NewABCClient(cfg, &mockPWService{})
	assert.NotNil(t, client)
	assert.Equal(t, cfg, client.cfg)
}

func TestWriteManagedMarker(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, writeManagedMarker(dir))
	path := ManagedMarkerPath(dir)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "managed_at=")
}

func TestProtoCommandToAudioctl(t *testing.T) {
	vol := uint32(0)
	muted := false
	gain := uint32(100)
	cmd := &protov2.AudioControlCommand{
		CommandID:       "abc-audio/x/1",
		DesiredRevision: 1,
		Output: &protov2.AudioOutputDesired{
			DeviceUID:     "usb:0d8c:0014:serial:S1",
			VolumePercent: &vol,
			Muted:         &muted,
		},
		Input: &protov2.AudioInputDesired{
			DeviceUID:   "usb:0d8c:0014:serial:S1",
			GainPercent: &gain,
		},
	}
	acmd, err := protoCommandToAudioctl(cmd)
	require.NoError(t, err)
	require.NotNil(t, acmd.Output.VolumePercent)
	assert.Equal(t, 0, *acmd.Output.VolumePercent)
	require.NotNil(t, acmd.Output.Muted)
	assert.False(t, *acmd.Output.Muted)
	require.NotNil(t, acmd.Input.GainPercent)
	assert.Equal(t, 100, *acmd.Input.GainPercent)
}

func TestHandleAudioCommand_InvalidNoApply(t *testing.T) {
	ctrl := &mockAudioController{}
	client := NewABCClient(&ABCConfig{ServerURL: "ws://x", Token: "t"}, &mockPWService{})
	stateDir := t.TempDir()
	client.SetAudioController(ctrl, stateDir)

	var sent [][]byte
	var mu sync.Mutex
	send := func(b []byte) error {
		mu.Lock()
		defer mu.Unlock()
		sent = append(sent, append([]byte(nil), b...))
		return nil
	}

	// Empty command → invalid, controller Apply must not be called.
	client.handleAudioCommand(context.Background(), &protov2.AudioControlCommand{
		CommandID:       "c",
		DesiredRevision: 1,
	}, send)
	assert.Equal(t, 0, ctrl.applyCalls)
	mu.Lock()
	require.NotEmpty(t, sent)
	mu.Unlock()
	// Marker should not be written for invalid
	_, err := os.Stat(ManagedMarkerPath(stateDir))
	assert.True(t, os.IsNotExist(err))
}

func TestHandleAudioCommand_ApplyWritesMarker(t *testing.T) {
	ctrl := &mockAudioController{
		applyRep: &audioctl.Report{
			CommandID:       "c",
			DesiredRevision: 2,
			Output:          &audioctl.OutputObserved{VolumeState: audioctl.StateApplied},
			Input:           &audioctl.InputObserved{GainState: audioctl.StateApplied},
		},
	}
	client := NewABCClient(&ABCConfig{ServerURL: "ws://x", Token: "t"}, &mockPWService{})
	stateDir := t.TempDir()
	client.SetAudioController(ctrl, stateDir)

	var sent int
	send := func([]byte) error { sent++; return nil }
	vol := uint32(25)
	client.handleAudioCommand(context.Background(), &protov2.AudioControlCommand{
		CommandID:       "c",
		DesiredRevision: 2,
		Output: &protov2.AudioOutputDesired{
			DeviceUID:     "usb:0d8c:0014:serial:S1",
			VolumePercent: &vol,
		},
	}, send)
	assert.Equal(t, 1, ctrl.applyCalls)
	assert.Equal(t, 1, sent)
	_, err := os.Stat(ManagedMarkerPath(stateDir))
	require.NoError(t, err)
}

func TestAudioCommandLoopStopsOnCancel(t *testing.T) {
	client := NewABCClient(&ABCConfig{ServerURL: "ws://x", Token: "t"}, &mockPWService{})
	client.SetAudioController(&mockAudioController{}, t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan *protov2.AudioControlCommand)
	done := make(chan struct{})
	go func() {
		client.audioCommandLoop(ctx, ch, func([]byte) error { return nil })
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("command loop did not stop")
	}
}

func TestAudioHeartbeatLoopStopsOnCancel(t *testing.T) {
	client := NewABCClient(&ABCConfig{ServerURL: "ws://x", Token: "t"}, &mockPWService{})
	client.SetAudioController(&mockAudioController{}, t.TempDir())
	client.heartbeatEvery = 50 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		client.audioHeartbeatLoop(ctx, func([]byte) error { return nil })
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("heartbeat loop did not stop")
	}
}

// mockPWService implements crosstalk.PipeWireService for testing.
type mockPWService struct{}

func (m *mockPWService) Discover() ([]crosstalk.Source, []crosstalk.Sink, error) {
	return nil, nil, nil
}

type mockAudioController struct {
	applyCalls int
	applyRep   *audioctl.Report
}

func (m *mockAudioController) Inventory(ctx context.Context) (*audioctl.Report, error) {
	return &audioctl.Report{}, nil
}

func (m *mockAudioController) Apply(ctx context.Context, cmd audioctl.Command) (*audioctl.Report, error) {
	m.applyCalls++
	if m.applyRep != nil {
		return m.applyRep, nil
	}
	return &audioctl.Report{CommandID: cmd.CommandID, DesiredRevision: cmd.DesiredRevision}, nil
}

func (m *mockAudioController) Readback(ctx context.Context) (*audioctl.Report, error) {
	return &audioctl.Report{}, nil
}
