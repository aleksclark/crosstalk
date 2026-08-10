package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/aleksclark/crosstalk/server/api"
	"github.com/aleksclark/crosstalk/server/auth"
	"github.com/aleksclark/crosstalk/server/mediaticket"
	"github.com/aleksclark/crosstalk/server/ownership"
	"github.com/aleksclark/crosstalk/server/postgres"
	"github.com/aleksclark/crosstalk/server/sessionrtc"
	"github.com/aleksclark/crosstalk/server/web"
	"github.com/aleksclark/crosstalk/server/webrtc"

	crosstalk "github.com/aleksclark/crosstalk/server"
)

const (
	defaultJWTSecret     = "change-me-in-production"
	defaultAdminPassword = "admin"
	defaultLeaseTTL      = 5 * time.Minute
	defaultRenewInterval = 90 * time.Second
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	cfg, err := loadConfig()
	if err != nil {
		log.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	log.Info("starting CrossTalk server",
		"addr", cfg.Addr,
		"instance_id", cfg.InstanceID,
		"test_mode", cfg.TestMode,
		"public_media_url_set", cfg.PublicMediaURL != "",
	)

	db, err := postgres.Open(cfg.DatabaseURL, log)
	if err != nil {
		log.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	sessionStore := postgres.NewSessionStore(db)
	channelStore := postgres.NewChannelStore(db)
	sourceStore := postgres.NewSourceStore(db)
	mixStore := postgres.NewMixStore(db)
	abcStore := postgres.NewABCStore(db)
	userStore := postgres.NewUserStore(db)
	refreshTokenStore := postgres.NewRefreshTokenStore(db)
	ticketStore := postgres.NewMediaTicketStore(db)
	recordingStore := postgres.NewRecordingStore(db)

	leaseStore := ownership.NewStore(db.DB)
	ticketService := mediaticket.NewService(ticketStore, []byte(cfg.MediaTicketSecret))

	ensureDefaultAdmin(context.Background(), userStore, cfg, log)

	authService := auth.NewService(auth.Config{
		Secret:          cfg.JWTSecret,
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
	}, userStore, refreshTokenStore)

	iceCfg := webrtc.ICEConfig{
		STUNServers: cfg.STUNServers,
		TURNServer:  cfg.TURNURL,
		TURNUser:    cfg.TURNUsername,
		TURNCred:    cfg.TURNCredential,
		PublicIP:    cfg.PublicIP,
		UDPMuxPort:  cfg.UDPMuxPort,
	}
	peerManager, err := webrtc.NewPeerManagerValidated(iceCfg)
	if err != nil {
		log.Error("invalid ICE/TURN configuration", "error", err)
		os.Exit(1)
	}

	// Production path: real Opus codec factory (audiocodec.Factory) + NopObserver.
	sessionMedia := sessionrtc.NewManager(sessionrtc.Stores{
		Channels: channelStore,
		Sources:  sourceStore,
		Mix:      mixStore,
	}, log, sessionrtc.WithObserver(sessionrtc.NopObserver{}))

	// Track leases this instance holds so SIGTERM can release them.
	// Wire Note before API construction so ticket-issue acquires are observed.
	leaseTracker := newLeaseTracker(leaseStore, cfg.InstanceID, cfg.LeaseTTL, cfg.LeaseRenew, log)
	leaseTracker.Start(context.Background())

	svc := api.Services{
		Sessions:      sessionStore,
		Channels:      channelStore,
		Sources:       sourceStore,
		Mix:           mixStore,
		ABCs:          abcStore,
		Users:         userStore,
		RefreshTokens: refreshTokenStore,
		Recordings:    recordingStore,
		Auth:          authService,
		PeerManager:   peerManager,
		SessionMedia:  sessionMedia,
		MediaTickets:  ticketService,
		Leases:        leaseStore,
		InstanceID:    cfg.InstanceID,
		OnLeaseAcquired: func(lease ownership.Lease) {
			leaseTracker.Note(lease)
		},
	}

	webApps, err := web.Apps()
	if err != nil {
		log.Error("failed to load embedded web apps", "error", err)
		os.Exit(1)
	}
	svc.WebApps = webApps

	apiSrv := api.NewServer(api.Config{
		Addr:      cfg.Addr,
		JWTSecret: cfg.JWTSecret,
	}, svc, log)

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           apiSrv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		log.Info("shutting down...", "signal", sig.String())

		// 1. Stop admission (reject new peers).
		peerManager.CloseAll("shutdown")

		// 2. Mark owned sessions draining (best-effort).
		drainOwnedSessions(context.Background(), sessionStore, leaseStore, cfg.InstanceID, log)

		// 3. Stop lease renewals and release leases.
		leaseTracker.Stop()

		// 4. Wait for peers to finish (mixers drain via peer OnClose).
		peerManager.Wait()

		// 5. HTTP shutdown.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
	}()

	log.Info("server listening", "addr", cfg.Addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error("server error", "error", err)
		os.Exit(1)
	}
}

// serverConfig is the production composition configuration.
type serverConfig struct {
	Addr              string
	DatabaseURL       string
	JWTSecret         string
	MediaTicketSecret string
	AdminPassword     string
	InstanceID        string
	PublicMediaURL    string
	PublicIP          string
	UDPMuxPort        int
	STUNServers       []string
	TURNURL           string
	TURNUsername      string
	TURNCredential    string
	LeaseTTL          time.Duration
	LeaseRenew        time.Duration
	TestMode          bool
	AllowInsecure     bool
	MultiReplica      bool
}

func loadConfig() (serverConfig, error) {
	testMode := envTruthy("CT_TEST_MODE")
	allowInsecure := envTruthy("CT_ALLOW_INSECURE_DEFAULTS") || testMode

	cfg := serverConfig{
		Addr:           envOr("CT_ADDR", ":8080"),
		DatabaseURL:    os.Getenv("CT_DATABASE_URL"),
		JWTSecret:      os.Getenv("CT_JWT_SECRET"),
		AdminPassword:  envOr("CT_ADMIN_PASSWORD", defaultAdminPassword),
		InstanceID:     os.Getenv("CT_INSTANCE_ID"),
		PublicMediaURL: strings.TrimSpace(os.Getenv("CT_PUBLIC_MEDIA_URL")),
		PublicIP:       os.Getenv("CT_PUBLIC_IP"),
		UDPMuxPort:     envInt("CT_UDP_MUX_PORT", 0),
		STUNServers:    stunServers(),
		TURNURL:        strings.TrimSpace(os.Getenv("CT_TURN_URL")),
		TURNUsername:   os.Getenv("CT_TURN_USERNAME"),
		TURNCredential: os.Getenv("CT_TURN_CREDENTIAL"),
		LeaseTTL:       envDuration("CT_LEASE_TTL", defaultLeaseTTL),
		LeaseRenew:     envDuration("CT_LEASE_RENEW", defaultRenewInterval),
		TestMode:       testMode,
		AllowInsecure:  allowInsecure,
		MultiReplica:   envTruthy("CT_MULTI_REPLICA"),
	}

	if cfg.DatabaseURL == "" {
		if allowInsecure {
			cfg.DatabaseURL = "postgres://postgres:postgres@127.0.0.1:5432/crosstalk?sslmode=disable"
		} else {
			return cfg, fmt.Errorf("CT_DATABASE_URL is required")
		}
	}

	if cfg.JWTSecret == "" || cfg.JWTSecret == defaultJWTSecret {
		if !allowInsecure {
			return cfg, fmt.Errorf("CT_JWT_SECRET must be set to a non-default value (or set CT_TEST_MODE=1 / CT_ALLOW_INSECURE_DEFAULTS=1)")
		}
		if cfg.JWTSecret == "" {
			cfg.JWTSecret = defaultJWTSecret
		}
	}

	// Media tickets share the JWT secret by default unless overridden.
	cfg.MediaTicketSecret = envOr("CT_MEDIA_TICKET_SECRET", cfg.JWTSecret)

	if cfg.AdminPassword == "" || cfg.AdminPassword == defaultAdminPassword {
		if !allowInsecure {
			return cfg, fmt.Errorf("CT_ADMIN_PASSWORD must be set to a non-default value outside test/insecure mode")
		}
		if cfg.AdminPassword == "" {
			cfg.AdminPassword = defaultAdminPassword
		}
	}

	if cfg.InstanceID == "" {
		if allowInsecure {
			cfg.InstanceID = "ct-test"
		} else {
			return cfg, fmt.Errorf("CT_INSTANCE_ID is required (unique per server process)")
		}
	}

	// Reject multi-replica without owner routing / public media URL.
	if cfg.MultiReplica && cfg.PublicMediaURL == "" {
		return cfg, fmt.Errorf("CT_MULTI_REPLICA=1 requires CT_PUBLIC_MEDIA_URL for owner routing")
	}

	if cfg.LeaseTTL <= 0 {
		cfg.LeaseTTL = defaultLeaseTTL
	}
	if cfg.LeaseRenew <= 0 || cfg.LeaseRenew >= cfg.LeaseTTL {
		cfg.LeaseRenew = cfg.LeaseTTL / 3
		if cfg.LeaseRenew < time.Second {
			cfg.LeaseRenew = time.Second
		}
	}

	return cfg, nil
}

func ensureDefaultAdmin(ctx context.Context, users crosstalk.UserService, cfg serverConfig, log *slog.Logger) {
	existing, err := users.List(ctx)
	if err != nil || len(existing) > 0 {
		return
	}

	hash, err := auth.HashPassword(cfg.AdminPassword)
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

func drainOwnedSessions(ctx context.Context, sessions crosstalk.SessionService, leases ownership.Service, instanceID string, log *slog.Logger) {
	if sessions == nil || leases == nil {
		return
	}
	list, err := sessions.List(ctx)
	if err != nil {
		log.Warn("shutdown: list sessions failed", "error", err)
		return
	}
	for _, sess := range list {
		cur, err := leases.Current(ctx, sess.ID)
		if err != nil || cur.OwnerID != instanceID {
			continue
		}
		gen := cur.Generation
		// waiting|active → draining; ignore illegal transitions.
		if err := sessions.TransitionState(ctx, sess.ID, crosstalk.SessionDraining, &gen); err != nil {
			log.Debug("shutdown: drain session skipped", "session_id", sess.ID, "error", err)
		}
	}
}

// leaseTracker periodically renews leases this instance holds and releases on stop.
type leaseTracker struct {
	leases     ownership.Service
	instanceID string
	ttl        time.Duration
	renewEvery time.Duration
	log        *slog.Logger

	mu      sync.Mutex
	held    map[string]ownership.Lease // sessionID → lease
	cancel  context.CancelFunc
	stopped chan struct{}
}

func newLeaseTracker(leases ownership.Service, instanceID string, ttl, renewEvery time.Duration, log *slog.Logger) *leaseTracker {
	return &leaseTracker{
		leases:     leases,
		instanceID: instanceID,
		ttl:        ttl,
		renewEvery: renewEvery,
		log:        log,
		held:       make(map[string]ownership.Lease),
		stopped:    make(chan struct{}),
	}
}

func (t *leaseTracker) Start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	t.cancel = cancel
	go t.loop(ctx)
}

func (t *leaseTracker) loop(ctx context.Context) {
	defer close(t.stopped)
	ticker := time.NewTicker(t.renewEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.renewAll(ctx)
		}
	}
}

func (t *leaseTracker) renewAll(ctx context.Context) {
	// Discover sessions we currently own and renew them.
	// ownership.Current is the source of truth; we also keep a local cache.
	t.mu.Lock()
	cached := make([]ownership.Lease, 0, len(t.held))
	for _, l := range t.held {
		cached = append(cached, l)
	}
	t.mu.Unlock()

	for _, l := range cached {
		renewed, err := t.leases.Renew(ctx, l, t.ttl)
		if err != nil {
			t.log.Warn("lease renew failed", "session_id", l.SessionID, "error", err)
			t.mu.Lock()
			delete(t.held, l.SessionID)
			t.mu.Unlock()
			continue
		}
		t.mu.Lock()
		t.held[l.SessionID] = renewed
		t.mu.Unlock()
	}
}

// Note tracks a lease acquired out-of-band (e.g. by API ticket issuance via
// Services.OnLeaseAcquired). Enables renew loop and SIGTERM release.
func (t *leaseTracker) Note(lease ownership.Lease) {
	if lease.SessionID == "" || lease.OwnerID != t.instanceID {
		return
	}
	t.mu.Lock()
	t.held[lease.SessionID] = lease
	t.mu.Unlock()
}

func (t *leaseTracker) Stop() {
	if t.cancel != nil {
		t.cancel()
	}
	<-t.stopped

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	t.mu.Lock()
	held := make([]ownership.Lease, 0, len(t.held))
	for _, l := range t.held {
		held = append(held, l)
	}
	t.held = make(map[string]ownership.Lease)
	t.mu.Unlock()

	for _, l := range held {
		if err := t.leases.Release(ctx, l); err != nil {
			t.log.Warn("lease release failed", "session_id", l.SessionID, "error", err)
		}
	}
}

func envOr(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func envInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultVal
}

func envDuration(key string, defaultVal time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		// Also accept seconds as integer.
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return defaultVal
}

func envTruthy(key string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
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
