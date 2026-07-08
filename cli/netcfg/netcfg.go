// Package netcfg implements crosstalk.WiFiManager on top of nmcli
// (NetworkManager). It is the only package that shells out to nmcli; the rest
// of the code depends on the crosstalk.WiFiManager interface.
package netcfg

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	crosstalk "github.com/aleksclark/crosstalk/cli"
)

// hotspotConnName is the NetworkManager connection profile name used for the
// captive-portal access point.
const hotspotConnName = "ct-hotspot"

// dnsmasqSharedDir is where NetworkManager reads extra dnsmasq options for
// shared (hotspot) connections. Writing a wildcard address record here makes
// every DNS lookup resolve to the AP, which triggers the captive-portal
// prompt on phones.
const dnsmasqSharedDir = "/etc/NetworkManager/dnsmasq-shared.d"

// Manager wraps nmcli to satisfy crosstalk.WiFiManager.
type Manager struct {
	// Iface is the wireless interface to operate on (e.g. "wlan0"). Empty lets
	// NetworkManager pick the default wifi device.
	Iface string
	// APAddress is the IPv4 address the hotspot gateway listens on. Used for
	// the captive DNS redirect. Defaults to NetworkManager's shared address.
	APAddress string

	// run executes a command and returns combined stdout+stderr. Overridable
	// in tests.
	run func(ctx context.Context, name string, args ...string) ([]byte, error)
	// writeFile writes captive-portal dnsmasq config. Overridable in tests.
	writeFile func(path string, data []byte) error
}

// New returns a Manager that shells out to the real nmcli binary.
func New(iface string) *Manager {
	return &Manager{
		Iface:     iface,
		APAddress: "10.42.0.1",
		run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			cmd := exec.CommandContext(ctx, name, args...)
			var buf bytes.Buffer
			cmd.Stdout = &buf
			cmd.Stderr = &buf
			err := cmd.Run()
			return buf.Bytes(), err
		},
		writeFile: func(path string, data []byte) error {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			return os.WriteFile(path, data, 0o644)
		},
	}
}

var _ crosstalk.WiFiManager = (*Manager)(nil)

// Online reports whether NetworkManager considers the device fully connected.
func (m *Manager) Online(ctx context.Context) bool {
	out, err := m.run(ctx, "nmcli", "-t", "networking", "connectivity", "check")
	if err != nil {
		slog.Debug("netcfg: connectivity check failed", "error", err, "output", string(out))
		return false
	}
	return strings.TrimSpace(string(out)) == "full"
}

// Scan returns visible wireless networks sorted by descending signal strength.
func (m *Manager) Scan(ctx context.Context) ([]crosstalk.WiFiNetwork, error) {
	out, err := m.run(ctx, "nmcli", "-t", "-f", "IN-USE,SIGNAL,SECURITY,SSID",
		"device", "wifi", "list", "--rescan", "yes")
	if err != nil {
		return nil, fmt.Errorf("scanning wifi: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return parseScan(string(out)), nil
}

// parseScan parses terse `nmcli device wifi list` output. Terse mode escapes
// ':' and '\' with a backslash, so fields are split on unescaped colons.
func parseScan(out string) []crosstalk.WiFiNetwork {
	seen := make(map[string]int) // ssid -> index in result
	var nets []crosstalk.WiFiNetwork

	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := splitTerse(line)
		if len(fields) < 4 {
			continue
		}
		inUse := fields[0] == "*"
		signal, _ := strconv.Atoi(fields[1])
		security := strings.TrimSpace(fields[2])
		ssid := fields[3]
		if ssid == "" {
			continue // hidden network
		}
		secured := security != "" && security != "--"

		if idx, ok := seen[ssid]; ok {
			// Keep the strongest reading for a duplicated SSID.
			if signal > nets[idx].Signal {
				nets[idx].Signal = signal
			}
			nets[idx].Active = nets[idx].Active || inUse
			nets[idx].Secured = nets[idx].Secured || secured
			continue
		}
		seen[ssid] = len(nets)
		nets = append(nets, crosstalk.WiFiNetwork{
			SSID:    ssid,
			Signal:  signal,
			Secured: secured,
			Active:  inUse,
		})
	}

	sort.SliceStable(nets, func(i, j int) bool {
		return nets[i].Signal > nets[j].Signal
	})
	return nets
}

// splitTerse splits an nmcli terse-mode line on unescaped ':' separators and
// unescapes '\:' and '\\'.
func splitTerse(line string) []string {
	var fields []string
	var cur strings.Builder
	escaped := false
	for _, r := range line {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case r == ':':
			fields = append(fields, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	fields = append(fields, cur.String())
	return fields
}

// Connect joins a network, replacing any existing saved profile for that SSID.
func (m *Manager) Connect(ctx context.Context, ssid, passphrase string) error {
	if ssid == "" {
		return &crosstalk.ValidationError{Field: "ssid", Message: "required"}
	}

	// Remove a stale profile so a changed passphrase takes effect.
	_, _ = m.run(ctx, "nmcli", "connection", "delete", ssid)

	args := []string{"device", "wifi", "connect", ssid}
	if passphrase != "" {
		args = append(args, "password", passphrase)
	}
	if m.Iface != "" {
		args = append(args, "ifname", m.Iface)
	}

	out, err := m.run(ctx, "nmcli", args...)
	if err != nil {
		return fmt.Errorf("connecting to %q: %w: %s", ssid, err, strings.TrimSpace(string(out)))
	}
	slog.Info("netcfg: joined network", "ssid", ssid)
	return nil
}

// StartHotspot brings up a shared-mode access point and installs a captive DNS
// redirect so phones present the portal automatically.
func (m *Manager) StartHotspot(ctx context.Context, ssid, passphrase string) error {
	if ssid == "" {
		return &crosstalk.ValidationError{Field: "ssid", Message: "required"}
	}

	// Best-effort captive DNS redirect: resolve every name to the AP gateway.
	if m.writeFile != nil && m.APAddress != "" {
		conf := fmt.Sprintf("address=/#/%s\n", m.APAddress)
		if err := m.writeFile(filepath.Join(dnsmasqSharedDir, "ct-captive.conf"), []byte(conf)); err != nil {
			slog.Warn("netcfg: captive dns redirect not installed", "error", err)
		}
	}

	// Free the (typically single) wifi radio before switching it to AP mode.
	// If a station profile is stuck retrying a bad passphrase, NetworkManager
	// keeps the device busy and the hotspot activation fails. Disconnecting the
	// device drops that attempt so AP mode can claim the radio.
	dev := m.Iface
	if dev == "" {
		dev = m.wifiDevice(ctx)
	}
	if dev != "" {
		if out, err := m.run(ctx, "nmcli", "device", "disconnect", dev); err != nil {
			slog.Debug("netcfg: could not disconnect wifi before hotspot",
				"iface", dev, "error", err, "output", strings.TrimSpace(string(out)))
		} else {
			slog.Info("netcfg: disconnected wifi station before hotspot", "iface", dev)
		}
	}

	// Clear any previous hotspot profile so settings changes apply.
	_, _ = m.run(ctx, "nmcli", "connection", "delete", hotspotConnName)

	args := []string{"device", "wifi", "hotspot", "con-name", hotspotConnName, "ssid", ssid}
	if m.Iface != "" {
		args = append(args, "ifname", m.Iface)
	}
	if passphrase != "" {
		args = append(args, "password", passphrase)
	}

	out, err := m.run(ctx, "nmcli", args...)
	if err != nil {
		return fmt.Errorf("starting hotspot %q: %w: %s", ssid, err, strings.TrimSpace(string(out)))
	}
	slog.Info("netcfg: hotspot up", "ssid", ssid)
	return nil
}

// StopHotspot tears down the captive-portal access point.
func (m *Manager) StopHotspot(ctx context.Context) error {
	out, err := m.run(ctx, "nmcli", "connection", "down", hotspotConnName)
	if err != nil {
		return fmt.Errorf("stopping hotspot: %w: %s", err, strings.TrimSpace(string(out)))
	}
	slog.Info("netcfg: hotspot down")
	return nil
}

var _ crosstalk.EthernetManager = (*Manager)(nil)

// wiredDevice returns the first ethernet device NetworkManager knows about, or
// "" if there is none.
func (m *Manager) wiredDevice(ctx context.Context) string {
	return m.deviceOfType(ctx, "ethernet")
}

// wifiDevice returns the first wifi device NetworkManager knows about, or "" if
// there is none.
func (m *Manager) wifiDevice(ctx context.Context) string {
	return m.deviceOfType(ctx, "wifi")
}

// deviceOfType returns the first device of the given nmcli type, or "".
func (m *Manager) deviceOfType(ctx context.Context, typ string) string {
	out, err := m.run(ctx, "nmcli", "-t", "-f", "DEVICE,TYPE", "device", "status")
	if err != nil {
		slog.Debug("netcfg: device status failed", "error", err, "output", string(out))
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := splitTerse(strings.TrimRight(line, "\r"))
		if len(fields) < 2 {
			continue
		}
		if fields[1] == typ {
			return fields[0]
		}
	}
	return ""
}

// EnableDHCP brings up the wired ethernet interface as a DHCP client. If no
// wired device exists it returns ("", nil).
func (m *Manager) EnableDHCP(ctx context.Context) (string, error) {
	dev := m.wiredDevice(ctx)
	if dev == "" {
		slog.Info("netcfg: no wired ethernet device present")
		return "", nil
	}

	// If the wired device already has a DHCP lease (common when it is owned by
	// systemd-networkd rather than NetworkManager), it is already "up in DHCP
	// mode" — nothing to do, and we must not fight the other manager.
	if _, ip, up := m.EthernetStatus(ctx); up {
		slog.Info("netcfg: ethernet already has a lease", "iface", dev, "ipv4", ip)
		return dev, nil
	}

	// Otherwise hand the device to NetworkManager and let it drive DHCP. On
	// images where the device is strictly unmanaged by config this is a no-op
	// and `device connect` will fail; that is non-fatal (the caller logs it).
	if out, err := m.run(ctx, "nmcli", "device", "set", dev, "managed", "yes"); err != nil {
		slog.Debug("netcfg: could not set ethernet managed",
			"iface", dev, "error", err, "output", strings.TrimSpace(string(out)))
	}

	// `nmcli device connect` activates the device using an existing profile or
	// auto-creates a default one, which uses DHCP (ipv4.method auto) by
	// default. This is idempotent and safe to call when already connected.
	out, err := m.run(ctx, "nmcli", "device", "connect", dev)
	if err != nil {
		return dev, fmt.Errorf("bringing up ethernet %q: %w: %s", dev, err, strings.TrimSpace(string(out)))
	}
	slog.Info("netcfg: ethernet up (dhcp)", "iface", dev)
	return dev, nil
}

// EthernetStatus reports the wired interface state.
func (m *Manager) EthernetStatus(ctx context.Context) (iface, ipv4 string, up bool) {
	dev := m.wiredDevice(ctx)
	if dev == "" {
		return "", "", false
	}
	out, err := m.run(ctx, "nmcli", "-t", "-f", "IP4.ADDRESS", "device", "show", dev)
	if err != nil {
		return dev, "", false
	}
	// Output lines look like: IP4.ADDRESS[1]:192.168.0.42/24
	for _, line := range strings.Split(string(out), "\n") {
		fields := splitTerse(strings.TrimRight(line, "\r"))
		if len(fields) < 2 || !strings.HasPrefix(fields[0], "IP4.ADDRESS") {
			continue
		}
		addr := strings.TrimSpace(fields[1])
		if addr == "" || addr == "--" {
			continue
		}
		if i := strings.IndexByte(addr, '/'); i >= 0 {
			addr = addr[:i]
		}
		return dev, addr, true
	}
	return dev, "", false
}
