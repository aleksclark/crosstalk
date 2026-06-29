package mixer

import "errors"

// Errors returned by the mixer package.
var (
	ErrBufferTooSmall = errors.New("mixer: output buffer too small")
	ErrInputNotFound  = errors.New("mixer: input not found")
	ErrMixerStopped   = errors.New("mixer: mixer is stopped")
)
