// Package auth provides JWT authentication and authorization.
package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/oklog/ulid/v2"
	"golang.org/x/crypto/bcrypt"

	crosstalk "github.com/aleksclark/crosstalk/server"
)

// Claims represents the JWT claims used by CrossTalk.
type Claims struct {
	jwt.RegisteredClaims
	Role string `json:"role"`
}

// Config holds auth configuration.
type Config struct {
	Secret          string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
}

// DefaultConfig returns sensible defaults for auth config.
func DefaultConfig() Config {
	return Config{
		Secret:          "change-me-in-production",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
	}
}

// Service handles authentication logic.
type Service struct {
	cfg            Config
	users          crosstalk.UserService
	refreshTokens  crosstalk.RefreshTokenService
}

// NewService creates an auth service.
func NewService(cfg Config, users crosstalk.UserService, refreshTokens crosstalk.RefreshTokenService) *Service {
	return &Service{
		cfg:           cfg,
		users:         users,
		refreshTokens: refreshTokens,
	}
}

// TokenPair holds access + refresh tokens.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// Login authenticates a user and returns a token pair.
func (s *Service) Login(ctx context.Context, username, password string) (*TokenPair, error) {
	user, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	return s.generateTokenPair(ctx, user)
}

// Refresh exchanges a refresh token for a new token pair.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (*TokenPair, error) {
	hash := HashToken(refreshToken)
	rt, err := s.refreshTokens.GetByHash(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token")
	}

	if time.Now().After(rt.ExpiresAt) {
		_ = s.refreshTokens.DeleteByHash(ctx, hash)
		return nil, fmt.Errorf("refresh token expired")
	}

	// Delete old refresh token (rotation)
	_ = s.refreshTokens.DeleteByHash(ctx, hash)

	user, err := s.users.Get(ctx, rt.UserID)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}

	return s.generateTokenPair(ctx, user)
}

// Logout revokes a refresh token.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	hash := HashToken(refreshToken)
	return s.refreshTokens.DeleteByHash(ctx, hash)
}

// ValidateAccessToken validates an access token and returns claims.
func (s *Service) ValidateAccessToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(s.cfg.Secret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}

func (s *Service) generateTokenPair(ctx context.Context, user *crosstalk.User) (*TokenPair, error) {
	now := time.Now()

	// Access token
	accessClaims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.cfg.AccessTokenTTL)),
		},
		Role: user.Role,
	}
	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessStr, err := accessToken.SignedString([]byte(s.cfg.Secret))
	if err != nil {
		return nil, fmt.Errorf("sign access token: %w", err)
	}

	// Refresh token (opaque)
	refreshStr := ulid.Make().String() + ulid.Make().String()
	refreshHash := HashToken(refreshStr)

	rt := &crosstalk.RefreshToken{
		ID:        ulid.Make().String(),
		UserID:    user.ID,
		TokenHash: refreshHash,
		ExpiresAt: now.Add(s.cfg.RefreshTokenTTL),
		CreatedAt: now,
	}
	if err := s.refreshTokens.Create(ctx, rt); err != nil {
		return nil, fmt.Errorf("store refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessStr,
		RefreshToken: refreshStr,
	}, nil
}

// HashToken creates a SHA-256 hash of a token string.
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// HashPassword hashes a password using bcrypt.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// contextKey is a private type for context keys.
type contextKey string

const claimsKey contextKey = "auth_claims"

// ContextWithClaims stores claims in context.
func ContextWithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, claimsKey, claims)
}

// ClaimsFromContext extracts claims from context.
func ClaimsFromContext(ctx context.Context) *Claims {
	claims, _ := ctx.Value(claimsKey).(*Claims)
	return claims
}
