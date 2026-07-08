package netcfg

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseScan(t *testing.T) {
	// IN-USE:SIGNAL:SECURITY:SSID, terse output with an escaped colon in an SSID.
	out := strings.Join([]string{
		"*:82:WPA2:HomeNet",
		" :47:WPA2:Cafe\\: Wifi",
		" :30:--:OpenGuest",
		" :90:WPA2:HomeNet", // duplicate SSID, stronger signal
		" :10::",            // hidden network, skipped
		"",
	}, "\n")

	nets := parseScan(out)
	require.Len(t, nets, 3)

	// Sorted by descending signal; HomeNet keeps the stronger 90 reading.
	assert.Equal(t, "HomeNet", nets[0].SSID)
	assert.Equal(t, 90, nets[0].Signal)
	assert.True(t, nets[0].Secured)
	assert.True(t, nets[0].Active)

	assert.Equal(t, "Cafe: Wifi", nets[1].SSID)
	assert.True(t, nets[1].Secured)

	assert.Equal(t, "OpenGuest", nets[2].SSID)
	assert.False(t, nets[2].Secured)
}

func TestOnline(t *testing.T) {
	m := New("")
	m.run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte("full\n"), nil
	}
	assert.True(t, m.Online(context.Background()))

	m.run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte("none\n"), nil
	}
	assert.False(t, m.Online(context.Background()))
}

func TestConnectBuildsArgs(t *testing.T) {
	var calls [][]string
	m := New("wlan0")
	m.run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		return nil, nil
	}

	require.NoError(t, m.Connect(context.Background(), "HomeNet", "secret123"))

	// First call deletes any stale profile, second connects.
	require.Len(t, calls, 2)
	assert.Equal(t, []string{"nmcli", "connection", "delete", "HomeNet"}, calls[0])
	assert.Equal(t, []string{
		"nmcli", "device", "wifi", "connect", "HomeNet",
		"password", "secret123", "ifname", "wlan0",
	}, calls[1])
}

func TestConnectOpenNetworkOmitsPassword(t *testing.T) {
	var connectArgs []string
	m := New("")
	m.run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "device" {
			connectArgs = args
		}
		return nil, nil
	}

	require.NoError(t, m.Connect(context.Background(), "OpenGuest", ""))
	assert.NotContains(t, connectArgs, "password")
	assert.NotContains(t, connectArgs, "ifname")
}

func TestConnectRequiresSSID(t *testing.T) {
	m := New("")
	err := m.Connect(context.Background(), "", "pw")
	require.Error(t, err)
}

func TestStartHotspotWritesCaptiveDNSAndBuildsArgs(t *testing.T) {
	var calls [][]string
	var written = map[string]string{}
	m := New("wlan0")
	m.APAddress = "10.42.0.1"
	m.run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		return nil, nil
	}
	m.writeFile = func(path string, data []byte) error {
		written[path] = string(data)
		return nil
	}

	require.NoError(t, m.StartHotspot(context.Background(), "CrossTalk-Setup", "crosstalk"))

	// Captive DNS redirect written.
	found := false
	for _, v := range written {
		if strings.Contains(v, "address=/#/10.42.0.1") {
			found = true
		}
	}
	assert.True(t, found, "expected captive dns redirect to be written")

	// The single wifi radio is freed before switching to AP mode: a
	// "device disconnect wlan0" must precede the hotspot bring-up.
	discIdx, hotspotIdx := -1, -1
	for i, c := range calls {
		if hasArg(c, "disconnect") && hasArg(c, "wlan0") {
			discIdx = i
		}
		if hasArg(c, "hotspot") {
			hotspotIdx = i
		}
	}
	assert.GreaterOrEqual(t, discIdx, 0, "expected wifi disconnect before hotspot")
	assert.Less(t, discIdx, hotspotIdx, "disconnect must come before hotspot")

	// Last call is the hotspot bring-up with all params.
	last := calls[len(calls)-1]
	assert.Equal(t, "nmcli", last[0])
	assert.Contains(t, last, "hotspot")
	assert.Contains(t, last, "CrossTalk-Setup")
	assert.Contains(t, last, "crosstalk")
	assert.Contains(t, last, "wlan0")
	assert.Contains(t, last, hotspotConnName)
}

func TestStopHotspot(t *testing.T) {
	var got []string
	m := New("")
	m.run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		got = append([]string{name}, args...)
		return nil, nil
	}
	require.NoError(t, m.StopHotspot(context.Background()))
	assert.Equal(t, []string{"nmcli", "connection", "down", hotspotConnName}, got)
}

func TestEnableDHCPBringsUpWiredDevice(t *testing.T) {
	var connectArgs []string
	m := New("")
	m.run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if hasArg(args, "status") {
			return []byte("wlan0:wifi\neth0:ethernet\nlo:loopback\n"), nil
		}
		if hasArg(args, "connect") {
			connectArgs = args
		}
		return nil, nil
	}

	iface, err := m.EnableDHCP(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "eth0", iface)
	assert.Equal(t, []string{"device", "connect", "eth0"}, connectArgs)
}

func TestEnableDHCPShortCircuitsWhenLeaseExists(t *testing.T) {
	var connectCalled bool
	m := New("")
	m.run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if hasArg(args, "status") {
			return []byte("eth0:ethernet\n"), nil
		}
		if hasArg(args, "show") {
			return []byte("IP4.ADDRESS[1]:192.168.0.43/20\n"), nil
		}
		if hasArg(args, "connect") {
			connectCalled = true
		}
		return nil, nil
	}

	iface, err := m.EnableDHCP(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "eth0", iface)
	assert.False(t, connectCalled, "must not fight an existing lease (e.g. networkd-owned)")
}

func TestEnableDHCPNoWiredDevice(t *testing.T) {
	m := New("")
	m.run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte("wlan0:wifi\nlo:loopback\n"), nil
	}
	iface, err := m.EnableDHCP(context.Background())
	require.NoError(t, err)
	assert.Empty(t, iface)
}

func TestEthernetStatusParsesIP(t *testing.T) {
	m := New("")
	m.run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if hasArg(args, "status") {
			return []byte("eth0:ethernet\n"), nil
		}
		if hasArg(args, "show") {
			return []byte("IP4.ADDRESS[1]:192.168.0.42/24\nIP4.GATEWAY:192.168.0.1\n"), nil
		}
		return nil, nil
	}
	iface, ip, up := m.EthernetStatus(context.Background())
	assert.True(t, up)
	assert.Equal(t, "eth0", iface)
	assert.Equal(t, "192.168.0.42", ip)
}

func TestEthernetStatusNoLease(t *testing.T) {
	m := New("")
	m.run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if hasArg(args, "status") {
			return []byte("eth0:ethernet\n"), nil
		}
		return []byte("IP4.ADDRESS:--\n"), nil
	}
	iface, ip, up := m.EthernetStatus(context.Background())
	assert.False(t, up)
	assert.Equal(t, "eth0", iface)
	assert.Empty(t, ip)
}

func hasArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}
