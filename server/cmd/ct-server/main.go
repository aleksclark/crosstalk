// Package main ties together all dependencies and starts the ct-server.
// This is the only place where concrete implementations are wired to domain interfaces.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	crosstalk "github.com/aleksclark/crosstalk/server"
	"github.com/aleksclark/crosstalk/server/broadcast"
	cthttp "github.com/aleksclark/crosstalk/server/http"
	ctpion "github.com/aleksclark/crosstalk/server/pion"
	ctws "github.com/aleksclark/crosstalk/server/ws"
	"github.com/aleksclark/crosstalk/server/sqlite"
	"github.com/oklog/ulid/v2"
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

	// Open SQLite database (runs migrations automatically).
	db, err := sqlite.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	slog.Info("database opened", "path", cfg.DBPath)

	// Wire domain service implementations.
	userService := &sqlite.UserService{DB: db.DB}
	tokenService := &sqlite.TokenService{DB: db.DB}
	templateService := &sqlite.SessionTemplateService{DB: db.DB}
	sessionService := &sqlite.SessionService{DB: db.DB}

	// Seed admin user on first run.
	if err := seedAdmin(userService, tokenService); err != nil {
		return fmt.Errorf("seeding admin: %w", err)
	}

	// Create WebRTC peer manager and WS signaling handler.
	pm := ctpion.NewPeerManager(cfg.WebRTC)

	// Create Orchestrator for session/channel management and audio forwarding.
	orch := ctpion.NewOrchestrator(sessionService, templateService)
	orch.PeerManager = pm
	if cfg.RecordingPath != "" {
		orch.RecordingPath = cfg.RecordingPath
	}

	sigHandler := ctws.SignalingHandler{
		TokenService:   tokenService,
		SessionService: sessionService,
		PeerManager:    pm,
		Orchestrator:   orch,
		ServerVersion:  "0.1.0",
	}

	// Build embedded web FS (strip "web/dist" prefix).
	webFS, err := fs.Sub(crosstalk.WebDist, "web/dist")
	if err != nil {
		return fmt.Errorf("creating web sub-filesystem: %w", err)
	}

	// Enable test mode when CROSSTALK_TEST_MODE env var is set.
	testMode := os.Getenv("CROSSTALK_TEST_MODE") == "1"
	if testMode {
		slog.Warn("test mode enabled — test-only endpoints are active")
	}

	// Create broadcast token store.
	var broadcastTTL time.Duration
	if d, err := time.ParseDuration(cfg.Auth.BroadcastTokenLifetime); err == nil {
		broadcastTTL = d
	} else {
		broadcastTTL = 15 * time.Minute
	}
	broadcastStore := broadcast.NewTokenStore(cfg.Auth.SessionSecret, broadcastTTL)
	defer broadcastStore.Stop()

	broadcastSigHandler := &ctws.BroadcastSignalingHandler{
		BroadcastTokenStore: broadcastStore,
		PeerManager:         pm,
		Orchestrator:        orch,
	}

	// Create HTTP handler with all services injected.
	handler := &cthttp.Handler{
		UserService:               userService,
		TokenService:              tokenService,
		SessionTemplateService:    templateService,
		SessionService:            sessionService,
		Config:                    cfg,
		WebFS:                     webFS,
		DevMode:                   cfg.Web.DevMode,
		DevProxyURL:               cfg.Web.DevProxyURL,
		SignalingHandler:          &sigHandler,
		BroadcastSignalingHandler: broadcastSigHandler,
		Orchestrator:              orch,
		PeerLister:                pm,
		BroadcastTokenStore:       broadcastStore,
		TestMode:                  testMode,
		DB:                        db.DB,
	}

	// Build the HTTP server.
	srv := &http.Server{
		Addr:    cfg.Listen,
		Handler: handler.Router(),
	}

	// Listen for shutdown signals.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Start serving in a goroutine.
	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.Listen)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	// Wait for signal or server error.
	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("server error: %w", err)
		}
	case <-ctx.Done():
		slog.Info("shutdown signal received, draining connections...")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("server shutdown: %w", err)
		}
		slog.Info("server stopped gracefully")
	}

	return nil
}

// seedAdmin creates the initial admin user and API token if no admin user
// exists. This is idempotent — if the admin already exists, it is a no-op.
func seedAdmin(userService crosstalk.UserService, tokenService crosstalk.TokenService) error {
	_, err := userService.FindUserByUsername("admin")
	if err == nil {
		slog.Debug("admin user already exists, skipping seed")
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("checking for admin user: %w", err)
	}

	// Generate a random password for the admin user.
	password := "Password!"

	hash, err := cthttp.HashPassword(password)
	if err != nil {
		return fmt.Errorf("hashing admin password: %w", err)
	}

	now := time.Now().UTC()
	adminUser := &crosstalk.User{
		ID:           ulid.Make().String(),
		Username:     "admin",
		PasswordHash: hash,
		CreatedAt:    now,
	}
	if err := userService.CreateUser(adminUser); err != nil {
		return fmt.Errorf("creating admin user: %w", err)
	}

	slog.Info("admin user seeded",
		"username", "admin",
		"password", password,
	)

	// Create an initial API token for the admin.
	plaintext := cthttp.GenerateToken()
	apiToken := &crosstalk.APIToken{
		ID:        ulid.Make().String(),
		Name:      "seed",
		TokenHash: cthttp.HashToken(plaintext),
		UserID:    adminUser.ID,
		CreatedAt: now,
	}
	if err := tokenService.CreateToken(apiToken); err != nil {
		return fmt.Errorf("creating seed token: %w", err)
	}

	slog.Info("seed API token created",
		"name", "seed",
		"token", plaintext,
	)

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
