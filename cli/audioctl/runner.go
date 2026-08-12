package audioctl

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"
)

// Runner executes a program with discrete argv (never a shell).
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error)
}

// ExecRunner is the production runner using exec.CommandContext.
type ExecRunner struct {
	// MaxOutput caps captured stdout/stderr.
	MaxOutput int
}

// NewExecRunner returns a production direct-exec runner.
func NewExecRunner() *ExecRunner {
	return &ExecRunner{MaxOutput: 64 * 1024}
}

// Run implements Runner.
func (r *ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	if name == "" {
		return nil, nil, errors.New("empty executable")
	}
	// Hard reject shell interpreters — defense in depth.
	base := name
	if i := bytes.LastIndexByte([]byte(name), '/'); i >= 0 {
		base = name[i+1:]
	}
	switch base {
	case "sh", "bash", "zsh", "dash", "fish", "csh", "ksh":
		return nil, nil, errors.New("shell execution is forbidden")
	}
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	max := r.MaxOutput
	if max <= 0 {
		max = 64 * 1024
	}
	cmd.Stdout = &limitedWriter{buf: &stdout, max: max}
	cmd.Stderr = &limitedWriter{buf: &stderr, max: max}
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

type limitedWriter struct {
	buf *bytes.Buffer
	max int
	n   int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if w.n >= w.max {
		return len(p), nil
	}
	remain := w.max - w.n
	if len(p) > remain {
		_, _ = w.buf.Write(p[:remain])
		w.n = w.max
		return len(p), nil
	}
	n, err := w.buf.Write(p)
	w.n += n
	return n, err
}

// RecordingRunner records invocations for tests and returns scripted results.
type RecordingRunner struct {
	Calls [][]string
	// Handler returns stdout, stderr, err for each call. If nil, succeeds empty.
	Handler func(name string, args []string) (stdout, stderr []byte, err error)
}

// Run implements Runner.
func (r *RecordingRunner) Run(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	_ = ctx
	call := append([]string{name}, args...)
	r.Calls = append(r.Calls, call)
	if r.Handler != nil {
		return r.Handler(name, args)
	}
	return nil, nil, nil
}

// CallCount returns number of recorded invocations.
func (r *RecordingRunner) CallCount() int { return len(r.Calls) }

// withTimeout derives a child context with controller timeout.
func withTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		d = 3 * time.Second
	}
	return context.WithTimeout(ctx, d)
}
