package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	crosstalk "github.com/aleksclark/crosstalk/cli"
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

// mockPWService implements crosstalk.PipeWireService for testing.
type mockPWService struct{}

func (m *mockPWService) Discover() ([]crosstalk.Source, []crosstalk.Sink, error) {
	return nil, nil, nil
}
