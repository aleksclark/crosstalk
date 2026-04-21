// Package main ties together all dependencies and starts the ct-server.
// This is the only place where concrete implementations are wired to domain interfaces.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	crosstalk "github.com/anthropics/crosstalk/server"
)

func main() {
	// Bootstrap a minimal logger for startup messages. This will be replaced
	// once we know the configured log level.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	if err := run(); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// Resolve config path: --config flag > CROSSTALK_CONFIG env > default.
	configPath := resolveConfigPath()

	slog.Info("loading configuration", "path", configPath)

	cfg, err := crosstalk.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Reconfigure the global logger with the level from config.
	logLevel := crosstalk.ParseLogLevel(cfg.LogLevel)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	slog.Info("configuration loaded",
		"listen", cfg.Listen,
		"db_path", cfg.DBPath,
		"log_level", cfg.LogLevel,
	)

	// TODO: Open SQLite database.
	// db := sqlite.Open(cfg.DBPath)
	// defer db.Close()

	// TODO: Wire domain service implementations.
	// var userService crosstalk.UserService = &sqlite.UserService{DB: db}
	// var templateService crosstalk.SessionTemplateService = &sqlite.SessionTemplateService{DB: db}
	// var sessionService crosstalk.SessionService = &sqlite.SessionService{DB: db}

	// TODO: Create HTTP handler, inject services.
	// var handler http.Handler
	// handler.UserService = userService
	// handler.SessionTemplateService = templateService

	// TODO: Start HTTP server (serves REST API + embedded web UI).

	fmt.Println("ct-server: not yet implemented")
	return nil
}

// resolveConfigPath determines which config file to use.
// Priority: --config flag > CROSSTALK_CONFIG env var > default "ct-server.json".
func resolveConfigPath() string {
	configFlag := flag.String("config", "", "path to configuration file")
	flag.Parse()

	// Flag takes highest priority.
	if *configFlag != "" {
		return *configFlag
	}

	// Then environment variable.
	if env := os.Getenv("CROSSTALK_CONFIG"); env != "" {
		return env
	}

	return crosstalk.DefaultConfigPath
}
