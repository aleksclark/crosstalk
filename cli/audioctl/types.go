// Package audioctl applies absolute, allowlisted ALSA mixer controls for K2B USB audio.
package audioctl

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Stable machine error codes reported to the server / logs.
const (
	ErrCodeDeviceNotFound     = "DEVICE_NOT_FOUND"
	ErrCodeDeviceMismatch     = "DEVICE_MISMATCH"
	ErrCodeUnsupportedBackend = "UNSUPPORTED_BACKEND"
	ErrCodeControlNotFound    = "CONTROL_NOT_FOUND"
	ErrCodeAGCDisableFailed   = "AGC_DISABLE_FAILED"
	ErrCodeApplyFailed        = "APPLY_FAILED"
	ErrCodeReadbackFailed     = "READBACK_FAILED"
	ErrCodeInvalidCommand     = "INVALID_COMMAND"
)

// ApplyState is the per-control apply/readback result.
type ApplyState string

const (
	StateUnspecified    ApplyState = ""
	StateApplied        ApplyState = "applied"
	StateUnsupported    ApplyState = "unsupported"
	StateError          ApplyState = "error"
	StateDeviceMismatch ApplyState = "device_mismatch"
	StateStaleRevision  ApplyState = "stale_revision"
)

// Backend identifies the control plane for a device.
type Backend string

const (
	BackendALSA     Backend = "alsa"
	BackendPipeWire Backend = "pipewire"
	BackendUnknown  Backend = "unknown"
)

// DeviceCapability describes one discovered audio endpoint.
type DeviceCapability struct {
	DeviceUID          string
	Direction          string // "input", "output", or "both"
	Backend            Backend
	VendorID           string
	ProductID          string
	Serial             string
	Path               string
	ALSACardID         string
	CardName           string
	PCMRoute           string
	SupportsVolume     bool
	SupportsMute       bool
	SupportsGain       bool
	SupportsAGCDisable bool
	// Selected allowlisted control names (local only; never from server).
	OutputControl string
	CaptureControl string
	AGCControl     string
}

// OutputDesired is absolute desired playback mixer state.
type OutputDesired struct {
	DeviceUID     string
	VolumePercent *int // inclusive 0..100; nil = do not set
	Muted         *bool
}

// InputDesired is absolute desired capture mixer state.
type InputDesired struct {
	DeviceUID   string
	GainPercent *int // inclusive 0..100; nil = do not set
}

// Command is an absolute desired USB mixer state.
type Command struct {
	CommandID       string
	DesiredRevision uint64
	Output          *OutputDesired
	Input           *InputDesired
}

// OutputObserved is effective playback mixer readback.
type OutputObserved struct {
	DeviceUID     string
	VolumePercent *int
	Muted         *bool
	VolumeState   ApplyState
	MuteState     ApplyState
}

// InputObserved is effective capture mixer readback.
type InputObserved struct {
	DeviceUID   string
	GainPercent *int
	GainState   ApplyState
}

// Report is inventory and/or apply result.
type Report struct {
	CommandID       string
	DesiredRevision uint64
	Devices         []DeviceCapability
	Output          *OutputObserved
	Input           *InputObserved
	ErrorCode       string
	ErrorDetail     string
}

// Controller inventories devices and applies absolute mixer commands.
type Controller interface {
	// Inventory discovers configured endpoints and their capabilities.
	Inventory(ctx context.Context) (*Report, error)
	// Apply validates and applies an absolute command, then reads back.
	// Always returns a report (possibly with error codes); error is only for
	// unexpected internal failures.
	Apply(ctx context.Context, cmd Command) (*Report, error)
	// Readback returns current observed values without applying.
	Readback(ctx context.Context) (*Report, error)
}

// Config configures the ALSA USB controller.
type Config struct {
	// SourceRoute / SinkRoute are configured PCM names (hw:/plughw: preferred).
	SourceRoute string
	SinkRoute   string
	// SysfsRoot defaults to /sys. Tests inject a fake tree.
	SysfsRoot string
	// ProcAsoundCards defaults to /proc/asound/cards.
	ProcAsoundCards string
	// AmixerPath defaults to /usr/bin/amixer.
	AmixerPath string
	// Runner executes amixer; nil uses production exec runner.
	Runner Runner
	// Timeout bounds each amixer invocation.
	Timeout time.Duration
}

// ALSAController is the production Controller implementation.
type ALSAController struct {
	cfg Config
	mu  sync.Mutex

	// highestApplied tracks the highest desired revision applied in this process.
	// Lower revisions report STALE_REVISION without re-running mixer ops
	// (same revision is always re-read/applied safely).
	highestApplied uint64
}

// NewController builds an ALSAController from config.
func NewController(cfg Config) *ALSAController {
	if cfg.SysfsRoot == "" {
		cfg.SysfsRoot = "/sys"
	}
	if cfg.ProcAsoundCards == "" {
		cfg.ProcAsoundCards = "/proc/asound/cards"
	}
	if cfg.AmixerPath == "" {
		cfg.AmixerPath = "/usr/bin/amixer"
	}
	if cfg.Runner == nil {
		cfg.Runner = NewExecRunner()
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 3 * time.Second
	}
	return &ALSAController{cfg: cfg}
}

// Inventory implements Controller.
func (c *ALSAController) Inventory(ctx context.Context) (*Report, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.inventoryLocked(ctx)
}

// Readback implements Controller.
func (c *ALSAController) Readback(ctx context.Context) (*Report, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	rep, err := c.inventoryLocked(ctx)
	if err != nil {
		return nil, err
	}
	c.fillObserved(ctx, rep)
	return rep, nil
}

// Apply implements Controller.
func (c *ALSAController) Apply(ctx context.Context, cmd Command) (*Report, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	rep, err := c.inventoryLocked(ctx)
	if err != nil {
		return nil, err
	}
	rep.CommandID = cmd.CommandID
	rep.DesiredRevision = cmd.DesiredRevision

	if err := validateCommand(cmd); err != nil {
		rep.ErrorCode = ErrCodeInvalidCommand
		rep.ErrorDetail = sanitizeDetail(err.Error())
		rep.Output = &OutputObserved{VolumeState: StateError, MuteState: StateError}
		rep.Input = &InputObserved{GainState: StateError}
		return rep, nil
	}

	// Stale revision: lower than highest applied in this process.
	if cmd.DesiredRevision > 0 && c.highestApplied > 0 && cmd.DesiredRevision < c.highestApplied {
		rep.ErrorCode = ""
		rep.Output = &OutputObserved{
			VolumeState: StateStaleRevision,
			MuteState:   StateStaleRevision,
		}
		rep.Input = &InputObserved{GainState: StateStaleRevision}
		c.fillObserved(ctx, rep)
		return rep, nil
	}

	outObs := &OutputObserved{}
	inObs := &InputObserved{}
	rep.Output = outObs
	rep.Input = inObs

	// Resolve devices.
	var outDev, inDev *DeviceCapability
	if cmd.Output != nil {
		outObs.DeviceUID = cmd.Output.DeviceUID
		outDev = findDevice(rep.Devices, cmd.Output.DeviceUID)
		if outDev == nil {
			outObs.VolumeState = StateDeviceMismatch
			outObs.MuteState = StateDeviceMismatch
			if rep.ErrorCode == "" {
				rep.ErrorCode = ErrCodeDeviceMismatch
				rep.ErrorDetail = sanitizeDetail("output device_uid not active")
			}
		} else if outDev.Backend != BackendALSA {
			outObs.VolumeState = StateUnsupported
			outObs.MuteState = StateUnsupported
			if rep.ErrorCode == "" {
				rep.ErrorCode = ErrCodeUnsupportedBackend
				rep.ErrorDetail = sanitizeDetail("output backend is " + string(outDev.Backend))
			}
		}
	}
	if cmd.Input != nil {
		inObs.DeviceUID = cmd.Input.DeviceUID
		inDev = findDevice(rep.Devices, cmd.Input.DeviceUID)
		if inDev == nil {
			inObs.GainState = StateDeviceMismatch
			if rep.ErrorCode == "" {
				rep.ErrorCode = ErrCodeDeviceMismatch
				rep.ErrorDetail = sanitizeDetail("input device_uid not active")
			}
		} else if inDev.Backend != BackendALSA {
			inObs.GainState = StateUnsupported
			if rep.ErrorCode == "" {
				rep.ErrorCode = ErrCodeUnsupportedBackend
				rep.ErrorDetail = sanitizeDetail("input backend is " + string(inDev.Backend))
			}
		}
	}

	// Apply order: volume → mute → disable AGC → capture gain. Report each result.
	if cmd.Output != nil && outDev != nil && outDev.Backend == BackendALSA {
		if cmd.Output.VolumePercent != nil {
			c.applyVolume(ctx, outDev, *cmd.Output.VolumePercent, outObs, rep)
		}
		if cmd.Output.Muted != nil {
			c.applyMute(ctx, outDev, *cmd.Output.Muted, outObs, rep)
		}
	}
	if cmd.Input != nil && inDev != nil && inDev.Backend == BackendALSA && cmd.Input.GainPercent != nil {
		c.applyGain(ctx, inDev, *cmd.Input.GainPercent, inObs, rep)
	}

	// Always fill remaining observed values via readback where not already set.
	c.fillObserved(ctx, rep)

	if cmd.DesiredRevision > c.highestApplied {
		c.highestApplied = cmd.DesiredRevision
	}
	return rep, nil
}

func (c *ALSAController) inventoryLocked(ctx context.Context) (*Report, error) {
	_ = ctx
	rep := &Report{Devices: make([]DeviceCapability, 0, 2)}

	// Discover from configured routes (source + sink may share a card).
	seen := map[string]struct{}{}
	for _, route := range []string{c.cfg.SourceRoute, c.cfg.SinkRoute} {
		if route == "" {
			continue
		}
		dev, err := c.resolveRoute(route)
		if err != nil {
			// Keep going; report empty inventory with code if nothing found later.
			continue
		}
		if _, ok := seen[dev.DeviceUID]; ok {
			// Merge direction if same device.
			for i := range rep.Devices {
				if rep.Devices[i].DeviceUID == dev.DeviceUID {
					rep.Devices[i].Direction = mergeDirection(rep.Devices[i].Direction, dev.Direction)
					if rep.Devices[i].PCMRoute == "" {
						rep.Devices[i].PCMRoute = dev.PCMRoute
					}
				}
			}
			continue
		}
		seen[dev.DeviceUID] = struct{}{}
		// Probe mixer controls for ALSA devices.
		if dev.Backend == BackendALSA && dev.ALSACardID != "" {
			c.probeControls(ctx, &dev)
		}
		rep.Devices = append(rep.Devices, dev)
		if len(rep.Devices) >= 8 {
			break
		}
	}
	return rep, nil
}

func (c *ALSAController) resolveRoute(route string) (DeviceCapability, error) {
	if isPipeWireRoute(route) {
		return DeviceCapability{
			DeviceUID: "pipewire:" + clamp(route, 100),
			Direction: "both",
			Backend:   BackendPipeWire,
			PCMRoute:  route,
		}, nil
	}

	cardID, devN, ok := parseALSARoute(route)
	if !ok {
		return DeviceCapability{}, fmt.Errorf("%s: unparseable route", ErrCodeInvalidCommand)
	}
	_ = devN

	usb, err := resolveUSBIdentity(c.cfg.SysfsRoot, c.cfg.ProcAsoundCards, cardID)
	if err != nil {
		return DeviceCapability{}, err
	}
	uid := canonicalUID(usb)
	dir := "both"
	return DeviceCapability{
		DeviceUID:  uid,
		Direction:  dir,
		Backend:    BackendALSA,
		VendorID:   usb.VendorID,
		ProductID:  usb.ProductID,
		Serial:     usb.Serial,
		Path:       usb.PathTag,
		ALSACardID: cardID,
		CardName:   usb.CardName,
		PCMRoute:   route,
	}, nil
}

func findDevice(devs []DeviceCapability, uid string) *DeviceCapability {
	for i := range devs {
		if devs[i].DeviceUID == uid {
			return &devs[i]
		}
	}
	return nil
}

func mergeDirection(a, b string) string {
	if a == b || b == "" {
		return a
	}
	if a == "" {
		return b
	}
	return "both"
}

func clamp(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func validateCommand(cmd Command) error {
	if cmd.Output == nil && cmd.Input == nil {
		return fmt.Errorf("empty command")
	}
	if cmd.Output != nil {
		if cmd.Output.DeviceUID == "" || len(cmd.Output.DeviceUID) > 128 {
			return fmt.Errorf("invalid output device_uid")
		}
		if cmd.Output.VolumePercent != nil {
			v := *cmd.Output.VolumePercent
			if v < 0 || v > 100 {
				return fmt.Errorf("output volume out of range")
			}
		}
	}
	if cmd.Input != nil {
		if cmd.Input.DeviceUID == "" || len(cmd.Input.DeviceUID) > 128 {
			return fmt.Errorf("invalid input device_uid")
		}
		if cmd.Input.GainPercent != nil {
			v := *cmd.Input.GainPercent
			if v < 0 || v > 100 {
				return fmt.Errorf("input gain out of range")
			}
		}
	}
	if len(cmd.CommandID) > 128 {
		return fmt.Errorf("command_id too long")
	}
	return nil
}
