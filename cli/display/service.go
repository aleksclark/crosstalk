package display

import (
	"context"
	"log/slog"
	"time"
)

const (
	renderInterval      = 200 * time.Millisecond
	networkPollInterval = 5 * time.Second
)

// Service manages the display lifecycle: probing network, rendering
// frames, and flushing them to the framebuffer.
type Service struct {
	status  *Status
	fbPath  string
	inMeter  *LevelMeter
	outMeter *LevelMeter
}

// NewService creates a display service. Pass an empty fbPath to auto-detect.
func NewService(fbPath string) *Service {
	s := &Service{
		status: &Status{},
		fbPath: fbPath,
	}
	s.status.SetControlState("disconnected")
	return s
}

// SetLevelMeters sets the in-process PCM level meters for VU display.
func (s *Service) SetLevelMeters(in, out *LevelMeter) {
	s.inMeter = in
	s.outMeter = out
}

// Status returns the shared status object for external code to update.
func (s *Service) Status() *Status {
	return s.status
}

// Run opens the display and starts rendering. It blocks until the
// context is cancelled.
func (s *Service) Run(ctx context.Context) error {
	disp, err := OpenDisplay(s.fbPath)
	if err != nil {
		return err
	}
	defer disp.Close()

	w, h := disp.Size()
	slog.Info("display opened", "fb", s.fbPath, "width", w, "height", h)

	bl, err := OpenBacklight()
	if err != nil {
		slog.Warn("backlight not available", "error", err)
	} else {
		slog.Info("backlight opened", "max", bl.Max())
		bl.SetBrightness(1.0)
	}

	s.probeNetwork()

	renderTicker := time.NewTicker(renderInterval)
	defer renderTicker.Stop()

	networkTicker := time.NewTicker(networkPollInterval)
	defer networkTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			snap := s.status.Snapshot()
			snap.ControlState = "shutdown"
			img := Render(&snap)
			disp.Flush(img)
			if bl != nil {
				bl.SetBrightness(0)
			}
			return nil

		case <-networkTicker.C:
			s.probeNetwork()

		case <-renderTicker.C:
			var in, out float64
			if s.inMeter != nil {
				in = s.inMeter.Level()
			}
			if s.outMeter != nil {
				out = s.outMeter.Level()
			}
			s.status.SetVU(in, out)

			snap := s.status.Snapshot()
			img := Render(&snap)
			disp.Flush(img)
		}
	}
}

func (s *Service) probeNetwork() {
	iface, addr, up := ProbeNetwork()
	s.status.SetNetwork(iface, addr, up)
}
