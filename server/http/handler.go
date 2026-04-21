package http

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	crosstalk "github.com/anthropics/crosstalk/server"
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
}

// Router builds and returns the chi router with the full route tree.
func (h *Handler) Router() *chi.Mux {
	r := chi.NewRouter()

	r.Route("/api", func(r chi.Router) {
		// Public: login does not require auth.
		r.Post("/auth/login", h.handleLogin)

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

	writeJSON(w, http.StatusOK, map[string]string{"token": plaintext})
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
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	token := "wrtc_" + hex.EncodeToString(b)

	lifetime, err := time.ParseDuration(h.Config.Auth.WebRTCTokenLifetime)
	if err != nil {
		lifetime = 24 * time.Hour
	}
	expiresAt := time.Now().UTC().Add(lifetime)

	writeJSON(w, http.StatusOK, map[string]string{
		"token":      token,
		"expires_at": expiresAt.Format(time.RFC3339),
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
	spec := map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":   "CrossTalk API",
			"version": "0.1.0",
		},
		"paths": map[string]any{},
	}
	writeJSON(w, http.StatusOK, spec)
}
