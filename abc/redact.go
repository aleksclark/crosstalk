package abc

import (
	"net/url"
	"strings"
)

const redactedToken = "[redacted]"

// RedactURL strips token query values (and any other credential-like keys)
// from a URL so logs never include the ABC credential.
func RedactURL(raw string) string {
	if raw == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	changed := false
	for key := range q {
		if isCredentialQueryKey(key) {
			q.Set(key, redactedToken)
			changed = true
		}
	}
	if changed {
		u.RawQuery = q.Encode()
	}
	return u.String()
}

func isCredentialQueryKey(key string) bool {
	switch strings.ToLower(key) {
	case "token", "access_token", "auth", "authorization", "secret", "password":
		return true
	default:
		return false
	}
}

func redactTokenSubstrings(text, token string) string {
	if text == "" {
		return text
	}
	out := RedactURL(text)
	if token != "" && token != redactedToken && strings.Contains(out, token) {
		out = strings.ReplaceAll(out, token, redactedToken)
	}
	return out
}

func redactError(err error, token string) error {
	if err == nil {
		return nil
	}
	msg := redactTokenSubstrings(err.Error(), token)
	if msg == err.Error() {
		return err
	}
	return &redactedError{msg: msg, cause: err}
}

type redactedError struct {
	msg   string
	cause error
}

func (e *redactedError) Error() string { return e.msg }
func (e *redactedError) Unwrap() error { return e.cause }
