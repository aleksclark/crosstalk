package audioctl

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Allowlisted semantic simple-control names (never from server).
var (
	outputControlAllowlist  = []string{"Speaker", "Headphone", "PCM"}
	captureControlAllowlist = []string{"Mic", "Capture"}
	agcControlName          = "Auto Gain Control"
)

var (
	// amixer scontrols: Simple mixer control 'Mic',0
	scontrolsRe = regexp.MustCompile(`Simple mixer control '([^']+)'`)
	// [42%] or [42%
	percentRe = regexp.MustCompile(`\[(\d{1,3})%\]`)
	// [on] / [off] switch state (prefer last relevant)
	switchRe = regexp.MustCompile(`\[(on|off)\]`)
)

func (c *ALSAController) probeControls(ctx context.Context, dev *DeviceCapability) {
	names, err := c.listSimpleControls(ctx, dev.ALSACardID)
	if err != nil {
		return
	}
	set := map[string]struct{}{}
	for _, n := range names {
		set[n] = struct{}{}
	}
	for _, n := range outputControlAllowlist {
		if _, ok := set[n]; ok {
			dev.OutputControl = n
			dev.SupportsVolume = true
			dev.SupportsMute = true
			break
		}
	}
	for _, n := range captureControlAllowlist {
		if _, ok := set[n]; ok {
			dev.CaptureControl = n
			dev.SupportsGain = true
			break
		}
	}
	if _, ok := set[agcControlName]; ok {
		dev.AGCControl = agcControlName
		dev.SupportsAGCDisable = true
	}
}

func (c *ALSAController) listSimpleControls(ctx context.Context, cardID string) ([]string, error) {
	if !alsaCardIDRe.MatchString(cardID) {
		return nil, fmt.Errorf("%s: bad card id", ErrCodeInvalidCommand)
	}
	ctx, cancel := withTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	// amixer -D hw:CARD=<id> scontrols
	stdout, stderr, err := c.cfg.Runner.Run(ctx, c.cfg.AmixerPath,
		"-D", "hw:CARD="+cardID, "scontrols")
	if err != nil {
		return nil, fmt.Errorf("scontrols: %w (%s)", err, sanitizeDetail(string(stderr)))
	}
	var names []string
	for _, m := range scontrolsRe.FindAllSubmatch(stdout, -1) {
		if len(m) >= 2 {
			names = append(names, string(m[1]))
		}
	}
	return names, nil
}

func (c *ALSAController) applyVolume(ctx context.Context, dev *DeviceCapability, pct int, out *OutputObserved, rep *Report) {
	if dev.OutputControl == "" {
		out.VolumeState = StateUnsupported
		if rep.ErrorCode == "" {
			rep.ErrorCode = ErrCodeControlNotFound
			rep.ErrorDetail = sanitizeDetail("no allowlisted output volume control")
		}
		return
	}
	ctx, cancel := withTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	args := []string{"-M", "-D", "hw:CARD=" + dev.ALSACardID, "sset", dev.OutputControl, fmt.Sprintf("%d%%", pct)}
	_, stderr, err := c.cfg.Runner.Run(ctx, c.cfg.AmixerPath, args...)
	if err != nil {
		out.VolumeState = StateError
		if rep.ErrorCode == "" {
			rep.ErrorCode = ErrCodeApplyFailed
			rep.ErrorDetail = sanitizeDetail(string(stderr))
		}
		return
	}
	// Readback
	got, _, rerr := c.sgetPercent(ctx, dev.ALSACardID, dev.OutputControl)
	if rerr != nil {
		out.VolumeState = StateError
		if rep.ErrorCode == "" {
			rep.ErrorCode = ErrCodeReadbackFailed
			rep.ErrorDetail = sanitizeDetail(rerr.Error())
		}
		return
	}
	out.VolumePercent = &got
	out.VolumeState = StateApplied
}

func (c *ALSAController) applyMute(ctx context.Context, dev *DeviceCapability, muted bool, out *OutputObserved, rep *Report) {
	if dev.OutputControl == "" {
		out.MuteState = StateUnsupported
		if rep.ErrorCode == "" {
			rep.ErrorCode = ErrCodeControlNotFound
			rep.ErrorDetail = sanitizeDetail("no allowlisted output mute control")
		}
		return
	}
	ctx, cancel := withTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	state := "unmute"
	if muted {
		state = "mute"
	}
	args := []string{"-M", "-D", "hw:CARD=" + dev.ALSACardID, "sset", dev.OutputControl, state}
	_, stderr, err := c.cfg.Runner.Run(ctx, c.cfg.AmixerPath, args...)
	if err != nil {
		out.MuteState = StateError
		if rep.ErrorCode == "" {
			rep.ErrorCode = ErrCodeApplyFailed
			rep.ErrorDetail = sanitizeDetail(string(stderr))
		}
		return
	}
	got, rerr := c.sgetSwitch(ctx, dev.ALSACardID, dev.OutputControl)
	if rerr != nil {
		out.MuteState = StateError
		if rep.ErrorCode == "" {
			rep.ErrorCode = ErrCodeReadbackFailed
			rep.ErrorDetail = sanitizeDetail(rerr.Error())
		}
		return
	}
	// amixer switch [on]=unmuted, [off]=muted
	isMuted := !got
	out.Muted = &isMuted
	out.MuteState = StateApplied
}

func (c *ALSAController) applyGain(ctx context.Context, dev *DeviceCapability, pct int, in *InputObserved, rep *Report) {
	if dev.CaptureControl == "" {
		in.GainState = StateUnsupported
		if rep.ErrorCode == "" {
			rep.ErrorCode = ErrCodeControlNotFound
			rep.ErrorDetail = sanitizeDetail("no allowlisted capture control")
		}
		return
	}
	// Disable AGC first when present.
	if dev.AGCControl != "" {
		ctxAGC, cancel := withTimeout(ctx, c.cfg.Timeout)
		_, stderr, err := c.cfg.Runner.Run(ctxAGC, c.cfg.AmixerPath,
			"-M", "-D", "hw:CARD="+dev.ALSACardID, "sset", dev.AGCControl, "off")
		cancel()
		if err != nil {
			in.GainState = StateError
			if rep.ErrorCode == "" {
				rep.ErrorCode = ErrCodeAGCDisableFailed
				rep.ErrorDetail = sanitizeDetail(string(stderr))
			}
			return
		}
		// Readback AGC must be off.
		on, rerr := c.sgetSwitch(ctx, dev.ALSACardID, dev.AGCControl)
		if rerr != nil || on {
			in.GainState = StateError
			if rep.ErrorCode == "" {
				rep.ErrorCode = ErrCodeAGCDisableFailed
				if rerr != nil {
					rep.ErrorDetail = sanitizeDetail(rerr.Error())
				} else {
					rep.ErrorDetail = sanitizeDetail("AGC still on after disable")
				}
			}
			return
		}
	}

	ctx, cancel := withTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	args := []string{"-M", "-D", "hw:CARD=" + dev.ALSACardID, "sset", dev.CaptureControl, fmt.Sprintf("%d%%", pct), "cap"}
	_, stderr, err := c.cfg.Runner.Run(ctx, c.cfg.AmixerPath, args...)
	if err != nil {
		in.GainState = StateError
		if rep.ErrorCode == "" {
			rep.ErrorCode = ErrCodeApplyFailed
			rep.ErrorDetail = sanitizeDetail(string(stderr))
		}
		return
	}
	got, _, rerr := c.sgetPercent(ctx, dev.ALSACardID, dev.CaptureControl)
	if rerr != nil {
		in.GainState = StateError
		if rep.ErrorCode == "" {
			rep.ErrorCode = ErrCodeReadbackFailed
			rep.ErrorDetail = sanitizeDetail(rerr.Error())
		}
		return
	}
	in.GainPercent = &got
	in.GainState = StateApplied
}

func (c *ALSAController) sgetPercent(ctx context.Context, cardID, control string) (int, string, error) {
	ctx, cancel := withTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	stdout, stderr, err := c.cfg.Runner.Run(ctx, c.cfg.AmixerPath,
		"-M", "-D", "hw:CARD="+cardID, "sget", control)
	if err != nil {
		return 0, string(stderr), fmt.Errorf("sget: %w", err)
	}
	m := percentRe.FindSubmatch(stdout)
	if m == nil {
		return 0, string(stdout), fmt.Errorf("no percent in sget")
	}
	n, err := strconv.Atoi(string(m[1]))
	if err != nil {
		return 0, string(stdout), err
	}
	if n > 100 {
		n = 100
	}
	return n, string(stdout), nil
}

func (c *ALSAController) sgetSwitch(ctx context.Context, cardID, control string) (on bool, err error) {
	ctx, cancel := withTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	stdout, _, err := c.cfg.Runner.Run(ctx, c.cfg.AmixerPath,
		"-M", "-D", "hw:CARD="+cardID, "sget", control)
	if err != nil {
		return false, err
	}
	// Find last [on]/[off] that is a switch (amixer prints channel + switch).
	matches := switchRe.FindAllSubmatch(stdout, -1)
	if len(matches) == 0 {
		return false, fmt.Errorf("no switch in sget")
	}
	last := string(matches[len(matches)-1][1])
	return last == "on", nil
}

func (c *ALSAController) fillObserved(ctx context.Context, rep *Report) {
	if len(rep.Devices) == 0 {
		return
	}
	// Prefer first ALSA device for observed fill.
	var dev *DeviceCapability
	for i := range rep.Devices {
		if rep.Devices[i].Backend == BackendALSA {
			dev = &rep.Devices[i]
			break
		}
	}
	if dev == nil {
		return
	}
	if rep.Output == nil {
		rep.Output = &OutputObserved{DeviceUID: dev.DeviceUID}
	} else if rep.Output.DeviceUID == "" {
		rep.Output.DeviceUID = dev.DeviceUID
	}
	if rep.Input == nil {
		rep.Input = &InputObserved{DeviceUID: dev.DeviceUID}
	} else if rep.Input.DeviceUID == "" {
		rep.Input.DeviceUID = dev.DeviceUID
	}
	if dev.OutputControl != "" {
		if rep.Output.VolumePercent == nil {
			if pct, _, err := c.sgetPercent(ctx, dev.ALSACardID, dev.OutputControl); err == nil {
				rep.Output.VolumePercent = &pct
			}
		}
		if rep.Output.Muted == nil {
			if on, err := c.sgetSwitch(ctx, dev.ALSACardID, dev.OutputControl); err == nil {
				muted := !on
				rep.Output.Muted = &muted
			}
		}
	}
	if dev.CaptureControl != "" && rep.Input.GainPercent == nil {
		if pct, _, err := c.sgetPercent(ctx, dev.ALSACardID, dev.CaptureControl); err == nil {
			rep.Input.GainPercent = &pct
		}
	}
}

// sanitizeDetail redacts control chars and caps length for reports/logs.
func sanitizeDetail(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\t' || r == '\n' || r == ' ':
			b.WriteRune(' ')
		case unicode.IsPrint(r):
			b.WriteRune(r)
		default:
			b.WriteByte('?')
		}
		if b.Len() >= 256 {
			break
		}
	}
	return strings.TrimSpace(b.String())
}
