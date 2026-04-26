package display

import (
	"context"
	"encoding/binary"
	"log/slog"
	"math"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// VUMonitor reads PCM audio from PipeWire devices and computes RMS
// levels for the VU meter display.
type VUMonitor struct {
	mu      sync.Mutex
	levelIn  float64
	levelOut float64
}

// NewVUMonitor creates a VU monitor.
func NewVUMonitor() *VUMonitor {
	return &VUMonitor{}
}

// Levels returns the current VU levels (0.0–1.0).
func (v *VUMonitor) Levels() (in, out float64) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.levelIn, v.levelOut
}

// Run starts monitoring the given PipeWire source and sink. Blocks
// until ctx is cancelled.
func (v *VUMonitor) Run(ctx context.Context, sourceName, sinkName string) {
	var wg sync.WaitGroup

	if sourceName != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v.monitorDevice(ctx, sourceName, true)
		}()
	}

	if sinkName != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v.monitorDevice(ctx, sinkName, false)
		}()
	}

	wg.Wait()
}

func (v *VUMonitor) monitorDevice(ctx context.Context, device string, isInput bool) {
	for {
		if ctx.Err() != nil {
			return
		}

		err := v.runOnce(ctx, device, isInput)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			slog.Debug("vu monitor: device read failed, retrying",
				"device", device, "input", isInput, "error", err)
		}

		// Decay to zero when not reading
		v.mu.Lock()
		if isInput {
			v.levelIn = 0
		} else {
			v.levelOut = 0
		}
		v.mu.Unlock()

		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func (v *VUMonitor) runOnce(ctx context.Context, device string, isInput bool) error {
	args := []string{
		"--format=s16",
		"--rate=48000",
		"--channels=1",
	}

	if isInput {
		args = append(args, "--target="+device)
	} else {
		args = append(args, "--target="+device+".monitor")
	}
	args = append(args, "-")

	cmd := exec.CommandContext(ctx, "pw-record", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	defer cmd.Process.Kill() //nolint:errcheck

	// Read 20ms frames of S16LE mono at 48kHz = 960 samples = 1920 bytes
	const frameBytes = 1920
	buf := make([]byte, frameBytes)
	decay := 0.85

	for {
		if ctx.Err() != nil {
			return nil
		}

		n, err := stdout.Read(buf)
		if err != nil {
			return err
		}
		if n < 2 {
			continue
		}

		// Compute RMS of S16LE samples
		samples := n / 2
		var sumSq float64
		for i := 0; i < samples; i++ {
			s := int16(binary.LittleEndian.Uint16(buf[i*2:]))
			sumSq += float64(s) * float64(s)
		}
		rms := math.Sqrt(sumSq / float64(samples))

		// Normalize: S16 max is 32767. Scale so typical speech (~2000-5000 RMS)
		// reads around 0.4-0.7 on the meter.
		level := rms / 16000.0
		if level > 1.0 {
			level = 1.0
		}

		v.mu.Lock()
		if isInput {
			if level > v.levelIn {
				v.levelIn = level
			} else {
				v.levelIn = v.levelIn*decay + level*(1-decay)
			}
		} else {
			if level > v.levelOut {
				v.levelOut = level
			} else {
				v.levelOut = v.levelOut*decay + level*(1-decay)
			}
		}
		v.mu.Unlock()
	}
}
