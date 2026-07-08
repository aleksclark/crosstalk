// ct-netcfg provisions the box's WiFi from a phone. On start it checks whether
// the box already has connectivity; if not, it brings up a WiFi hotspot and a
// captive-portal web interface so an operator can select a network and enter
// its password. Once the box successfully joins the chosen network the hotspot
// is torn down and the process exits so the main application service can start.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aleksclark/crosstalk/cli/netcfg"
	"github.com/aleksclark/crosstalk/cli/portal"
)

func main() {
	var (
		iface         = flag.String("iface", envOr("CT_NETCFG_IFACE", ""), "wireless interface (blank = NetworkManager default)")
		hotspotSSID   = flag.String("hotspot-ssid", envOr("CT_NETCFG_HOTSPOT_SSID", "CrossTalk-Setup"), "captive-portal hotspot SSID")
		hotspotPass   = flag.String("hotspot-pass", envOr("CT_NETCFG_HOTSPOT_PASS", "crosstalk"), "captive-portal hotspot password (8+ chars, blank = open)")
		portalAddr    = flag.String("portal-addr", envOr("CT_NETCFG_PORTAL_ADDR", ":80"), "captive-portal listen address")
		onlineTimeout = flag.Duration("online-timeout", durOr("CT_NETCFG_ONLINE_TIMEOUT", 45*time.Second), "how long to wait for existing WiFi before starting the hotspot")
		logLevel      = flag.String("log-level", envOr("CT_NETCFG_LOG_LEVEL", "info"), "log level: debug, info, warn, error")
	)
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: parseLevel(*logLevel),
	})))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	nm := netcfg.New(*iface)

	if waitOnline(ctx, nm, *onlineTimeout) {
		slog.Info("netcfg: already online, nothing to provision")
		return
	}
	if ctx.Err() != nil {
		return
	}

	slog.Info("netcfg: no connectivity, starting captive portal", "hotspot", *hotspotSSID)

	// Wired fallback: bring up ethernet in DHCP mode so an operator has a wired
	// management/connectivity path in addition to the captive portal. Best
	// effort — a box may have no wired device or no cable plugged in.
	if iface, err := nm.EnableDHCP(ctx); err != nil {
		slog.Warn("netcfg: ethernet dhcp bring-up failed", "iface", iface, "error", err)
	} else if iface != "" {
		if dev, ip, up := nm.EthernetStatus(ctx); up {
			slog.Info("netcfg: ethernet online (dhcp)", "iface", dev, "ipv4", ip,
				"portal", fmt.Sprintf("http://%s/", ip))
		} else {
			slog.Info("netcfg: ethernet enabled, waiting for lease/carrier", "iface", iface)
		}
	}

	if err := nm.StartHotspot(ctx, *hotspotSSID, *hotspotPass); err != nil {
		slog.Error("netcfg: failed to start hotspot", "error", err)
		os.Exit(1)
	}
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := nm.StopHotspot(shutCtx); err != nil {
			slog.Warn("netcfg: failed to stop hotspot", "error", err)
		}
	}()

	srv := portal.New(nm, *portalAddr)
	slog.Info("netcfg: captive portal ready",
		"join_ssid", *hotspotSSID, "open", srv.ProbeURL())

	ssid, err := srv.Run(ctx)
	if err != nil && ctx.Err() == nil {
		slog.Error("netcfg: portal error", "error", err)
		os.Exit(1)
	}
	if ssid == "" {
		slog.Info("netcfg: portal exited without provisioning")
		return
	}

	slog.Info("netcfg: joined network, waiting for connectivity", "ssid", ssid)
	confirmCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if waitOnline(confirmCtx, nm, 30*time.Second) {
		slog.Info("netcfg: provisioning complete, online", "ssid", ssid)
	} else {
		slog.Warn("netcfg: joined network but connectivity not confirmed", "ssid", ssid)
	}
}

// waitOnline polls until the box reports connectivity or the timeout/ctx ends.
func waitOnline(ctx context.Context, nm interface {
	Online(context.Context) bool
}, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	if nm.Online(ctx) {
		return true
	}
	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			if nm.Online(ctx) {
				return true
			}
			if time.Now().After(deadline) {
				return false
			}
		}
	}
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func durOr(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
