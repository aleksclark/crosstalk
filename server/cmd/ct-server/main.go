package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/aleksclark/crosstalk/server/api"
	"github.com/aleksclark/crosstalk/server/auth"
	"github.com/aleksclark/crosstalk/server/postgres"
	"github.com/aleksclark/crosstalk/server/sessionrtc"
	"github.com/aleksclark/crosstalk/server/web"
	"github.com/aleksclark/crosstalk/server/webrtc"

	crosstalk "github.com/aleksclark/crosstalk/server"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	// Config from env or defaults
	addr := envOr("CT_ADDR", ":8080")
	dbURL := envOr("CT_DATABASE_URL", "postgres://postgres:postgres@localhost:5432/crosstalk?sslmode=disable")
	jwtSecret := envOr("CT_JWT_SECRET", "change-me-in-production")

	log.Info("starting CrossTalk server",
		"addr", addr,
	)

	// Open database
	db, err := postgres.Open(dbURL, log)
	if err != nil {
		log.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Create stores
	sessionStore := postgres.NewSessionStore(db)
	channelStore := postgres.NewChannelStore(db)
	sourceStore := postgres.NewSourceStore(db)
	mixStore := postgres.NewMixStore(db)
	abcStore := postgres.NewABCStore(db)
	userStore := postgres.NewUserStore(db)
	refreshTokenStore := postgres.NewRefreshTokenStore(db)

	// Create default admin if no users exist
	ensureDefaultAdmin(context.Background(), userStore, log)

	// Auth service
	authCfg := auth.Config{
		Secret:          jwtSecret,
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
	}
	authService := auth.NewService(authCfg, userStore, refreshTokenStore)

	// WebRTC peer manager: powers the signaling endpoint and the admin debug
	// API (live peer state + event logs). ICE config comes from env so it can
	// be tuned per environment (Fly sets CT_UDP_MUX_PORT + CT_PUBLIC_IP).
	peerManager := webrtc.NewPeerManager(webrtc.ICEConfig{
		STUNServers: stunServers(),
		PublicIP:    os.Getenv("CT_PUBLIC_IP"),
		UDPMuxPort:  envInt("CT_UDP_MUX_PORT", 0),
	})

	// Session media manager: bridges connected peers' audio into per-session
	// mixers and streams mixed channel output back to listeners.
	sessionMedia := sessionrtc.NewManager(sessionrtc.Stores{
		Channels: channelStore,
		Sources:  sourceStore,
		Mix:      mixStore,
	}, log)

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
		PeerManager:   peerManager,
		SessionMedia:  sessionMedia,
	}

	// Embedded frontend SPAs (served at /admin, /broadcast, /translator).
	webApps, err := web.Apps()
	if err != nil {
		log.Error("failed to load embedded web apps", "error", err)
		os.Exit(1)
	}
	svc.WebApps = webApps

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

// envInt reads an integer env var, falling back to defaultVal when unset or invalid.
func envInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultVal
}

// stunServers returns the STUN server list from CT_STUN_SERVERS (comma-separated),
// defaulting to Google's public STUN server.
func stunServers() []string {
	if v := os.Getenv("CT_STUN_SERVERS"); v != "" {
		parts := strings.Split(v, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if s := strings.TrimSpace(p); s != "" {
				out = append(out, s)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return []string{"stun:stun.l.google.com:19302"}
}
