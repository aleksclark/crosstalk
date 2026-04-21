package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_FromFile(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "crosstalk.json")
	err := os.WriteFile(cfgFile, []byte(`{
		"server_url": "https://example.com",
		"token": "test-token-123",
		"source_name": "mic-1",
		"sink_name": "speakers-1",
		"log_level": "debug"
	}`), 0644)
	require.NoError(t, err)

	// Clear env vars that might interfere
	t.Setenv("CROSSTALK_SERVER", "")
	t.Setenv("CROSSTALK_TOKEN", "")
	t.Setenv("CROSSTALK_SOURCE_NAME", "")
	t.Setenv("CROSSTALK_SINK_NAME", "")
	t.Setenv("CROSSTALK_LOG_LEVEL", "")
	t.Setenv("CROSSTALK_CONFIG", cfgFile)

	// Reset global flag
	old := configPath
	configPath = ""
	defer func() { configPath = old }()

	cfg, err := loadConfig()
	require.NoError(t, err)

	assert.Equal(t, "https://example.com", cfg.ServerURL)
	assert.Equal(t, "test-token-123", cfg.Token)
	assert.Equal(t, "mic-1", cfg.SourceName)
	assert.Equal(t, "speakers-1", cfg.SinkName)
	assert.Equal(t, "debug", cfg.LogLevel)
}

func TestLoadConfig_EnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "crosstalk.json")
	err := os.WriteFile(cfgFile, []byte(`{
		"server_url": "https://from-file.com",
		"token": "file-token",
		"source_name": "file-source",
		"sink_name": "file-sink",
		"log_level": "info"
	}`), 0644)
	require.NoError(t, err)

	t.Setenv("CROSSTALK_CONFIG", cfgFile)
	t.Setenv("CROSSTALK_SERVER", "https://from-env.com")
	t.Setenv("CROSSTALK_TOKEN", "env-token")
	t.Setenv("CROSSTALK_SOURCE_NAME", "env-source")
	t.Setenv("CROSSTALK_SINK_NAME", "env-sink")
	t.Setenv("CROSSTALK_LOG_LEVEL", "error")

	old := configPath
	configPath = ""
	defer func() { configPath = old }()

	cfg, err := loadConfig()
	require.NoError(t, err)

	// Env vars should win
	assert.Equal(t, "https://from-env.com", cfg.ServerURL)
	assert.Equal(t, "env-token", cfg.Token)
	assert.Equal(t, "env-source", cfg.SourceName)
	assert.Equal(t, "env-sink", cfg.SinkName)
	assert.Equal(t, "error", cfg.LogLevel)
}

func TestLoadConfig_EnvOnlyNoFile(t *testing.T) {
	t.Setenv("CROSSTALK_CONFIG", "")
	t.Setenv("CROSSTALK_SERVER", "https://env-only.com")
	t.Setenv("CROSSTALK_TOKEN", "env-only-token")
	t.Setenv("CROSSTALK_SOURCE_NAME", "")
	t.Setenv("CROSSTALK_SINK_NAME", "")
	t.Setenv("CROSSTALK_LOG_LEVEL", "")

	old := configPath
	configPath = ""
	defer func() { configPath = old }()

	// Change to a temp dir where no crosstalk.json exists
	origDir, _ := os.Getwd()
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))
	defer func() { _ = os.Chdir(origDir) }()

	cfg, err := loadConfig()
	require.NoError(t, err)

	assert.Equal(t, "https://env-only.com", cfg.ServerURL)
	assert.Equal(t, "env-only-token", cfg.Token)
	assert.Equal(t, "info", cfg.LogLevel) // default
}

func TestLoadConfig_MissingServerURL(t *testing.T) {
	t.Setenv("CROSSTALK_CONFIG", "")
	t.Setenv("CROSSTALK_SERVER", "")
	t.Setenv("CROSSTALK_TOKEN", "some-token")
	t.Setenv("CROSSTALK_SOURCE_NAME", "")
	t.Setenv("CROSSTALK_SINK_NAME", "")
	t.Setenv("CROSSTALK_LOG_LEVEL", "")

	old := configPath
	configPath = ""
	defer func() { configPath = old }()

	origDir, _ := os.Getwd()
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))
	defer func() { _ = os.Chdir(origDir) }()

	_, err := loadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server_url")
}

func TestLoadConfig_MissingToken(t *testing.T) {
	t.Setenv("CROSSTALK_CONFIG", "")
	t.Setenv("CROSSTALK_SERVER", "https://example.com")
	t.Setenv("CROSSTALK_TOKEN", "")
	t.Setenv("CROSSTALK_SOURCE_NAME", "")
	t.Setenv("CROSSTALK_SINK_NAME", "")
	t.Setenv("CROSSTALK_LOG_LEVEL", "")

	old := configPath
	configPath = ""
	defer func() { configPath = old }()

	origDir, _ := os.Getwd()
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))
	defer func() { _ = os.Chdir(origDir) }()

	_, err := loadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token")
}

func TestLoadConfig_InvalidLogLevel(t *testing.T) {
	t.Setenv("CROSSTALK_CONFIG", "")
	t.Setenv("CROSSTALK_SERVER", "https://example.com")
	t.Setenv("CROSSTALK_TOKEN", "valid-token")
	t.Setenv("CROSSTALK_SOURCE_NAME", "")
	t.Setenv("CROSSTALK_SINK_NAME", "")
	t.Setenv("CROSSTALK_LOG_LEVEL", "invalid-level")

	old := configPath
	configPath = ""
	defer func() { configPath = old }()

	origDir, _ := os.Getwd()
	dir := t.TempDir()
	require.NoError(t, os.Chdir(dir))
	defer func() { _ = os.Chdir(origDir) }()

	_, err := loadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "log_level")
}

func TestLoadConfig_ExplicitConfigFileMissing(t *testing.T) {
	t.Setenv("CROSSTALK_CONFIG", "/nonexistent/config.json")
	t.Setenv("CROSSTALK_SERVER", "")
	t.Setenv("CROSSTALK_TOKEN", "")

	old := configPath
	configPath = ""
	defer func() { configPath = old }()

	_, err := loadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading config file")
}

func TestLoadConfig_FlagOverridesEnv(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "custom-config.json")
	err := os.WriteFile(cfgFile, []byte(`{
		"server_url": "https://flag-config.com",
		"token": "flag-token"
	}`), 0644)
	require.NoError(t, err)

	t.Setenv("CROSSTALK_CONFIG", "")
	t.Setenv("CROSSTALK_SERVER", "")
	t.Setenv("CROSSTALK_TOKEN", "")
	t.Setenv("CROSSTALK_SOURCE_NAME", "")
	t.Setenv("CROSSTALK_SINK_NAME", "")
	t.Setenv("CROSSTALK_LOG_LEVEL", "")

	old := configPath
	configPath = cfgFile
	defer func() { configPath = old }()

	cfg, err := loadConfig()
	require.NoError(t, err)

	assert.Equal(t, "https://flag-config.com", cfg.ServerURL)
	assert.Equal(t, "flag-token", cfg.Token)
}

func TestParseSlogLevel(t *testing.T) {
	assert.Equal(t, slog.LevelDebug, parseSlogLevel("debug"))
	assert.Equal(t, slog.LevelInfo, parseSlogLevel("info"))
	assert.Equal(t, slog.LevelWarn, parseSlogLevel("warn"))
	assert.Equal(t, slog.LevelError, parseSlogLevel("error"))
	assert.Equal(t, slog.LevelInfo, parseSlogLevel("unknown"))
}
