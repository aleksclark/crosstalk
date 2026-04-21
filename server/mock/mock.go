// Package mock provides in-memory mock implementations of domain service
// interfaces for testing. Mocks use function injection: callers set the
// XxxFn field to control behavior, and check XxxInvoked to verify calls.
package mock

import (
	crosstalk "github.com/anthropics/crosstalk/server"
)

// UserService is a mock implementation of crosstalk.UserService.
type UserService struct {
	FindUserByIDFn      func(id string) (*crosstalk.User, error)
	FindUserByIDInvoked bool

	FindUserByUsernameFn      func(username string) (*crosstalk.User, error)
	FindUserByUsernameInvoked bool

	CreateUserFn      func(user *crosstalk.User) error
	CreateUserInvoked bool

	DeleteUserFn      func(id string) error
	DeleteUserInvoked bool
}

func (s *UserService) FindUserByID(id string) (*crosstalk.User, error) {
	s.FindUserByIDInvoked = true
	return s.FindUserByIDFn(id)
}

func (s *UserService) FindUserByUsername(username string) (*crosstalk.User, error) {
	s.FindUserByUsernameInvoked = true
	return s.FindUserByUsernameFn(username)
}

func (s *UserService) CreateUser(user *crosstalk.User) error {
	s.CreateUserInvoked = true
	return s.CreateUserFn(user)
}

func (s *UserService) DeleteUser(id string) error {
	s.DeleteUserInvoked = true
	return s.DeleteUserFn(id)
}

// TokenService is a mock implementation of crosstalk.TokenService.
type TokenService struct {
	FindTokenByHashFn      func(hash string) (*crosstalk.APIToken, error)
	FindTokenByHashInvoked bool

	CreateTokenFn      func(token *crosstalk.APIToken) error
	CreateTokenInvoked bool

	DeleteTokenFn      func(id string) error
	DeleteTokenInvoked bool

	ListTokensFn      func() ([]crosstalk.APIToken, error)
	ListTokensInvoked bool
}

func (s *TokenService) FindTokenByHash(hash string) (*crosstalk.APIToken, error) {
	s.FindTokenByHashInvoked = true
	return s.FindTokenByHashFn(hash)
}

func (s *TokenService) CreateToken(token *crosstalk.APIToken) error {
	s.CreateTokenInvoked = true
	return s.CreateTokenFn(token)
}

func (s *TokenService) DeleteToken(id string) error {
	s.DeleteTokenInvoked = true
	return s.DeleteTokenFn(id)
}

func (s *TokenService) ListTokens() ([]crosstalk.APIToken, error) {
	s.ListTokensInvoked = true
	return s.ListTokensFn()
}

// SessionTemplateService is a mock implementation of crosstalk.SessionTemplateService.
type SessionTemplateService struct {
	FindTemplateByIDFn      func(id string) (*crosstalk.SessionTemplate, error)
	FindTemplateByIDInvoked bool

	ListTemplatesFn      func() ([]crosstalk.SessionTemplate, error)
	ListTemplatesInvoked bool

	CreateTemplateFn      func(tmpl *crosstalk.SessionTemplate) error
	CreateTemplateInvoked bool

	UpdateTemplateFn      func(tmpl *crosstalk.SessionTemplate) error
	UpdateTemplateInvoked bool

	DeleteTemplateFn      func(id string) error
	DeleteTemplateInvoked bool

	FindDefaultTemplateFn      func() (*crosstalk.SessionTemplate, error)
	FindDefaultTemplateInvoked bool
}

func (s *SessionTemplateService) FindTemplateByID(id string) (*crosstalk.SessionTemplate, error) {
	s.FindTemplateByIDInvoked = true
	return s.FindTemplateByIDFn(id)
}

func (s *SessionTemplateService) ListTemplates() ([]crosstalk.SessionTemplate, error) {
	s.ListTemplatesInvoked = true
	return s.ListTemplatesFn()
}

func (s *SessionTemplateService) CreateTemplate(tmpl *crosstalk.SessionTemplate) error {
	s.CreateTemplateInvoked = true
	return s.CreateTemplateFn(tmpl)
}

func (s *SessionTemplateService) UpdateTemplate(tmpl *crosstalk.SessionTemplate) error {
	s.UpdateTemplateInvoked = true
	return s.UpdateTemplateFn(tmpl)
}

func (s *SessionTemplateService) DeleteTemplate(id string) error {
	s.DeleteTemplateInvoked = true
	return s.DeleteTemplateFn(id)
}

func (s *SessionTemplateService) FindDefaultTemplate() (*crosstalk.SessionTemplate, error) {
	s.FindDefaultTemplateInvoked = true
	return s.FindDefaultTemplateFn()
}

// SessionService is a mock implementation of crosstalk.SessionService.
type SessionService struct {
	FindSessionByIDFn      func(id string) (*crosstalk.Session, error)
	FindSessionByIDInvoked bool

	ListSessionsFn      func() ([]crosstalk.Session, error)
	ListSessionsInvoked bool

	CreateSessionFn      func(session *crosstalk.Session) error
	CreateSessionInvoked bool

	EndSessionFn      func(id string) error
	EndSessionInvoked bool
}

func (s *SessionService) FindSessionByID(id string) (*crosstalk.Session, error) {
	s.FindSessionByIDInvoked = true
	return s.FindSessionByIDFn(id)
}

func (s *SessionService) ListSessions() ([]crosstalk.Session, error) {
	s.ListSessionsInvoked = true
	return s.ListSessionsFn()
}

func (s *SessionService) CreateSession(session *crosstalk.Session) error {
	s.CreateSessionInvoked = true
	return s.CreateSessionFn(session)
}

func (s *SessionService) EndSession(id string) error {
	s.EndSessionInvoked = true
	return s.EndSessionFn(id)
}
