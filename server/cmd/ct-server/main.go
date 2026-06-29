package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/aleksclark/crosstalk/server/api"
	"github.com/aleksclark/crosstalk/server/auth"
	"github.com/aleksclark/crosstalk/server/sqlite"

	crosstalk "github.com/aleksclark/crosstalk/server"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	// Config from env or defaults
	addr := envOr("CT_ADDR", ":8080")
	dbPath := envOr("CT_DB_PATH", "crosstalk.db")
	jwtSecret := envOr("CT_JWT_SECRET", "change-me-in-production")

	log.Info("starting CrossTalk server",
		"addr", addr,
		"db", dbPath,
	)

	// Open database
	db, err := sqlite.Open(dbPath, log)
	if err != nil {
		log.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Create stores
	sessionStore := sqlite.NewSessionStore(db)
	channelStore := sqlite.NewChannelStore(db)
	sourceStore := sqlite.NewSourceStore(db)
	mixStore := sqlite.NewMixStore(db)
	abcStore := sqlite.NewABCStore(db)
	userStore := sqlite.NewUserStore(db)
	refreshTokenStore := sqlite.NewRefreshTokenStore(db)

	// Create default admin if no users exist
	ensureDefaultAdmin(context.Background(), userStore, log)

	// Auth service
	authCfg := auth.Config{
		Secret:          jwtSecret,
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
	}
	authService := auth.NewService(authCfg, userStore, refreshTokenStore)

	// API server
	svc := api.Services{
		Sessions:      sessionStore,
		Channels:      channelStore,
		Sources:       sourceStore,
		Mix:           mixStore,
		ABCs:          abcStore,
		Users:         userStore,
		RefreshTokens: refreshTokenStore,
		Auth:          authService,
	}

	apiCfg := api.Config{
		Addr:      addr,
		JWTSecret: jwtSecret,
	}

	srv := api.NewServer(apiCfg, svc, log)

	httpServer := &http.Server{
		Addr:    addr,
		Handler: srv.Handler(),
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Info("shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
	}()

	log.Info("server listening", "addr", addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error("server error", "error", err)
		os.Exit(1)
	}
}

func ensureDefaultAdmin(ctx context.Context, users crosstalk.UserService, log *slog.Logger) {
	existing, err := users.List(ctx)
	if err != nil || len(existing) > 0 {
		return
	}

	password := envOr("CT_ADMIN_PASSWORD", "admin")
	hash, err := auth.HashPassword(password)
	if err != nil {
		log.Error("failed to hash default admin password", "error", err)
		return
	}

	admin := &crosstalk.User{
		ID:           ulid.Make().String(),
		Username:     "admin",
		PasswordHash: hash,
		Role:         "admin",
	}

	if err := users.Create(ctx, admin); err != nil {
		log.Error("failed to create default admin", "error", err)
		return
	}

	log.Info("created default admin user", "username", "admin")
}

func envOr(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
