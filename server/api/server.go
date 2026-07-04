// Package api implements the HTTP API layer using huma + chi.
package api

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	crosstalk "github.com/aleksclark/crosstalk/server"
	"github.com/aleksclark/crosstalk/server/auth"
	"github.com/aleksclark/crosstalk/server/sessionrtc"
	"github.com/aleksclark/crosstalk/server/webrtc"
)

// Services holds all service dependencies for the API.
type Services struct {
	Sessions       crosstalk.SessionService
	Channels       crosstalk.ChannelService
	Sources        crosstalk.SourceService
	Mix            crosstalk.MixService
	ABCs           crosstalk.ABCService
	Users          crosstalk.UserService
	RefreshTokens  crosstalk.RefreshTokenService
	Recordings     crosstalk.RecordingService
	Auth           *auth.Service
	RecordingsPath string // base path for recording files
	// WebApps maps a URL path prefix ("/admin") to a filesystem serving that
	// SPA's build output. Registered with history-API fallback.
	WebApps map[string]fs.FS
	// PeerManager, when set, enables the WebRTC signaling endpoint and the
	// admin debug API (live peer state + per-peer event logs).
	PeerManager *webrtc.PeerManager
	// SessionMedia, when set (together with PeerManager), enables the
	// session-scoped signaling endpoint that bridges peer audio into a
	// session's mixer.
	SessionMedia *sessionrtc.Manager
}

// Config holds API configuration.
type Config struct {
	Addr      string
	JWTSecret string
}

// Server holds the HTTP server and dependencies.
type Server struct {
	router   chi.Router
	api      huma.API
	services Services
	auth     *auth.Service
	log      *slog.Logger
}

// NewServer creates a new API server with all routes registered.
func NewServer(cfg Config, svc Services, log *slog.Logger) *Server {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	humaConfig := huma.DefaultConfig("CrossTalk", "3.0.0")
	humaConfig.Servers = []*huma.Server{{URL: ""}}
	humaConfig.OpenAPIPath = "/api/openapi"
	humaConfig.DocsPath = "/api/docs"
	humaConfig.Components = &huma.Components{
		SecuritySchemes: map[string]*huma.SecurityScheme{
			"bearerAuth": {
				Type:         "http",
				Scheme:       "bearer",
				BearerFormat: "JWT",
			},
		},
	}

	api := humachi.New(r, humaConfig)

	s := &Server{
		router:   r,
		api:      api,
		services: svc,
		auth:     svc.Auth,
		log:      log,
	}

	s.registerRoutes()
	s.mountWebApps()
	s.mountWebRTC()
	return s
}

// Handler returns the http.Handler.
func (s *Server) Handler() http.Handler {
	return s.router
}

// OpenAPIJSON returns the generated OpenAPI spec as JSON, downgraded to 3.0.3.
// It reflects the registered routes and types exactly and is the source of
// truth for client code generation. 3.0.3 is emitted (rather than huma's native
// 3.1) so that both openapi-typescript and oapi-codegen can consume it.
func (s *Server) OpenAPIJSON() ([]byte, error) {
	return s.api.OpenAPI().Downgrade()
}

// registerRoutes registers all API endpoints.
func (s *Server) registerRoutes() {
	// Auth
	huma.Register(s.api, huma.Operation{
		OperationID: "login",
		Method:      http.MethodPost,
		Path:        "/api/auth/login",
		Summary:     "Login with username and password",
		Tags:        []string{"Auth"},
	}, s.handleLogin)

	huma.Register(s.api, huma.Operation{
		OperationID: "refresh",
		Method:      http.MethodPost,
		Path:        "/api/auth/refresh",
		Summary:     "Refresh access token",
		Tags:        []string{"Auth"},
	}, s.handleRefresh)

	huma.Register(s.api, huma.Operation{
		OperationID: "logout",
		Method:      http.MethodPost,
		Path:        "/api/auth/logout",
		Summary:     "Logout and revoke refresh token",
		Tags:        []string{"Auth"},
	}, s.handleLogout)

	// Sessions
	huma.Register(s.api, huma.Operation{
		OperationID: "list-sessions",
		Method:      http.MethodGet,
		Path:        "/api/sessions",
		Summary:     "List all sessions",
		Tags:        []string{"Sessions"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, s.handleListSessions)

	huma.Register(s.api, huma.Operation{
		OperationID: "create-session",
		Method:      http.MethodPost,
		Path:        "/api/sessions",
		Summary:     "Create a new session",
		Tags:        []string{"Sessions"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, s.handleCreateSession)

	huma.Register(s.api, huma.Operation{
		OperationID: "get-session",
		Method:      http.MethodGet,
		Path:        "/api/sessions/{id}",
		Summary:     "Get session details",
		Tags:        []string{"Sessions"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, s.handleGetSession)

	huma.Register(s.api, huma.Operation{
		OperationID: "update-session",
		Method:      http.MethodPut,
		Path:        "/api/sessions/{id}",
		Summary:     "Update session metadata",
		Tags:        []string{"Sessions"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, s.handleUpdateSession)

	huma.Register(s.api, huma.Operation{
		OperationID: "delete-session",
		Method:      http.MethodDelete,
		Path:        "/api/sessions/{id}",
		Summary:     "Delete (archive) a session",
		Tags:        []string{"Sessions"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, s.handleDeleteSession)

	huma.Register(s.api, huma.Operation{
		OperationID: "get-broadcast-url",
		Method:      http.MethodGet,
		Path:        "/api/sessions/{id}/broadcast-url",
		Summary:     "Get broadcast URL",
		Tags:        []string{"Sessions"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, s.handleGetBroadcastURL)

	huma.Register(s.api, huma.Operation{
		OperationID: "regenerate-broadcast-url",
		Method:      http.MethodPost,
		Path:        "/api/sessions/{id}/broadcast-url",
		Summary:     "Regenerate broadcast URL",
		Tags:        []string{"Sessions"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, s.handleRegenerateBroadcastURL)

	// Channels
	huma.Register(s.api, huma.Operation{
		OperationID: "list-channels",
		Method:      http.MethodGet,
		Path:        "/api/sessions/{id}/channels",
		Summary:     "List channels in a session",
		Tags:        []string{"Channels"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, s.handleListChannels)

	huma.Register(s.api, huma.Operation{
		OperationID: "create-channel",
		Method:      http.MethodPost,
		Path:        "/api/sessions/{id}/channels",
		Summary:     "Create a channel in a session",
		Tags:        []string{"Channels"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, s.handleCreateChannel)

	huma.Register(s.api, huma.Operation{
		OperationID: "update-channel",
		Method:      http.MethodPut,
		Path:        "/api/sessions/{id}/channels/{ch_id}",
		Summary:     "Update a channel",
		Tags:        []string{"Channels"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, s.handleUpdateChannel)

	huma.Register(s.api, huma.Operation{
		OperationID: "delete-channel",
		Method:      http.MethodDelete,
		Path:        "/api/sessions/{id}/channels/{ch_id}",
		Summary:     "Delete a channel",
		Tags:        []string{"Channels"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, s.handleDeleteChannel)

	// Mix
	huma.Register(s.api, huma.Operation{
		OperationID: "get-mix",
		Method:      http.MethodGet,
		Path:        "/api/sessions/{id}/channels/{ch_id}/mix",
		Summary:     "Get current mix state for a channel",
		Tags:        []string{"Mixing"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, s.handleGetMix)

	huma.Register(s.api, huma.Operation{
		OperationID: "update-mix",
		Method:      http.MethodPut,
		Path:        "/api/sessions/{id}/channels/{ch_id}/mix",
		Summary:     "Update mix state for a channel",
		Tags:        []string{"Mixing"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, s.handleUpdateMix)

	// ABCs
	huma.Register(s.api, huma.Operation{
		OperationID: "list-abcs",
		Method:      http.MethodGet,
		Path:        "/api/abcs",
		Summary:     "List all ABCs",
		Tags:        []string{"ABCs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, s.handleListABCs)

	huma.Register(s.api, huma.Operation{
		OperationID: "create-abc",
		Method:      http.MethodPost,
		Path:        "/api/abcs",
		Summary:     "Register a new ABC",
		Tags:        []string{"ABCs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, s.handleCreateABC)

	huma.Register(s.api, huma.Operation{
		OperationID: "get-abc",
		Method:      http.MethodGet,
		Path:        "/api/abcs/{id}",
		Summary:     "Get ABC details",
		Tags:        []string{"ABCs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, s.handleGetABC)

	huma.Register(s.api, huma.Operation{
		OperationID: "update-abc",
		Method:      http.MethodPut,
		Path:        "/api/abcs/{id}",
		Summary:     "Update ABC configuration",
		Tags:        []string{"ABCs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, s.handleUpdateABC)

	huma.Register(s.api, huma.Operation{
		OperationID: "delete-abc",
		Method:      http.MethodDelete,
		Path:        "/api/abcs/{id}",
		Summary:     "Remove an ABC",
		Tags:        []string{"ABCs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, s.handleDeleteABC)

	huma.Register(s.api, huma.Operation{
		OperationID: "restart-abc",
		Method:      http.MethodPost,
		Path:        "/api/abcs/{id}/restart",
		Summary:     "Send restart command to an ABC",
		Tags:        []string{"ABCs"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, s.handleRestartABC)

	// Translators
	huma.Register(s.api, huma.Operation{
		OperationID: "list-translators",
		Method:      http.MethodGet,
		Path:        "/api/translators",
		Summary:     "List translator accounts",
		Tags:        []string{"Translators"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, s.handleListTranslators)

	huma.Register(s.api, huma.Operation{
		OperationID: "create-translator",
		Method:      http.MethodPost,
		Path:        "/api/translators",
		Summary:     "Create a translator account",
		Tags:        []string{"Translators"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, s.handleCreateTranslator)

	huma.Register(s.api, huma.Operation{
		OperationID: "update-translator",
		Method:      http.MethodPut,
		Path:        "/api/translators/{id}",
		Summary:     "Update a translator",
		Tags:        []string{"Translators"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, s.handleUpdateTranslator)

	huma.Register(s.api, huma.Operation{
		OperationID: "delete-translator",
		Method:      http.MethodDelete,
		Path:        "/api/translators/{id}",
		Summary:     "Delete a translator",
		Tags:        []string{"Translators"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, s.handleDeleteTranslator)

	huma.Register(s.api, huma.Operation{
		OperationID: "assign-translator-sessions",
		Method:      http.MethodPut,
		Path:        "/api/translators/{id}/sessions",
		Summary:     "Assign sessions to a translator",
		Tags:        []string{"Translators"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, s.handleAssignTranslatorSessions)

	// Users
	huma.Register(s.api, huma.Operation{
		OperationID: "list-users",
		Method:      http.MethodGet,
		Path:        "/api/users",
		Summary:     "List admin users",
		Tags:        []string{"Users"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, s.handleListUsers)

	huma.Register(s.api, huma.Operation{
		OperationID: "create-user",
		Method:      http.MethodPost,
		Path:        "/api/users",
		Summary:     "Create an admin user",
		Tags:        []string{"Users"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, s.handleCreateUser)

	huma.Register(s.api, huma.Operation{
		OperationID: "delete-user",
		Method:      http.MethodDelete,
		Path:        "/api/users/{id}",
		Summary:     "Delete an admin user",
		Tags:        []string{"Users"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, s.handleDeleteUser)

	// Public - broadcast
	huma.Register(s.api, huma.Operation{
		OperationID: "get-broadcast-info",
		Method:      http.MethodGet,
		Path:        "/api/sessions/{id}/broadcast",
		Summary:     "Get broadcast info (public)",
		Tags:        []string{"Public"},
	}, s.handleGetBroadcastInfo)

	// WebRTC
	huma.Register(s.api, huma.Operation{
		OperationID: "get-webrtc-token",
		Method:      http.MethodPost,
		Path:        "/api/webrtc/token",
		Summary:     "Get a short-lived WebRTC signaling token",
		Tags:        []string{"WebRTC"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, s.handleWebRTCToken)

	// Recordings
	huma.Register(s.api, huma.Operation{
		OperationID: "list-session-recordings",
		Method:      http.MethodGet,
		Path:        "/api/sessions/{id}/recordings",
		Summary:     "List recordings for a session",
		Tags:        []string{"Recordings"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, s.handleListRecordings)

	huma.Register(s.api, huma.Operation{
		OperationID: "get-recording-download",
		Method:      http.MethodGet,
		Path:        "/api/recordings/{id}/download",
		Summary:     "Get recording file download path",
		Tags:        []string{"Recordings"},
		Security:    []map[string][]string{{"bearerAuth": {}}},
	}, s.handleDownloadRecording)
}

// requireAuth validates the JWT from the Authorization header.
func (s *Server) requireAuth(ctx context.Context, authHeader string) (*auth.Claims, error) {
	if authHeader == "" {
		return nil, huma.Error401Unauthorized("missing authorization header")
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return nil, huma.Error401Unauthorized("invalid authorization header format")
	}

	claims, err := s.auth.ValidateAccessToken(parts[1])
	if err != nil {
		return nil, huma.Error401Unauthorized(fmt.Sprintf("invalid token: %v", err))
	}

	return claims, nil
}

// requireRole checks that the authenticated user has one of the allowed roles.
func (s *Server) requireRole(claims *auth.Claims, roles ...string) error {
	for _, r := range roles {
		if claims.Role == r {
			return nil
		}
	}
	return huma.Error403Forbidden("insufficient permissions")
}
