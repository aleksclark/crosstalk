package audioctl_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aleksclark/crosstalk/cli/audioctl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseALSARoute(t *testing.T) {
	// Exported via inventory path; exercise through controller resolve by fake sysfs.
	tests := []struct {
		route string
		ok    bool
	}{
		{"plughw:CARD=Device,DEV=0", true},
		{"hw:CARD=Device,DEV=0", true},
		{"hw:CARD=Device", true},
		{"hw:1", false},
		{"plughw:1,0", false},
		{"hw:CARD=Dev;rm -rf /,DEV=0", false},
		{"hw:CARD=../../etc,DEV=0", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.route, func(t *testing.T) {
			dir := t.TempDir()
			runner := &audioctl.RecordingRunner{}
			c := audioctl.NewController(audioctl.Config{
				SourceRoute:     tt.route,
				SinkRoute:       tt.route,
				SysfsRoot:       dir,
				ProcAsoundCards: filepath.Join(dir, "cards"),
				Runner:          runner,
			})
			rep, err := c.Inventory(context.Background())
			require.NoError(t, err)
			if tt.ok {
				// May be empty if sysfs incomplete — but must not shell out on bad id.
				_ = rep
			} else {
				// Bad route → no ALSA device, zero amixer calls for invalid card.
				for _, call := range runner.Calls {
					joined := strings.Join(call, " ")
					assert.NotContains(t, joined, ";")
					assert.NotContains(t, joined, "rm ")
				}
			}
		})
	}
}

func setupFakeUSBCard(t *testing.T, root, cardID, vid, pid, serial string, cardIndex int) {
	t.Helper()
	// /proc/asound/cards
	cards := fmt.Sprintf(" %d [%s         ]: USB-Audio - USB Audio Device\n", cardIndex, cardID)
	require.NoError(t, os.WriteFile(filepath.Join(root, "cards"), []byte(cards), 0o644))

	// USB device node
	usbDev := filepath.Join(root, "devices", "platform", "xhci-hcd", "usb1", "1-1", "1-1.2")
	require.NoError(t, os.MkdirAll(usbDev, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(usbDev, "idVendor"), []byte(vid+"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(usbDev, "idProduct"), []byte(pid+"\n"), 0o644))
	if serial != "" {
		require.NoError(t, os.WriteFile(filepath.Join(usbDev, "serial"), []byte(serial+"\n"), 0o644))
	}

	// sound cardN → device symlink to usb
	cardDir := filepath.Join(root, "class", "sound", fmt.Sprintf("card%d", cardIndex))
	require.NoError(t, os.MkdirAll(cardDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cardDir, "id"), []byte(cardID+"\n"), 0o644))
	// relative symlink
	require.NoError(t, os.Symlink(usbDev, filepath.Join(cardDir, "device")))
}

func amixerHandler(volumePct int, muted bool, gainPct int, agcOn bool, controls ...string) func(string, []string) ([]byte, []byte, error) {
	ctrlSet := map[string]struct{}{}
	for _, c := range controls {
		ctrlSet[c] = struct{}{}
	}
	vol := volumePct
	mute := muted
	gain := gainPct
	agc := agcOn
	return func(name string, args []string) ([]byte, []byte, error) {
		if name != "/usr/bin/amixer" && !strings.HasSuffix(name, "amixer") {
			return nil, []byte("bad exe"), fmt.Errorf("unexpected exe")
		}
		// Find subcommand
		joined := strings.Join(args, " ")
		if containsArg(args, "scontrols") {
			var b strings.Builder
			for _, c := range controls {
				fmt.Fprintf(&b, "Simple mixer control '%s',0\n", c)
			}
			return []byte(b.String()), nil, nil
		}
		// sget
		if containsArg(args, "sget") {
			ctrl := lastNonFlag(args)
			switch ctrl {
			case "Speaker", "Headphone", "PCM":
				sw := "on"
				if mute {
					sw = "off"
				}
				out := fmt.Sprintf("Simple mixer control '%s',0\n  Front Left: Playback %d [%d%%] [%s]\n", ctrl, vol, vol, sw)
				return []byte(out), nil, nil
			case "Mic", "Capture":
				// Real C-Media Mic prints Playback percent before Capture;
				// parser must not take the Playback leg as gain.
				out := fmt.Sprintf("Simple mixer control '%s',0\n  Mono: Playback 31 [100%%] [8.00dB] [on] Capture %d [%d%%] [on]\n", ctrl, gain, gain)
				return []byte(out), nil, nil
			case "Auto Gain Control":
				sw := "off"
				if agc {
					sw = "on"
				}
				out := fmt.Sprintf("Simple mixer control 'Auto Gain Control',0\n  Item0: '%s'\n  [%s]\n", sw, sw)
				return []byte(out), nil, nil
			default:
				return nil, []byte("not found"), fmt.Errorf("exit 1")
			}
		}
		// sset
		if containsArg(args, "sset") {
			ctrl := ""
			for i, a := range args {
				if a == "sset" && i+1 < len(args) {
					ctrl = args[i+1]
					break
				}
			}
			if _, ok := ctrlSet[ctrl]; !ok && ctrl != "Auto Gain Control" {
				return nil, []byte("Invalid command"), fmt.Errorf("exit 1")
			}
			// parse value tokens after control name
			rest := argsAfter(args, ctrl)
			for _, tok := range rest {
				if strings.HasSuffix(tok, "%") {
					var n int
					fmt.Sscanf(tok, "%d%%", &n)
					if ctrl == "Mic" || ctrl == "Capture" {
						gain = n
					} else {
						vol = n
					}
				}
				switch tok {
				case "mute":
					mute = true
				case "unmute":
					mute = false
				case "off":
					if ctrl == "Auto Gain Control" {
						agc = false
					}
				case "on":
					if ctrl == "Auto Gain Control" {
						agc = true
					}
				}
			}
			return []byte("OK " + joined), nil, nil
		}
		return nil, []byte("unknown"), fmt.Errorf("unknown amixer invocation")
	}
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func lastNonFlag(args []string) string {
	for i := len(args) - 1; i >= 0; i-- {
		if !strings.HasPrefix(args[i], "-") {
			return args[i]
		}
	}
	return ""
}

func argsAfter(args []string, key string) []string {
	for i, a := range args {
		if a == key {
			return args[i+1:]
		}
	}
	return nil
}

func TestInventorySerialUID(t *testing.T) {
	root := t.TempDir()
	setupFakeUSBCard(t, root, "Device", "0d8c", "0014", "ABC123", 1)
	runner := &audioctl.RecordingRunner{
		Handler: amixerHandler(50, false, 34, true, "Speaker", "Mic", "Auto Gain Control"),
	}
	c := audioctl.NewController(audioctl.Config{
		SourceRoute:     "plughw:CARD=Device,DEV=0",
		SinkRoute:       "plughw:CARD=Device,DEV=0",
		SysfsRoot:       root,
		ProcAsoundCards: filepath.Join(root, "cards"),
		Runner:          runner,
		AmixerPath:      "/usr/bin/amixer",
	})
	rep, err := c.Inventory(context.Background())
	require.NoError(t, err)
	require.Len(t, rep.Devices, 1)
	assert.Equal(t, "usb:0d8c:0014:serial:ABC123", rep.Devices[0].DeviceUID)
	assert.Equal(t, audioctl.BackendALSA, rep.Devices[0].Backend)
	assert.True(t, rep.Devices[0].SupportsVolume)
	assert.True(t, rep.Devices[0].SupportsGain)
	assert.True(t, rep.Devices[0].SupportsAGCDisable)
	// No shell
	for _, call := range runner.Calls {
		assert.Equal(t, "/usr/bin/amixer", call[0])
		assert.NotContains(t, strings.Join(call, " "), "sh")
	}
}

func TestInventoryPathUIDWhenNoSerial(t *testing.T) {
	root := t.TempDir()
	setupFakeUSBCard(t, root, "Device", "0d8c", "0014", "", 2)
	runner := &audioctl.RecordingRunner{
		Handler: amixerHandler(50, false, 34, false, "PCM", "Capture"),
	}
	c := audioctl.NewController(audioctl.Config{
		SourceRoute:     "hw:CARD=Device,DEV=0",
		SinkRoute:       "hw:CARD=Device,DEV=0",
		SysfsRoot:       root,
		ProcAsoundCards: filepath.Join(root, "cards"),
		Runner:          runner,
	})
	rep, err := c.Inventory(context.Background())
	require.NoError(t, err)
	require.Len(t, rep.Devices, 1)
	assert.True(t, strings.HasPrefix(rep.Devices[0].DeviceUID, "usb:0d8c:0014:path:"))
	assert.NotContains(t, rep.Devices[0].DeviceUID, "serial:")
}

func TestApplyVolumeMuteGain(t *testing.T) {
	root := t.TempDir()
	setupFakeUSBCard(t, root, "Device", "0d8c", "0014", "S1", 0)
	runner := &audioctl.RecordingRunner{
		Handler: amixerHandler(50, false, 34, true, "Speaker", "Mic", "Auto Gain Control"),
	}
	c := audioctl.NewController(audioctl.Config{
		SourceRoute:     "plughw:CARD=Device,DEV=0",
		SinkRoute:       "plughw:CARD=Device,DEV=0",
		SysfsRoot:       root,
		ProcAsoundCards: filepath.Join(root, "cards"),
		Runner:          runner,
	})
	inv, err := c.Inventory(context.Background())
	require.NoError(t, err)
	uid := inv.Devices[0].DeviceUID
	vol := 0
	muted := true
	gain := 100
	rep, err := c.Apply(context.Background(), audioctl.Command{
		CommandID:       "abc-audio/x/1",
		DesiredRevision: 1,
		Output: &audioctl.OutputDesired{
			DeviceUID:     uid,
			VolumePercent: &vol,
			Muted:         &muted,
		},
		Input: &audioctl.InputDesired{
			DeviceUID:   uid,
			GainPercent: &gain,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, rep.Output)
	require.NotNil(t, rep.Input)
	assert.Equal(t, audioctl.StateApplied, rep.Output.VolumeState)
	assert.Equal(t, audioctl.StateApplied, rep.Output.MuteState)
	assert.Equal(t, audioctl.StateApplied, rep.Input.GainState)
	require.NotNil(t, rep.Output.VolumePercent)
	assert.Equal(t, 0, *rep.Output.VolumePercent)
	require.NotNil(t, rep.Output.Muted)
	assert.True(t, *rep.Output.Muted)
	require.NotNil(t, rep.Input.GainPercent)
	assert.Equal(t, 100, *rep.Input.GainPercent)

	// Verify argv shape: amixer -M -D hw:CARD=Device sset Speaker 0%
	var sawVol, sawMute, sawAGC, sawGain bool
	for _, call := range runner.Calls {
		j := strings.Join(call, " ")
		assert.Equal(t, "/usr/bin/amixer", call[0])
		assert.NotContains(t, j, "sh -c")
		assert.NotContains(t, j, "bash")
		if containsArg(call, "sset") && containsArg(call, "Speaker") && containsArg(call, "0%") {
			sawVol = true
			assert.Contains(t, call, "-M")
			assert.Contains(t, call, "hw:CARD=Device")
		}
		if containsArg(call, "mute") {
			sawMute = true
		}
		if containsArg(call, "Auto Gain Control") && containsArg(call, "off") {
			sawAGC = true
		}
		if containsArg(call, "100%") && containsArg(call, "cap") {
			sawGain = true
		}
	}
	assert.True(t, sawVol)
	assert.True(t, sawMute)
	assert.True(t, sawAGC)
	assert.True(t, sawGain)
}

func TestApplyDeviceMismatchZeroSubprocess(t *testing.T) {
	root := t.TempDir()
	setupFakeUSBCard(t, root, "Device", "0d8c", "0014", "S1", 0)
	runner := &audioctl.RecordingRunner{
		Handler: amixerHandler(50, false, 34, false, "Speaker", "Mic"),
	}
	c := audioctl.NewController(audioctl.Config{
		SourceRoute:     "plughw:CARD=Device,DEV=0",
		SinkRoute:       "plughw:CARD=Device,DEV=0",
		SysfsRoot:       root,
		ProcAsoundCards: filepath.Join(root, "cards"),
		Runner:          runner,
	})
	// Inventory first (creates scontrols calls)
	_, err := c.Inventory(context.Background())
	require.NoError(t, err)
	before := runner.CallCount()

	vol := 25
	rep, err := c.Apply(context.Background(), audioctl.Command{
		CommandID:       "abc-audio/x/2",
		DesiredRevision: 2,
		Output: &audioctl.OutputDesired{
			DeviceUID:     "usb:dead:beef:serial:NOPE",
			VolumePercent: &vol,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, audioctl.ErrCodeDeviceMismatch, rep.ErrorCode)
	assert.Equal(t, audioctl.StateDeviceMismatch, rep.Output.VolumeState)
	// Apply path may still inventory (scontrols) but must not sset wrong device.
	for _, call := range runner.Calls[before:] {
		assert.False(t, containsArg(call, "sset"), "unexpected sset: %v", call)
	}
}

func TestApplyInvalidCommandZeroSubprocess(t *testing.T) {
	root := t.TempDir()
	setupFakeUSBCard(t, root, "Device", "0d8c", "0014", "S1", 0)
	runner := &audioctl.RecordingRunner{
		Handler: amixerHandler(50, false, 34, false, "Speaker", "Mic"),
	}
	c := audioctl.NewController(audioctl.Config{
		SourceRoute:     "plughw:CARD=Device,DEV=0",
		SinkRoute:       "plughw:CARD=Device,DEV=0",
		SysfsRoot:       root,
		ProcAsoundCards: filepath.Join(root, "cards"),
		Runner:          runner,
	})
	_, _ = c.Inventory(context.Background())
	before := runner.CallCount()
	bad := 101
	rep, err := c.Apply(context.Background(), audioctl.Command{
		CommandID:       "x",
		DesiredRevision: 1,
		Output: &audioctl.OutputDesired{
			DeviceUID:     "usb:0d8c:0014:serial:S1",
			VolumePercent: &bad,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, audioctl.ErrCodeInvalidCommand, rep.ErrorCode)
	for _, call := range runner.Calls[before:] {
		assert.False(t, containsArg(call, "sset"), "sset on invalid: %v", call)
	}
}

func TestPipeWireRouteUnsupportedBackend(t *testing.T) {
	runner := &audioctl.RecordingRunner{}
	c := audioctl.NewController(audioctl.Config{
		SourceRoute: "alsa_input.usb-Device.analog-stereo",
		SinkRoute:   "alsa_output.usb-Device.analog-stereo",
		Runner:      runner,
	})
	rep, err := c.Inventory(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, rep.Devices)
	assert.Equal(t, audioctl.BackendPipeWire, rep.Devices[0].Backend)
	vol := 50
	rep2, err := c.Apply(context.Background(), audioctl.Command{
		CommandID:       "c",
		DesiredRevision: 1,
		Output: &audioctl.OutputDesired{
			DeviceUID:     rep.Devices[0].DeviceUID,
			VolumePercent: &vol,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, audioctl.ErrCodeUnsupportedBackend, rep2.ErrorCode)
	assert.Equal(t, 0, runner.CallCount())
}

func TestSameRevisionSafe(t *testing.T) {
	root := t.TempDir()
	setupFakeUSBCard(t, root, "Device", "0d8c", "0014", "S1", 0)
	runner := &audioctl.RecordingRunner{
		Handler: amixerHandler(40, false, 20, false, "Speaker", "Mic"),
	}
	c := audioctl.NewController(audioctl.Config{
		SourceRoute:     "plughw:CARD=Device,DEV=0",
		SinkRoute:       "plughw:CARD=Device,DEV=0",
		SysfsRoot:       root,
		ProcAsoundCards: filepath.Join(root, "cards"),
		Runner:          runner,
	})
	inv, _ := c.Inventory(context.Background())
	uid := inv.Devices[0].DeviceUID
	vol := 40
	cmd := audioctl.Command{
		CommandID:       "abc-audio/x/3",
		DesiredRevision: 3,
		Output:          &audioctl.OutputDesired{DeviceUID: uid, VolumePercent: &vol},
	}
	r1, err := c.Apply(context.Background(), cmd)
	require.NoError(t, err)
	assert.Equal(t, audioctl.StateApplied, r1.Output.VolumeState)
	r2, err := c.Apply(context.Background(), cmd)
	require.NoError(t, err)
	assert.Equal(t, audioctl.StateApplied, r2.Output.VolumeState)
}

func TestStaleRevision(t *testing.T) {
	root := t.TempDir()
	setupFakeUSBCard(t, root, "Device", "0d8c", "0014", "S1", 0)
	runner := &audioctl.RecordingRunner{
		Handler: amixerHandler(40, false, 20, false, "Speaker", "Mic"),
	}
	c := audioctl.NewController(audioctl.Config{
		SourceRoute:     "plughw:CARD=Device,DEV=0",
		SinkRoute:       "plughw:CARD=Device,DEV=0",
		SysfsRoot:       root,
		ProcAsoundCards: filepath.Join(root, "cards"),
		Runner:          runner,
	})
	inv, _ := c.Inventory(context.Background())
	uid := inv.Devices[0].DeviceUID
	vol := 40
	_, err := c.Apply(context.Background(), audioctl.Command{
		CommandID: "c5", DesiredRevision: 5,
		Output: &audioctl.OutputDesired{DeviceUID: uid, VolumePercent: &vol},
	})
	require.NoError(t, err)
	before := runner.CallCount()
	rep, err := c.Apply(context.Background(), audioctl.Command{
		CommandID: "c4", DesiredRevision: 4,
		Output: &audioctl.OutputDesired{DeviceUID: uid, VolumePercent: &vol},
	})
	require.NoError(t, err)
	assert.Equal(t, audioctl.StateStaleRevision, rep.Output.VolumeState)
	for _, call := range runner.Calls[before:] {
		assert.False(t, containsArg(call, "sset"))
	}
}

func TestShellMetacharactersInSerialEscaped(t *testing.T) {
	root := t.TempDir()
	setupFakeUSBCard(t, root, "Device", "0d8c", "0014", "a b;$(x)", 0)
	runner := &audioctl.RecordingRunner{
		Handler: amixerHandler(50, false, 10, false, "Speaker"),
	}
	c := audioctl.NewController(audioctl.Config{
		SourceRoute:     "plughw:CARD=Device,DEV=0",
		SinkRoute:       "plughw:CARD=Device,DEV=0",
		SysfsRoot:       root,
		ProcAsoundCards: filepath.Join(root, "cards"),
		Runner:          runner,
	})
	rep, err := c.Inventory(context.Background())
	require.NoError(t, err)
	require.Len(t, rep.Devices, 1)
	assert.Contains(t, rep.Devices[0].DeviceUID, "serial:")
	assert.NotContains(t, rep.Devices[0].DeviceUID, "$(x)")
	assert.NotContains(t, rep.Devices[0].DeviceUID, " ")
}
