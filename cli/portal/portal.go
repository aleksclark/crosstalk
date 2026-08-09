// Package portal serves the captive-portal web interface used to provision the
// box's WiFi credentials from a phone. It depends only on the
// crosstalk.WiFiManager interface; the concrete NetworkManager implementation
// lives in the netcfg package and is wired in main.
//
// Single-radio note: the box's wifi chip cannot act as an access point and scan
// or join another network at the same time. So the portal never scans or
// connects while the hotspot is up: the network list is captured before the
// hotspot starts (injected via SetNetworks), and joining is performed by main
// after it tears the hotspot down. The portal's job is purely to show the
// cached list and capture the chosen SSID + passphrase.
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
	"sync"
	"time"

	crosstalk "github.com/aleksclark/crosstalk/cli"
)

//go:embed assets/*
var assets embed.FS

// Credentials are the WiFi parameters chosen by the operator in the portal.
type Credentials struct {
	SSID       string
	Passphrase string
}

// Server is the captive-portal HTTP server.
type Server struct {
	// Addr is the listen address (e.g. ":80").
	Addr string

	mu     sync.RWMutex
	cached []crosstalk.WiFiNetwork

	// provisioned is signalled with the chosen credentials once the operator
	// submits the connect form. Buffer size 1 + non-blocking send makes
	// duplicate submits idempotent.
	provisioned chan Credentials

	// accepted tracks whether credentials have already been accepted so
	// subsequent POSTs return the same connecting status without re-queueing.
	accepted sync.Once
	got      Credentials
}

// New creates a captive-portal server.
func New(addr string) *Server {
	return &Server{
		Addr:        addr,
		provisioned: make(chan Credentials, 1),
	}
}

// SetNetworks stores the network list to present in the portal. It is captured
// before the hotspot starts, because the single radio cannot scan while it is
// acting as an access point.
func (s *Server) SetNetworks(nets []crosstalk.WiFiNetwork) {
	s.mu.Lock()
	s.cached = nets
	s.mu.Unlock()
}

func (s *Server) networks() []crosstalk.WiFiNetwork {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cached
}

// Run starts the server and blocks until the context is cancelled or the
// operator submits credentials. On submission it returns the chosen
// credentials; the caller is responsible for tearing down the hotspot and
// performing the actual join.
func (s *Server) Run(ctx context.Context) (Credentials, error) {
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
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("portal: listening", "addr", s.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	var (
		creds  Credentials
		runErr error
	)
	select {
	case <-ctx.Done():
		runErr = ctx.Err()
	case creds = <-s.provisioned:
	case runErr = <-errCh:
	}

	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)

	return creds, runErr
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

// handleScan returns the network list captured before the hotspot started. The
// radio cannot rescan while hosting the AP, so this is a cached list; operators
// can always type a hidden/missing SSID by hand in the UI.
func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	nets := s.networks()
	if nets == nil {
		nets = []crosstalk.WiFiNetwork{}
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

	// Never log the passphrase. SSID alone is enough for diagnostics.
	slog.Info("portal: connect submitted", "ssid", req.SSID)

	// Hand the credentials to main exactly once. Duplicate submits (double-tap,
	// browser retry) are idempotent: they return the same connecting status
	// without re-queueing or overwriting the first choice.
	s.accepted.Do(func() {
		s.got = Credentials{SSID: req.SSID, Passphrase: req.Passphrase}
		select {
		case s.provisioned <- s.got:
		default:
		}
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "connecting",
		"ssid":   s.got.SSID,
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
