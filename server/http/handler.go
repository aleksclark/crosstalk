package http

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"time"

	crosstalk "github.com/aleksclark/crosstalk/server"
	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"
)

// Handler holds the service dependencies for all HTTP handlers.
type Handler struct {
	UserService            crosstalk.UserService
	TokenService           crosstalk.TokenService
	SessionTemplateService crosstalk.SessionTemplateService
	SessionService         crosstalk.SessionService
	Config                 crosstalk.Config

	// WebFS is the filesystem used for serving the web UI in production mode.
	// It should already have the "web/dist" prefix stripped (via fs.Sub).
	WebFS fs.FS

	// DevMode enables reverse-proxying to the Vite dev server instead of
	// serving embedded assets.
	DevMode bool

	// DevProxyURL is the Vite dev server URL (e.g. "http://localhost:5173").
	DevProxyURL string

	// SignalingHandler is the WebSocket signaling handler for WebRTC.
	// It handles its own authentication via query parameter tokens.
	SignalingHandler http.Handler

	// TestMode enables test-only endpoints (e.g. POST /api/test/reset).
	TestMode bool

	// DB is the raw database handle, only used in test mode for the reset
	// endpoint. It must not be nil when TestMode is true.
	DB *sql.DB
}

// Router builds and returns the chi router with the full route tree.
func (h *Handler) Router() *chi.Mux {
	r := chi.NewRouter()

	// Mount the web UI handler as a catch-all. API and WS routes are
	// registered first and take precedence.
	var webHandler http.Handler
	if h.DevMode {
		webHandler = DevProxyHandler(h.DevProxyURL)
	} else if h.WebFS != nil {
		webHandler = EmbedHandler(h.WebFS)
	}

	// Mount WebSocket signaling endpoint BEFORE the API routes.
	// This handler does its own token validation from query params.
	if h.SignalingHandler != nil {
		r.Handle("/ws/signaling", h.SignalingHandler)
	}

	r.Route("/api", func(r chi.Router) {
		// Public: login does not require auth.
		r.Post("/auth/login", h.handleLogin)

		// Test-only: reset endpoint truncates all tables.
		if h.TestMode {
			r.Post("/test/reset", h.handleTestReset)
		}

		// All other /api routes require auth.
		r.Group(func(r chi.Router) {
			r.Use(AuthMiddleware(h.TokenService))

			// Auth
			r.Post("/auth/logout", h.handleLogout)
			r.Post("/webrtc/token", h.handleWebRTCToken)

			// Users
			r.Get("/users", h.handleListUsers)
			r.Post("/users", h.handleCreateUser)
			r.Patch("/users/{id}", h.handleUpdateUser)
			r.Delete("/users/{id}", h.handleDeleteUser)

			// Tokens
			r.Get("/tokens", h.handleListTokens)
			r.Post("/tokens", h.handleCreateToken)
			r.Delete("/tokens/{id}", h.handleDeleteToken)

			// Templates
			r.Get("/templates", h.handleListTemplates)
			r.Post("/templates", h.handleCreateTemplate)
			r.Get("/templates/{id}", h.handleGetTemplate)
			r.Put("/templates/{id}", h.handleUpdateTemplate)
			r.Delete("/templates/{id}", h.handleDeleteTemplate)

			// Sessions
			r.Get("/sessions", h.handleListSessions)
			r.Post("/sessions", h.handleCreateSession)
			r.Get("/sessions/{id}", h.handleGetSession)
			r.Delete("/sessions/{id}", h.handleDeleteSession)

			// Clients (stub)
			r.Get("/clients", h.handleListClients)
			r.Get("/clients/{id}", h.handleGetClient)

			// OpenAPI
			r.Get("/openapi.json", h.handleOpenAPI)
		})
	})

	if webHandler != nil {
		r.NotFound(webHandler.ServeHTTP)
	}

	return r
}

// --- JSON helpers ---

// errorEnvelope is the standard error response format.
type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data) //nolint:errcheck
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorEnvelope{
		Error: errorBody{Status: status, Message: message},
	})
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func newID() string {
	return ulid.Make().String()
}

// --- Auth handlers ---

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Username == "" || body.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	user, err := h.UserService.FindUserByUsername(body.Username)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if !CheckPassword(user.PasswordHash, body.Password) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	plaintext := GenerateToken()
	apiToken := &crosstalk.APIToken{
		ID:        newID(),
		Name:      "login",
		TokenHash: HashToken(plaintext),
		UserID:    user.ID,
		CreatedAt: time.Now().UTC(),
	}
	if err := h.TokenService.CreateToken(apiToken); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"token": plaintext,
		"user":  toUserResponse(user),
	})
}

func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	tok := TokenFromContext(r.Context())
	if tok == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if err := h.TokenService.DeleteToken(tok.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete token")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleWebRTCToken(w http.ResponseWriter, r *http.Request) {
	apiToken := TokenFromContext(r.Context())
	if apiToken == nil {
		writeError(w, http.StatusUnauthorized, "missing authentication")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"token":   "use_api_token",
		"note":    "WebRTC signaling uses your API token directly. Connect to /ws/signaling?token=<your_api_token>.",
		"user_id": apiToken.UserID,
	})
}

// --- User handlers ---

// userResponse is the JSON representation of a user, omitting password_hash.
type userResponse struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
}

func toUserResponse(u *crosstalk.User) userResponse {
	return userResponse{
		ID:        u.ID,
		Username:  u.Username,
		CreatedAt: u.CreatedAt,
	}
}

func (h *Handler) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.UserService.ListUsers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	resp := make([]userResponse, len(users))
	for i := range users {
		resp[i] = toUserResponse(&users[i])
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Username == "" || body.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	hash, err := HashPassword(body.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	user := &crosstalk.User{
		ID:           newID(),
		Username:     body.Username,
		PasswordHash: hash,
		CreatedAt:    time.Now().UTC(),
	}
	if err := h.UserService.CreateUser(user); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}
	writeJSON(w, http.StatusCreated, toUserResponse(user))
}

func (h *Handler) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var body struct {
		Username *string `json:"username"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, err := h.UserService.FindUserByID(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to find user")
		return
	}

	if body.Username != nil {
		user.Username = *body.Username
	}

	if err := h.UserService.UpdateUser(user); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update user")
		return
	}
	writeJSON(w, http.StatusOK, toUserResponse(user))
}

func (h *Handler) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.UserService.DeleteUser(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Token handlers ---

type tokenResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type tokenCreateResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Token string `json:"token"`
}

func (h *Handler) handleListTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := h.TokenService.ListTokens()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tokens")
		return
	}
	resp := make([]tokenResponse, len(tokens))
	for i, t := range tokens {
		resp[i] = tokenResponse{
			ID:        t.ID,
			Name:      t.Name,
			CreatedAt: t.CreatedAt,
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	tok := TokenFromContext(r.Context())
	plaintext := GenerateToken()
	apiToken := &crosstalk.APIToken{
		ID:        newID(),
		Name:      body.Name,
		TokenHash: HashToken(plaintext),
		UserID:    tok.UserID,
		CreatedAt: time.Now().UTC(),
	}
	if err := h.TokenService.CreateToken(apiToken); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create token")
		return
	}

	writeJSON(w, http.StatusCreated, tokenCreateResponse{
		ID:    apiToken.ID,
		Name:  apiToken.Name,
		Token: plaintext,
	})
}

func (h *Handler) handleDeleteToken(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.TokenService.DeleteToken(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "token not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete token")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Template handlers ---

type templateRequest struct {
	Name      string             `json:"name"`
	Roles     []crosstalk.Role   `json:"roles"`
	Mappings  []crosstalk.Mapping `json:"mappings"`
	IsDefault bool               `json:"is_default"`
}

type templateResponse struct {
	ID        string              `json:"id"`
	Name      string              `json:"name"`
	IsDefault bool                `json:"is_default"`
	Roles     []crosstalk.Role    `json:"roles"`
	Mappings  []crosstalk.Mapping `json:"mappings"`
	CreatedAt time.Time           `json:"created_at"`
	UpdatedAt time.Time           `json:"updated_at"`
}

func toTemplateResponse(t *crosstalk.SessionTemplate) templateResponse {
	roles := t.Roles
	if roles == nil {
		roles = []crosstalk.Role{}
	}
	mappings := t.Mappings
	if mappings == nil {
		mappings = []crosstalk.Mapping{}
	}
	return templateResponse{
		ID:        t.ID,
		Name:      t.Name,
		IsDefault: t.IsDefault,
		Roles:     roles,
		Mappings:  mappings,
		CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
	}
}

func (h *Handler) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	templates, err := h.SessionTemplateService.ListTemplates()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list templates")
		return
	}
	resp := make([]templateResponse, len(templates))
	for i := range templates {
		resp[i] = toTemplateResponse(&templates[i])
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) handleCreateTemplate(w http.ResponseWriter, r *http.Request) {
	var body templateRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	now := time.Now().UTC()
	tmpl := &crosstalk.SessionTemplate{
		ID:        newID(),
		Name:      body.Name,
		IsDefault: body.IsDefault,
		Roles:     body.Roles,
		Mappings:  body.Mappings,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := tmpl.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.SessionTemplateService.CreateTemplate(tmpl); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create template")
		return
	}
	writeJSON(w, http.StatusCreated, toTemplateResponse(tmpl))
}

func (h *Handler) handleGetTemplate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tmpl, err := h.SessionTemplateService.FindTemplateByID(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "template not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to find template")
		return
	}
	writeJSON(w, http.StatusOK, toTemplateResponse(tmpl))
}

func (h *Handler) handleUpdateTemplate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var body templateRequest
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	// Verify the template exists.
	existing, err := h.SessionTemplateService.FindTemplateByID(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "template not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to find template")
		return
	}

	tmpl := &crosstalk.SessionTemplate{
		ID:        id,
		Name:      body.Name,
		IsDefault: body.IsDefault,
		Roles:     body.Roles,
		Mappings:  body.Mappings,
		CreatedAt: existing.CreatedAt,
		UpdatedAt: time.Now().UTC(),
	}

	if err := tmpl.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.SessionTemplateService.UpdateTemplate(tmpl); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update template")
		return
	}
	writeJSON(w, http.StatusOK, toTemplateResponse(tmpl))
}

func (h *Handler) handleDeleteTemplate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.SessionTemplateService.DeleteTemplate(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "template not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete template")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Session handlers ---

type sessionResponse struct {
	ID         string                  `json:"id"`
	TemplateID string                  `json:"template_id"`
	Name       string                  `json:"name"`
	Status     crosstalk.SessionStatus `json:"status"`
	CreatedAt  time.Time               `json:"created_at"`
	EndedAt    *time.Time              `json:"ended_at,omitempty"`
}

func toSessionResponse(s *crosstalk.Session) sessionResponse {
	return sessionResponse{
		ID:         s.ID,
		TemplateID: s.TemplateID,
		Name:       s.Name,
		Status:     s.Status,
		CreatedAt:  s.CreatedAt,
		EndedAt:    s.EndedAt,
	}
}

func (h *Handler) handleListSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := h.SessionService.ListSessions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list sessions")
		return
	}

	// Optional status filter.
	statusFilter := r.URL.Query().Get("status")

	var resp []sessionResponse
	for i := range sessions {
		if statusFilter != "" && string(sessions[i].Status) != statusFilter {
			continue
		}
		resp = append(resp, toSessionResponse(&sessions[i]))
	}
	if resp == nil {
		resp = []sessionResponse{}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TemplateID string `json:"template_id"`
		Name       string `json:"name"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.TemplateID == "" || body.Name == "" {
		writeError(w, http.StatusBadRequest, "template_id and name are required")
		return
	}

	// Verify template exists.
	if _, err := h.SessionTemplateService.FindTemplateByID(body.TemplateID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusBadRequest, "template not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to find template")
		return
	}

	session := &crosstalk.Session{
		ID:         newID(),
		TemplateID: body.TemplateID,
		Name:       body.Name,
		Status:     crosstalk.SessionWaiting,
		CreatedAt:  time.Now().UTC(),
	}
	if err := h.SessionService.CreateSession(session); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	writeJSON(w, http.StatusCreated, toSessionResponse(session))
}

func (h *Handler) handleGetSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	session, err := h.SessionService.FindSessionByID(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "session not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to find session")
		return
	}
	writeJSON(w, http.StatusOK, toSessionResponse(session))
}

func (h *Handler) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.SessionService.EndSession(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "session not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to end session")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Client handlers (stubs) ---

func (h *Handler) handleListClients(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, []any{})
}

func (h *Handler) handleGetClient(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotFound, "client not found")
}

// --- OpenAPI handler ---

func (h *Handler) handleOpenAPI(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, openapiSpec)
}

var openapiSpec = map[string]any{
	"openapi": "3.1.0",
	"info": map[string]any{
		"title":   "CrossTalk API",
		"version": "0.1.0",
	},
	"components": map[string]any{
		"securitySchemes": map[string]any{
			"bearerAuth": map[string]any{
				"type":         "http",
				"scheme":       "bearer",
				"description":  "API token (ct_ prefixed)",
			},
		},
		"schemas": map[string]any{
			"Error": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"error": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"status":  map[string]any{"type": "integer"},
							"message": map[string]any{"type": "string"},
						},
					},
				},
			},
			"User": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":         map[string]any{"type": "string"},
					"username":   map[string]any{"type": "string"},
					"created_at": map[string]any{"type": "string", "format": "date-time"},
				},
			},
			"Token": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":         map[string]any{"type": "string"},
					"name":       map[string]any{"type": "string"},
					"user_id":    map[string]any{"type": "string"},
					"created_at": map[string]any{"type": "string", "format": "date-time"},
				},
			},
			"SessionTemplate": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":         map[string]any{"type": "string"},
					"name":       map[string]any{"type": "string"},
					"is_default": map[string]any{"type": "boolean"},
					"roles":      map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/Role"}},
					"mappings":   map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/Mapping"}},
					"created_at": map[string]any{"type": "string", "format": "date-time"},
					"updated_at": map[string]any{"type": "string", "format": "date-time"},
				},
			},
			"Role": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":         map[string]any{"type": "string"},
					"multi_client": map[string]any{"type": "boolean"},
				},
			},
			"Mapping": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"source": map[string]any{"type": "string"},
					"sink":   map[string]any{"type": "string"},
				},
			},
			"Session": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":          map[string]any{"type": "string"},
					"template_id": map[string]any{"type": "string"},
					"name":        map[string]any{"type": "string"},
					"status":      map[string]any{"type": "string", "enum": []string{"waiting", "active", "ended"}},
					"created_at":  map[string]any{"type": "string", "format": "date-time"},
					"ended_at":    map[string]any{"type": "string", "format": "date-time", "nullable": true},
				},
			},
			"SessionClient": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":              map[string]any{"type": "string"},
					"session_id":      map[string]any{"type": "string"},
					"role":            map[string]any{"type": "string"},
					"client_id":       map[string]any{"type": "string"},
					"status":          map[string]any{"type": "string", "enum": []string{"connected", "disconnected"}},
					"connected_at":    map[string]any{"type": "string", "format": "date-time"},
					"disconnected_at": map[string]any{"type": "string", "format": "date-time", "nullable": true},
				},
			},
		},
	},
	"paths": map[string]any{
		"/api/auth/login": map[string]any{
			"post": map[string]any{
				"summary":     "Authenticate with username and password",
				"operationId": "login",
				"tags":        []string{"auth"},
				"requestBody": map[string]any{
					"required": true,
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"username": map[string]any{"type": "string"},
									"password": map[string]any{"type": "string"},
								},
								"required": []string{"username", "password"},
							},
						},
					},
				},
				"responses": map[string]any{
					"200": map[string]any{"description": "Login successful, returns API token"},
					"401": map[string]any{"description": "Invalid credentials"},
				},
			},
		},
		"/api/auth/logout": map[string]any{
			"post": map[string]any{
				"summary":     "Revoke the current API token",
				"operationId": "logout",
				"tags":        []string{"auth"},
				"security":    []map[string]any{{"bearerAuth": []string{}}},
				"responses": map[string]any{
					"204": map[string]any{"description": "Logged out"},
					"401": map[string]any{"description": "Unauthorized"},
				},
			},
		},
		"/api/webrtc/token": map[string]any{
			"post": map[string]any{
				"summary":     "Get WebRTC signaling connection info (use API token directly for WS auth)",
				"operationId": "getWebRTCToken",
				"tags":        []string{"auth"},
				"security":    []map[string]any{{"bearerAuth": []string{}}},
				"responses": map[string]any{
					"200": map[string]any{"description": "Returns signaling connection info"},
				},
			},
		},
		"/api/users": map[string]any{
			"get": map[string]any{
				"summary":     "List all users",
				"operationId": "listUsers",
				"tags":        []string{"users"},
				"security":    []map[string]any{{"bearerAuth": []string{}}},
				"responses": map[string]any{
					"200": map[string]any{"description": "Array of users"},
				},
			},
			"post": map[string]any{
				"summary":     "Create a new user",
				"operationId": "createUser",
				"tags":        []string{"users"},
				"security":    []map[string]any{{"bearerAuth": []string{}}},
				"requestBody": map[string]any{
					"required": true,
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"username": map[string]any{"type": "string"},
									"password": map[string]any{"type": "string"},
								},
								"required": []string{"username", "password"},
							},
						},
					},
				},
				"responses": map[string]any{
					"201": map[string]any{"description": "User created"},
					"409": map[string]any{"description": "Username already taken"},
				},
			},
		},
		"/api/users/{id}": map[string]any{
			"patch": map[string]any{
				"summary":     "Update a user",
				"operationId": "updateUser",
				"tags":        []string{"users"},
				"security":    []map[string]any{{"bearerAuth": []string{}}},
				"parameters":  []map[string]any{{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}}},
				"responses": map[string]any{
					"200": map[string]any{"description": "User updated"},
					"404": map[string]any{"description": "User not found"},
				},
			},
			"delete": map[string]any{
				"summary":     "Delete a user",
				"operationId": "deleteUser",
				"tags":        []string{"users"},
				"security":    []map[string]any{{"bearerAuth": []string{}}},
				"parameters":  []map[string]any{{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}}},
				"responses": map[string]any{
					"204": map[string]any{"description": "User deleted"},
					"404": map[string]any{"description": "User not found"},
				},
			},
		},
		"/api/tokens": map[string]any{
			"get": map[string]any{
				"summary":     "List all API tokens",
				"operationId": "listTokens",
				"tags":        []string{"tokens"},
				"security":    []map[string]any{{"bearerAuth": []string{}}},
				"responses": map[string]any{
					"200": map[string]any{"description": "Array of tokens"},
				},
			},
			"post": map[string]any{
				"summary":     "Create a new API token",
				"operationId": "createToken",
				"tags":        []string{"tokens"},
				"security":    []map[string]any{{"bearerAuth": []string{}}},
				"requestBody": map[string]any{
					"required": true,
					"content": map[string]any{
						"application/json": map[string]any{
							"schema": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"name": map[string]any{"type": "string"},
								},
								"required": []string{"name"},
							},
						},
					},
				},
				"responses": map[string]any{
					"201": map[string]any{"description": "Token created, plaintext returned once"},
				},
			},
		},
		"/api/tokens/{id}": map[string]any{
			"delete": map[string]any{
				"summary":     "Revoke an API token",
				"operationId": "deleteToken",
				"tags":        []string{"tokens"},
				"security":    []map[string]any{{"bearerAuth": []string{}}},
				"parameters":  []map[string]any{{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}}},
				"responses": map[string]any{
					"204": map[string]any{"description": "Token revoked"},
					"404": map[string]any{"description": "Token not found"},
				},
			},
		},
		"/api/templates": map[string]any{
			"get": map[string]any{
				"summary":     "List all session templates",
				"operationId": "listTemplates",
				"tags":        []string{"templates"},
				"security":    []map[string]any{{"bearerAuth": []string{}}},
				"responses": map[string]any{
					"200": map[string]any{"description": "Array of session templates"},
				},
			},
			"post": map[string]any{
				"summary":     "Create a session template",
				"operationId": "createTemplate",
				"tags":        []string{"templates"},
				"security":    []map[string]any{{"bearerAuth": []string{}}},
				"responses": map[string]any{
					"201": map[string]any{"description": "Template created"},
				},
			},
		},
		"/api/templates/{id}": map[string]any{
			"get": map[string]any{
				"summary":     "Get a session template by ID",
				"operationId": "getTemplate",
				"tags":        []string{"templates"},
				"security":    []map[string]any{{"bearerAuth": []string{}}},
				"parameters":  []map[string]any{{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}}},
				"responses": map[string]any{
					"200": map[string]any{"description": "Session template detail"},
					"404": map[string]any{"description": "Template not found"},
				},
			},
			"put": map[string]any{
				"summary":     "Update a session template",
				"operationId": "updateTemplate",
				"tags":        []string{"templates"},
				"security":    []map[string]any{{"bearerAuth": []string{}}},
				"parameters":  []map[string]any{{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}}},
				"responses": map[string]any{
					"200": map[string]any{"description": "Template updated"},
					"404": map[string]any{"description": "Template not found"},
				},
			},
			"delete": map[string]any{
				"summary":     "Delete a session template",
				"operationId": "deleteTemplate",
				"tags":        []string{"templates"},
				"security":    []map[string]any{{"bearerAuth": []string{}}},
				"parameters":  []map[string]any{{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}}},
				"responses": map[string]any{
					"204": map[string]any{"description": "Template deleted"},
					"404": map[string]any{"description": "Template not found"},
				},
			},
		},
		"/api/sessions": map[string]any{
			"get": map[string]any{
				"summary":     "List all sessions",
				"operationId": "listSessions",
				"tags":        []string{"sessions"},
				"security":    []map[string]any{{"bearerAuth": []string{}}},
				"responses": map[string]any{
					"200": map[string]any{"description": "Array of sessions"},
				},
			},
			"post": map[string]any{
				"summary":     "Create a new session",
				"operationId": "createSession",
				"tags":        []string{"sessions"},
				"security":    []map[string]any{{"bearerAuth": []string{}}},
				"responses": map[string]any{
					"201": map[string]any{"description": "Session created"},
				},
			},
		},
		"/api/sessions/{id}": map[string]any{
			"get": map[string]any{
				"summary":     "Get session detail",
				"operationId": "getSession",
				"tags":        []string{"sessions"},
				"security":    []map[string]any{{"bearerAuth": []string{}}},
				"parameters":  []map[string]any{{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}}},
				"responses": map[string]any{
					"200": map[string]any{"description": "Session detail"},
					"404": map[string]any{"description": "Session not found"},
				},
			},
			"delete": map[string]any{
				"summary":     "End a session",
				"operationId": "endSession",
				"tags":        []string{"sessions"},
				"security":    []map[string]any{{"bearerAuth": []string{}}},
				"parameters":  []map[string]any{{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}}},
				"responses": map[string]any{
					"204": map[string]any{"description": "Session ended"},
					"404": map[string]any{"description": "Session not found"},
				},
			},
		},
		"/api/clients": map[string]any{
			"get": map[string]any{
				"summary":     "List all connected clients",
				"operationId": "listClients",
				"tags":        []string{"clients"},
				"security":    []map[string]any{{"bearerAuth": []string{}}},
				"responses": map[string]any{
					"200": map[string]any{"description": "Array of session clients"},
				},
			},
		},
		"/api/clients/{id}": map[string]any{
			"get": map[string]any{
				"summary":     "Get client detail",
				"operationId": "getClient",
				"tags":        []string{"clients"},
				"security":    []map[string]any{{"bearerAuth": []string{}}},
				"parameters":  []map[string]any{{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}}},
				"responses": map[string]any{
					"200": map[string]any{"description": "Client detail"},
					"404": map[string]any{"description": "Client not found"},
				},
			},
		},
		"/api/openapi.json": map[string]any{
			"get": map[string]any{
				"summary":     "Get the OpenAPI specification",
				"operationId": "getOpenAPISpec",
				"tags":        []string{"meta"},
				"security":    []map[string]any{{"bearerAuth": []string{}}},
				"responses": map[string]any{
					"200": map[string]any{"description": "OpenAPI 3.1.0 specification"},
				},
			},
		},
	},
}

// --- Test-only handlers ---

// handleTestReset truncates all application tables. Only available when
// TestMode is true.
func (h *Handler) handleTestReset(w http.ResponseWriter, _ *http.Request) {
	if h.DB == nil {
		writeError(w, http.StatusInternalServerError, "test mode DB not configured")
		return
	}

	tables := []string{
		"session_clients",
		"sessions",
		"session_templates",
		"api_tokens",
		"users",
	}
	for _, table := range tables {
		if _, err := h.DB.Exec("DELETE FROM " + table); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to truncate "+table+": "+err.Error())
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
