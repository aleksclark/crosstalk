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
// frames, and flushing them to the SPI display.
type Service struct {
	status     *Status
	spiPath    string
	dcGPIO     int
	rstGPIO    int
	inMeter    *LevelMeter
	outMeter   *LevelMeter
}

// NewService creates a display service.
func NewService(spiPath string, dcGPIO, rstGPIO int) *Service {
	s := &Service{
		status:  &Status{},
		spiPath: spiPath,
		dcGPIO:  dcGPIO,
		rstGPIO: rstGPIO,
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
	disp, err := OpenDisplay(s.spiPath, s.dcGPIO, s.rstGPIO)
	if err != nil {
		return err
	}
	defer disp.Close()

	w, h := disp.Size()
	slog.Info("display opened", "spi", s.spiPath, "width", w, "height", h)

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
