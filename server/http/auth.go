package http

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	crosstalk "github.com/aleksclark/crosstalk/server"
	"golang.org/x/crypto/bcrypt"
)

// contextKey is an unexported type used for context keys in this package.
type contextKey int

const (
	// tokenContextKey is the context key for the authenticated API token.
	tokenContextKey contextKey = iota
)

// HashToken returns the hex-encoded SHA-256 hash of a plaintext token.
func HashToken(plaintext string) string {
	h := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(h[:])
}

// HashPassword returns a bcrypt hash of the plaintext password.
func HashPassword(plaintext string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword reports whether the plaintext password matches the bcrypt hash.
func CheckPassword(hash, plaintext string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext)) == nil
}

// GenerateToken generates a new random API token with the "ct_" prefix
// followed by 32 random bytes encoded as hex (64 hex characters).
func GenerateToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return "ct_" + hex.EncodeToString(b)
}

// AuthMiddleware returns HTTP middleware that authenticates requests using
// Bearer tokens. It extracts the token from the Authorization header,
// hashes it with SHA-256, and looks it up via the provided TokenService.
// If valid, the corresponding APIToken is stored in the request context.
func AuthMiddleware(tokenService crosstalk.TokenService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" {
				writeError(w, http.StatusUnauthorized, "missing authorization header")
				return
			}

			bearer, token, ok := strings.Cut(header, " ")
			if !ok || !strings.EqualFold(bearer, "Bearer") || token == "" {
				writeError(w, http.StatusUnauthorized, "invalid authorization header format")
				return
			}

			hash := HashToken(token)
			apiToken, err := tokenService.FindTokenByHash(hash)
			if err != nil || apiToken == nil {
				writeError(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}

			ctx := context.WithValue(r.Context(), tokenContextKey, apiToken)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// TokenFromContext extracts the authenticated APIToken from the request context.
// Returns nil if no token is present.
func TokenFromContext(ctx context.Context) *crosstalk.APIToken {
	tok, _ := ctx.Value(tokenContextKey).(*crosstalk.APIToken)
	return tok
}
