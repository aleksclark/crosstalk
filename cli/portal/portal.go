// Package portal serves the captive-portal web interface used to provision the
// box's WiFi credentials from a phone. It depends only on the
// crosstalk.WiFiManager interface; the concrete NetworkManager implementation
// lives in the netcfg package and is wired in main.
package portal

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	crosstalk "github.com/aleksclark/crosstalk/cli"
)

//go:embed assets/*
var assets embed.FS

// Server is the captive-portal HTTP server.
type Server struct {
	// WiFi controls scanning and joining networks.
	WiFi crosstalk.WiFiManager
	// Addr is the listen address (e.g. ":80").
	Addr string

	// connected is signalled with the joined SSID once provisioning succeeds.
	connected chan string
}

// New creates a captive-portal server.
func New(wifi crosstalk.WiFiManager, addr string) *Server {
	return &Server{
		WiFi:      wifi,
		Addr:      addr,
		connected: make(chan string, 1),
	}
}

// Run starts the server and blocks until the context is cancelled or the user
// successfully provisions WiFi. On success it returns the joined SSID.
func (s *Server) Run(ctx context.Context) (string, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/scan", s.handleScan)
	mux.HandleFunc("/connect", s.handleConnect)
	mux.Handle("/portal", http.RedirectHandler("/", http.StatusFound))
	// Root serves the UI; any other path is treated as an OS captive-portal
	// probe and redirected to the UI so the phone pops the sign-in sheet.
	mux.HandleFunc("/", s.handleRoot)

	srv := &http.Server{
		Addr:              s.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("portal: listening", "addr", s.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	var (
		ssid   string
		runErr error
	)
	select {
	case <-ctx.Done():
		runErr = ctx.Err()
	case ssid = <-s.connected:
	case runErr = <-errCh:
	}

	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)

	return ssid, runErr
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		// OS connectivity probe (e.g. /generate_204, /hotspot-detect.html):
		// redirect to the portal so the captive-portal UI appears.
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	page, err := assets.ReadFile("assets/index.html")
	if err != nil {
		http.Error(w, "portal unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(page)
}

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	nets, err := s.WiFi.Scan(ctx)
	if err != nil {
		slog.Warn("portal: scan failed", "error", err)
		writeJSON(w, http.StatusOK, map[string]any{"networks": []any{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"networks": nets})
}

type connectRequest struct {
	SSID       string `json:"ssid"`
	Passphrase string `json:"passphrase"`
}

func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return
	}

	var req connectRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
		return
	}
	if req.SSID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "SSID is required"})
		return
	}

	// The connect can take a while and will drop the hotspot link the phone is
	// on, so respond before the link is lost.
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	go func() {
		defer cancel()
		if err := s.WiFi.Connect(ctx, req.SSID, req.Passphrase); err != nil {
			slog.Warn("portal: connect failed", "ssid", req.SSID, "error", err)
			return
		}
		select {
		case s.connected <- req.SSID:
		default:
		}
	}()

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "connecting",
		"ssid":   req.SSID,
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// LocalIPv4 returns the first non-loopback IPv4 address, useful for logging the
// portal URL. Returns "" if none is found.
func LocalIPv4() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if ipNet, ok := a.(*net.IPNet); ok && ipNet.IP.To4() != nil {
				return ipNet.IP.String()
			}
		}
	}
	return ""
}

// ProbeURL returns the URL a phone should open to reach the portal.
func (s *Server) ProbeURL() string {
	ip := LocalIPv4()
	if ip == "" {
		return "http://<box-ip>/"
	}
	return fmt.Sprintf("http://%s/", ip)
}
