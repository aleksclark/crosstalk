package display

import (
	"context"
	"log/slog"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// IdleMonitor reads PCM audio from an input source and feeds a LevelMeter
// so the VU meter reflects the input port even when no session is active.
//
// A live capture (CaptureSource) taps the same device and feeds the meter
// directly, so the idle monitor releases the device whenever gate() reports
// an active capture — this avoids conflicting with single-open ALSA hw
// devices.
type IdleMonitor struct {
	meter *LevelMeter
	gate  func() bool
}

// NewIdleMonitor creates an idle input monitor that writes PCM samples into
// meter. gate reports whether a live capture currently owns the device; when
// it returns true the monitor stops reading and yields the device.
func NewIdleMonitor(meter *LevelMeter, gate func() bool) *IdleMonitor {
	return &IdleMonitor{meter: meter, gate: gate}
}

// Run continuously monitors sourceName until ctx is cancelled.
func (m *IdleMonitor) Run(ctx context.Context, sourceName string) {
	if m.meter == nil || sourceName == "" {
		return
	}

	for {
		if ctx.Err() != nil {
			return
		}

		if m.active() {
			// A live capture owns the device and feeds the meter itself.
			if !sleep(ctx, 500*time.Millisecond) {
				return
			}
			continue
		}

		if err := m.runOnce(ctx, sourceName); err != nil && ctx.Err() == nil {
			slog.Debug("idle vu monitor: source read failed, retrying",
				"source", sourceName, "error", err)
		}

		if !sleep(ctx, 500*time.Millisecond) {
			return
		}
	}
}

func (m *IdleMonitor) active() bool {
	return m.gate != nil && m.gate()
}

func (m *IdleMonitor) runOnce(ctx context.Context, sourceName string) error {
	var cmd *exec.Cmd
	if strings.HasPrefix(sourceName, "hw:") || strings.HasPrefix(sourceName, "plughw:") {
		cmd = exec.CommandContext(ctx, "arecord",
			"-D", sourceName,
			"-f", "S16_LE", "-r", "48000", "-c", "1",
			"-t", "raw", "-")
	} else {
		cmd = exec.CommandContext(ctx, "pw-record",
			"--format=s16", "--rate=48000", "--channels=1",
			"--target="+sourceName, "-")
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	defer cmd.Process.Kill() //nolint:errcheck

	// 20ms frames of S16LE mono at 48kHz = 960 samples = 1920 bytes.
	buf := make([]byte, 1920)
	for {
		if ctx.Err() != nil {
			return nil
		}
		// Yield the device the moment a live capture starts.
		if m.active() {
			return nil
		}
		n, err := stdout.Read(buf)
		if err != nil {
			return err
		}
		m.meter.Write(buf[:n]) //nolint:errcheck
	}
}

func sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}
