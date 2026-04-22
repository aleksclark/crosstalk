// Package main is the entrypoint for ct-client.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/aleksclark/crosstalk/cli/pion"
	"github.com/aleksclark/crosstalk/cli/pipewire"
)

func main() {
	flag.Parse()

	// Initial logger (will be reconfigured after config load)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("starting ct-client")

	if err := run(); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// 1. Load config
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	// 2. Reconfigure logger with config log level
	level := parseSlogLevel(cfg.LogLevel)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(logger)

	slog.Info("config loaded",
		"server_url", cfg.ServerURL,
		"log_level", cfg.LogLevel,
		"source_name", cfg.SourceName,
		"sink_name", cfg.SinkName,
	)

	// 3. Create PipeWire service
	pwSvc := pipewire.NewService(cfg.SourceName, cfg.SinkName)

	// 4. Create and run client
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		slog.Info("received signal, shutting down", "signal", sig)
		cancel()
	}()

	client := pion.NewClient(cfg, pwSvc,
		pion.WithClientOnConnected(func() {
			slog.Info("connected to server")
		}),
		pion.WithClientOnDisconnected(func() {
			slog.Warn("disconnected from server")
		}),
		pion.WithClientOnWelcome(func(w *pion.WelcomeMessage) {
			slog.Info("welcome received",
				"client_id", w.ClientID,
				"server_version", w.ServerVersion,
			)
		}),
	)

	return client.Run(ctx)
}
