package abc

import (
	"errors"
	"fmt"
	"net/http"
)

// AuthError is a non-retryable authentication failure.
type AuthError struct {
	StatusCode int
}

func (e *AuthError) Error() string {
	if e == nil {
		return "abc: authentication failed"
	}
	return fmt.Sprintf("abc: authentication failed (HTTP %d)", e.StatusCode)
}

// IsAuthError reports whether err is or wraps an AuthError.
func IsAuthError(err error) bool {
	var authErr *AuthError
	return errors.As(err, &authErr)
}

// ProtocolError is a non-retryable control-protocol failure.
type ProtocolError struct {
	Reason string
}

func (e *ProtocolError) Error() string {
	if e == nil || e.Reason == "" {
		return "abc: protocol error"
	}
	return "abc: protocol error: " + e.Reason
}

// IsProtocolError reports whether err is or wraps a ProtocolError.
func IsProtocolError(err error) bool {
	var protoErr *ProtocolError
	return errors.As(err, &protoErr)
}

func authErrorFromStatus(status int) error {
	if status == 0 {
		status = http.StatusUnauthorized
	}
	return &AuthError{StatusCode: status}
}
