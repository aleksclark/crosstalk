// Package main is the entrypoint for ct-client.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	crosstalkv1 "github.com/aleksclark/crosstalk/proto/gen/go/crosstalk/v1"

	"github.com/aleksclark/crosstalk/cli/display"
	"github.com/aleksclark/crosstalk/cli/pion"
	"github.com/aleksclark/crosstalk/cli/pipewire"
)

const (
	defaultSPIDevice = "/dev/spidev0.1"
	defaultDCGPIO    = 71  // PC7
	defaultRSTGPIO   = 76  // PC12
)

func main() {
	flag.Parse()

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
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		slog.Info("received signal, shutting down", "signal", sig)
		cancel()
	}()

	var disp *display.Service
	if useDisplay() {
		spiPath := os.Getenv("DISPLAY_SPI_DEVICE")
		if spiPath == "" {
			spiPath = defaultSPIDevice
		}
		disp = display.NewService(spiPath, defaultDCGPIO, defaultRSTGPIO)
		disp.SetAudioDevices(cfg.SourceName, cfg.SinkName)
		disp.Status().SetServer(cfg.ServerURL, "connecting")

		go func() {
			if err := disp.Run(ctx); err != nil {
				slog.Error("display service failed", "error", err)
			}
		}()
	}

	pwSvc := pipewire.NewService(cfg.SourceName, cfg.SinkName)

	clientOpts := []pion.ClientOption{
		pion.WithClientOnConnected(func() {
			slog.Info("connected to server")
			if disp != nil {
				disp.Status().SetControlState("connected")
			}
		}),
		pion.WithClientOnDisconnected(func() {
			slog.Warn("disconnected from server")
			if disp != nil {
				disp.Status().SetControlState("disconnected")
				disp.Status().SetChannels(nil)
				disp.Status().SetSession("", "", false)
				disp.Status().SetVU(0, 0)
			}
		}),
		pion.WithClientOnWelcome(func(w *crosstalkv1.Welcome) {
			slog.Info("welcome received",
				"client_id", w.GetClientId(),
				"server_version", w.GetServerVersion(),
			)
		}),
		pion.WithOnBindChannelClient(func(bind *pion.BindChannelMsg) {
			if disp != nil {
				disp.Status().UpsertChannel(display.ChannelInfo{
					ID:        bind.ChannelID,
					Direction: string(bind.Direction),
					Codec:     "opus",
					State:     "active",
				})
			}
		}),
	}

	client := pion.NewClient(cfg, pwSvc, clientOpts...)
	return client.Run(ctx)
}

func useDisplay() bool {
	v := os.Getenv("USE_DISPLAY")
	return strings.EqualFold(v, "true") || v == "1"
}
