package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	crosstalk "github.com/anthropics/crosstalk/cli"
)

var configPath string

func init() {
	flag.StringVar(&configPath, "config", "", "path to config file")
}

// loadConfig loads configuration from a JSON file with environment variable overrides.
// Priority: env vars > config file > defaults.
func loadConfig() (*crosstalk.Config, error) {
	path := configPath
	if path == "" {
		path = os.Getenv("CROSSTALK_CONFIG")
	}
	if path == "" {
		path = "crosstalk.json"
	}

	cfg := &crosstalk.Config{
		LogLevel: "info",
	}

	data, err := os.ReadFile(path)
	if err != nil {
		// If no config file found and no explicit path was given, allow env-only config.
		if os.IsNotExist(err) && configPath == "" && os.Getenv("CROSSTALK_CONFIG") == "" {
			slog.Debug("no config file found, using env vars only", "path", path)
		} else {
			return nil, fmt.Errorf("reading config file %s: %w", path, err)
		}
	} else {
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing config file %s: %w", path, err)
		}
	}

	// Environment variable overrides
	applyEnvOverrides(cfg)

	// Validate required fields
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// applyEnvOverrides applies environment variable overrides to the config.
func applyEnvOverrides(cfg *crosstalk.Config) {
	if v := os.Getenv("CROSSTALK_SERVER"); v != "" {
		cfg.ServerURL = v
	}
	if v := os.Getenv("CROSSTALK_TOKEN"); v != "" {
		cfg.Token = v
	}
	if v := os.Getenv("CROSSTALK_SOURCE_NAME"); v != "" {
		cfg.SourceName = v
	}
	if v := os.Getenv("CROSSTALK_SINK_NAME"); v != "" {
		cfg.SinkName = v
	}
	if v := os.Getenv("CROSSTALK_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
}

// validateConfig checks that required configuration fields are set and valid.
func validateConfig(cfg *crosstalk.Config) error {
	if cfg.ServerURL == "" {
		return &crosstalk.ValidationError{Field: "server_url", Message: "required"}
	}
	if cfg.Token == "" {
		return &crosstalk.ValidationError{Field: "token", Message: "required"}
	}

	// Validate log_level
	switch strings.ToLower(cfg.LogLevel) {
	case "debug", "info", "warn", "error":
		// valid
	default:
		return &crosstalk.ValidationError{
			Field:   "log_level",
			Message: fmt.Sprintf("must be one of: debug, info, warn, error; got %q", cfg.LogLevel),
		}
	}

	return nil
}

// parseSlogLevel converts a string log level to slog.Level.
func parseSlogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
