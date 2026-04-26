package display

import (
	"net"
	"strings"
)

// ProbeNetwork detects the primary network interface and IP address.
func ProbeNetwork() (iface, addr string, up bool) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", "", false
	}

	for _, priority := range []string{"eth", "wlan", "en"} {
		for _, ifc := range interfaces {
			if !strings.HasPrefix(ifc.Name, priority) {
				continue
			}
			if ifc.Flags&net.FlagUp == 0 {
				continue
			}
			addrs, err := ifc.Addrs()
			if err != nil || len(addrs) == 0 {
				continue
			}
			for _, a := range addrs {
				ipNet, ok := a.(*net.IPNet)
				if !ok {
					continue
				}
				if ipNet.IP.To4() != nil {
					return ifc.Name, ipNet.IP.String(), true
				}
			}
		}
	}

	for _, ifc := range interfaces {
		if ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		if ifc.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			if ipNet.IP.To4() != nil {
				return ifc.Name, ipNet.IP.String(), true
			}
		}
	}

	return "", "", false
}
