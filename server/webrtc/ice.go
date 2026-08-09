package webrtc

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/pion/webrtc/v4"
)

// supportedICESchemes are the ICE server URI schemes we accept.
var supportedICESchemes = map[string]struct{}{
	"stun":  {},
	"stuns": {},
	"turn":  {},
	"turns": {},
}

// supportedTURNSchemes are schemes valid for a TURN server URL.
var supportedTURNSchemes = map[string]struct{}{
	"turn":  {},
	"turns": {},
}

// ValidateICEConfig checks ICE configuration invariants.
//
// TURN fields are validated as a set: URL, username, and credential must be
// either all present or all absent. STUN and TURN URLs must parse and use a
// supported scheme. This function never logs credentials.
func ValidateICEConfig(cfg ICEConfig) error {
	for i, raw := range cfg.STUNServers {
		if err := validateICEURL(raw, supportedICESchemes); err != nil {
			return fmt.Errorf("webrtc: stun_servers[%d]: %w", i, err)
		}
	}
	return validateTURNSet(cfg.TURNServer, cfg.TURNUser, cfg.TURNCred)
}

// validateTURNSet enforces all-or-nothing TURN credentials and URL shape.
func validateTURNSet(server, user, cred string) error {
	server = strings.TrimSpace(server)
	user = strings.TrimSpace(user)
	// Credential may intentionally contain spaces; only treat fully empty as absent.
	hasServer := server != ""
	hasUser := user != ""
	hasCred := cred != ""

	present := 0
	if hasServer {
		present++
	}
	if hasUser {
		present++
	}
	if hasCred {
		present++
	}
	if present == 0 {
		return nil
	}
	if present != 3 {
		missing := make([]string, 0, 3)
		if !hasServer {
			missing = append(missing, "turn_server")
		}
		if !hasUser {
			missing = append(missing, "turn_username")
		}
		if !hasCred {
			missing = append(missing, "turn_credential")
		}
		return fmt.Errorf("webrtc: TURN config must be all present or all absent; missing %s", strings.Join(missing, ", "))
	}

	if err := validateICEURL(server, supportedTURNSchemes); err != nil {
		return fmt.Errorf("webrtc: turn_server: %w", err)
	}
	return nil
}

// validateICEURL parses an ICE server URI and checks its scheme.
func validateICEURL(raw string, allowed map[string]struct{}) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("empty URL")
	}
	// ICE URIs look like "stun:host:port" / "turn:host:port?transport=udp".
	// net/url handles these when the scheme is present.
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("malformed URL %q: %w", raw, err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme == "" {
		return fmt.Errorf("URL %q missing scheme", raw)
	}
	if _, ok := allowed[scheme]; !ok {
		return fmt.Errorf("unsupported scheme %q in URL %q", scheme, raw)
	}
	if u.Host == "" && u.Opaque == "" {
		// stun:host:port often lands in Opaque ("host:port") rather than Host.
		return fmt.Errorf("URL %q missing host", raw)
	}
	if u.Host == "" && u.Opaque != "" {
		// Opaque form is fine (e.g. stun:stun.l.google.com:19302).
		hostPort := u.Opaque
		if i := strings.IndexByte(hostPort, '?'); i >= 0 {
			hostPort = hostPort[:i]
		}
		if strings.TrimSpace(hostPort) == "" {
			return fmt.Errorf("URL %q missing host", raw)
		}
	}
	return nil
}

// BuildICEServers constructs the Pion ICE server list from cfg.
// On success, TURN credentials are present in the returned servers when
// configured. Credentials are never logged.
func BuildICEServers(cfg ICEConfig) ([]webrtc.ICEServer, error) {
	if err := ValidateICEConfig(cfg); err != nil {
		return nil, err
	}

	servers := make([]webrtc.ICEServer, 0, 2)
	if len(cfg.STUNServers) > 0 {
		// Copy to avoid retaining the caller's slice.
		urls := append([]string(nil), cfg.STUNServers...)
		servers = append(servers, webrtc.ICEServer{URLs: urls})
	}

	if strings.TrimSpace(cfg.TURNServer) != "" {
		servers = append(servers, webrtc.ICEServer{
			URLs:           []string{strings.TrimSpace(cfg.TURNServer)},
			Username:       strings.TrimSpace(cfg.TURNUser),
			Credential:     cfg.TURNCred,
			CredentialType: webrtc.ICECredentialTypePassword,
		})
	}
	return servers, nil
}

// BuildICEConfiguration builds a webrtc.Configuration from ICEConfig.
// This is the testable path proving TURN credentials reach Pion config
// without requiring a live PeerConnection.
func BuildICEConfiguration(cfg ICEConfig) (webrtc.Configuration, error) {
	servers, err := BuildICEServers(cfg)
	if err != nil {
		return webrtc.Configuration{}, err
	}
	return webrtc.Configuration{ICEServers: servers}, nil
}

// TURNConfigured reports whether a complete TURN set is present (after trim).
// Does not validate URL shape; use ValidateICEConfig for that.
func (cfg ICEConfig) TURNConfigured() bool {
	return strings.TrimSpace(cfg.TURNServer) != "" &&
		strings.TrimSpace(cfg.TURNUser) != "" &&
		cfg.TURNCred != ""
}
