package crosstalk

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTestConfig is a helper that writes JSON content to a temp file and
// returns the path. The file is automatically cleaned up after the test.
func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestLoadConfig_ValidFullConfig(t *testing.T) {
	path := writeTestConfig(t, `{
		"listen": ":9090",
		"db_path": "/tmp/test.db",
		"recording_path": "/tmp/recordings",
		"log_level": "debug",
		"webrtc": {
			"stun_servers": ["stun:stun1.example.com:3478"],
			"turn": {
				"enabled": true,
				"server": "turn:turn.example.com:3478",
				"username": "user",
				"credential": "pass"
			}
		},
		"auth": {
			"session_secret": "test-secret",
			"webrtc_token_lifetime": "1h"
		},
		"web": {
			"dev_mode": true,
			"dev_proxy_url": "http://localhost:3000"
		}
	}`)

	cfg, err := LoadConfig(path)
	require.NoError(t, err)

	assert.Equal(t, ":9090", cfg.Listen)
	assert.Equal(t, "/tmp/test.db", cfg.DBPath)
	assert.Equal(t, "/tmp/recordings", cfg.RecordingPath)
	assert.Equal(t, "debug", cfg.LogLevel)

	// WebRTC
	assert.Equal(t, []string{"stun:stun1.example.com:3478"}, cfg.WebRTC.STUNServers)
	assert.True(t, cfg.WebRTC.TURN.Enabled)
	assert.Equal(t, "turn:turn.example.com:3478", cfg.WebRTC.TURN.Server)
	assert.Equal(t, "user", cfg.WebRTC.TURN.Username)
	assert.Equal(t, "pass", cfg.WebRTC.TURN.Credential)

	// Auth
	assert.Equal(t, "test-secret", cfg.Auth.SessionSecret)
	assert.Equal(t, "1h", cfg.Auth.WebRTCTokenLifetime)

	// Web
	assert.True(t, cfg.Web.DevMode)
	assert.Equal(t, "http://localhost:3000", cfg.Web.DevProxyURL)
}

func TestLoadConfig_DefaultsApplied(t *testing.T) {
	// Minimal config: only the required field.
	path := writeTestConfig(t, `{
		"auth": {
			"session_secret": "my-secret"
		}
	}`)

	cfg, err := LoadConfig(path)
	require.NoError(t, err)

	assert.Equal(t, ":8080", cfg.Listen)
	assert.Equal(t, "./data/crosstalk.db", cfg.DBPath)
	assert.Equal(t, "./data/recordings", cfg.RecordingPath)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, []string{"stun:stun.l.google.com:19302"}, cfg.WebRTC.STUNServers)
	assert.False(t, cfg.WebRTC.TURN.Enabled)
	assert.Equal(t, "24h", cfg.Auth.WebRTCTokenLifetime)
	assert.False(t, cfg.Web.DevMode)
	assert.Equal(t, "http://localhost:5173", cfg.Web.DevProxyURL)
}

func TestLoadConfig_UnknownFieldsWarn(t *testing.T) {
	path := writeTestConfig(t, `{
		"auth": { "session_secret": "s" },
		"unknown_top": true,
		"webrtc": {
			"unknown_webrtc": 42,
			"turn": {
				"unknown_turn": "x"
			}
		},
		"web": { "unknown_web": 1 }
	}`)

	// Capture slog output to verify warnings are emitted.
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	slog.SetDefault(logger)
	t.Cleanup(func() {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	})

	cfg, err := LoadConfig(path)
	require.NoError(t, err)

	// Config should still load successfully.
	assert.Equal(t, "s", cfg.Auth.SessionSecret)

	// Verify warnings were logged for each unknown field.
	output := buf.String()
	assert.Contains(t, output, "unknown_top")
	assert.Contains(t, output, "unknown_webrtc")
	assert.Contains(t, output, "unknown_turn")
	assert.Contains(t, output, "unknown_web")
}

func TestLoadConfig_MissingRequiredSessionSecret(t *testing.T) {
	path := writeTestConfig(t, `{
		"listen": ":9090"
	}`)

	_, err := LoadConfig(path)
	require.Error(t, err)

	var cfgErr *ConfigError
	require.ErrorAs(t, err, &cfgErr)
	assert.Equal(t, "auth.session_secret", cfgErr.Field)
	assert.Contains(t, cfgErr.Message, "required")
}

func TestLoadConfig_InvalidLogLevel(t *testing.T) {
	path := writeTestConfig(t, `{
		"auth": { "session_secret": "s" },
		"log_level": "trace"
	}`)

	_, err := LoadConfig(path)
	require.Error(t, err)

	var cfgErr *ConfigError
	require.ErrorAs(t, err, &cfgErr)
	assert.Equal(t, "log_level", cfgErr.Field)
	assert.Contains(t, cfgErr.Message, "trace")
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	path := writeTestConfig(t, `{not valid json}`)

	_, err := LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing config JSON")
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading config file")
}

func TestLoadConfig_SchemaFieldIgnored(t *testing.T) {
	// The "$schema" key should not produce a warning.
	path := writeTestConfig(t, `{
		"$schema": "./config.schema.json",
		"auth": { "session_secret": "s" }
	}`)

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	slog.SetDefault(logger)
	t.Cleanup(func() {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	})

	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, "s", cfg.Auth.SessionSecret)

	// No warnings should be logged.
	assert.Empty(t, buf.String())
}

func TestLoadConfig_DevJSON(t *testing.T) {
	// Verify the project's dev.json loads correctly.
	// Find dev.json relative to this test file's package directory.
	path := "dev.json"

	cfg, err := LoadConfig(path)
	require.NoError(t, err)

	assert.Equal(t, ":8080", cfg.Listen)
	assert.Equal(t, "/data/crosstalk.db", cfg.DBPath)
	assert.Equal(t, "/data/recordings", cfg.RecordingPath)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, "dev-secret-change-in-production", cfg.Auth.SessionSecret)
	assert.True(t, cfg.Web.DevMode)
	assert.Equal(t, "http://192.168.1.100:5173", cfg.Web.DevProxyURL)
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, ":8080", cfg.Listen)
	assert.Equal(t, "./data/crosstalk.db", cfg.DBPath)
	assert.Equal(t, "./data/recordings", cfg.RecordingPath)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, []string{"stun:stun.l.google.com:19302"}, cfg.WebRTC.STUNServers)
	assert.False(t, cfg.WebRTC.TURN.Enabled)
	assert.Equal(t, "", cfg.Auth.SessionSecret)
	assert.Equal(t, "24h", cfg.Auth.WebRTCTokenLifetime)
	assert.False(t, cfg.Web.DevMode)
	assert.Equal(t, "http://localhost:5173", cfg.Web.DevProxyURL)
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"INFO", slog.LevelInfo},   // case insensitive
		{"", slog.LevelInfo},       // empty defaults to info
		{"unknown", slog.LevelInfo}, // unknown defaults to info
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, ParseLogLevel(tt.input))
		})
	}
}

func TestLoadConfig_PartialOverride(t *testing.T) {
	// Override only some fields; others keep defaults.
	path := writeTestConfig(t, `{
		"listen": ":3000",
		"log_level": "warn",
		"auth": {
			"session_secret": "partial-secret"
		}
	}`)

	cfg, err := LoadConfig(path)
	require.NoError(t, err)

	// Overridden values.
	assert.Equal(t, ":3000", cfg.Listen)
	assert.Equal(t, "warn", cfg.LogLevel)
	assert.Equal(t, "partial-secret", cfg.Auth.SessionSecret)

	// Defaults preserved.
	assert.Equal(t, "./data/crosstalk.db", cfg.DBPath)
	assert.Equal(t, "./data/recordings", cfg.RecordingPath)
	assert.Equal(t, "24h", cfg.Auth.WebRTCTokenLifetime)
	assert.Equal(t, "http://localhost:5173", cfg.Web.DevProxyURL)
}
