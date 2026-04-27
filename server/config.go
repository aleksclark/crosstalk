package crosstalk

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
)

// DefaultConfigPath is the default config file path used when neither --config
// flag nor CROSSTALK_CONFIG env var is set.
const DefaultConfigPath = "ct-server.json"

// Config holds all configuration for the CrossTalk server.
// Field defaults match the values declared in config.schema.json.
type Config struct {
	Listen        string       `json:"listen"`
	DBPath        string       `json:"db_path"`
	RecordingPath string       `json:"recording_path"`
	LogLevel      string       `json:"log_level"`
	WebRTC        WebRTCConfig `json:"webrtc"`
	Auth          AuthConfig   `json:"auth"`
	Web           WebConfig    `json:"web"`
}

// WebRTCConfig holds WebRTC-related settings.
type WebRTCConfig struct {
	STUNServers []string   `json:"stun_servers"`
	TURN        TURNConfig `json:"turn"`
	// PublicIP is the server's public IP advertised in ICE candidates.
	// Required on Fly.io and other NAT'd environments where the server's
	// local IP differs from its public IP.
	PublicIP string `json:"public_ip"`
	// UDPMuxPort, when set, enables a single-port UDP multiplexer for all
	// ICE traffic. Required on Fly.io where UDP must bind to a specific
	// address. On Fly.io set the bind host via FLY_UDP_BIND_HOST env var.
	UDPMuxPort int `json:"udp_mux_port"`
}

// TURNConfig holds TURN relay settings.
type TURNConfig struct {
	Enabled    bool   `json:"enabled"`
	Server     string `json:"server"`
	Username   string `json:"username"`
	Credential string `json:"credential"`
}

// AuthConfig holds authentication settings.
type AuthConfig struct {
	SessionSecret          string `json:"session_secret"`
	WebRTCTokenLifetime    string `json:"webrtc_token_lifetime"`
	BroadcastTokenLifetime string `json:"broadcast_token_lifetime"`
}

// WebConfig holds web UI settings.
type WebConfig struct {
	DevMode     bool   `json:"dev_mode"`
	DevProxyURL string `json:"dev_proxy_url"`
}

// DefaultConfig returns a Config populated with all schema defaults.
func DefaultConfig() Config {
	return Config{
		Listen:        ":8080",
		DBPath:        "./data/crosstalk.db",
		RecordingPath: "./data/recordings",
		LogLevel:      "info",
		WebRTC: WebRTCConfig{
			STUNServers: []string{"stun:stun.l.google.com:19302"},
			TURN: TURNConfig{
				Enabled:    false,
				Server:     "",
				Username:   "",
				Credential: "",
			},
		},
		Auth: AuthConfig{
			SessionSecret:          "",
			WebRTCTokenLifetime:    "24h",
			BroadcastTokenLifetime: "15m",
		},
		Web: WebConfig{
			DevMode:     false,
			DevProxyURL: "http://localhost:5173",
		},
	}
}

// LoadConfig reads a JSON config file from path, applies defaults for missing
// fields, and validates the result. Unknown top-level and nested keys produce
// warnings via slog. Type mismatches for known fields log warnings and keep
// defaults. Missing required fields (auth.session_secret) return an error.
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("reading config file: %w", err)
	}

	// Validate JSON syntax first.
	var syntaxCheck map[string]json.RawMessage
	if err := json.Unmarshal(data, &syntaxCheck); err != nil {
		return Config{}, fmt.Errorf("parsing config JSON: %w", err)
	}

	// Check for unknown fields.
	if err := warnUnknownFields(data); err != nil {
		return Config{}, fmt.Errorf("checking unknown fields: %w", err)
	}

	// Detect type mismatches, log warnings, and strip mismatched fields
	// so they don't silently zero out defaults.
	data = warnTypeMismatches(data)

	// Start from defaults so any field absent from the file keeps its default.
	cfg := DefaultConfig()

	// Decode into the typed struct (applies values over defaults).
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config JSON: %w", err)
	}

	// Validate required fields and enum values.
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// knownTopLevel is the set of recognised top-level JSON keys.
var knownTopLevel = []string{
	"$schema", "listen", "db_path", "recording_path", "log_level",
	"webrtc", "auth", "web",
}

// knownWebRTC is the set of recognised keys inside "webrtc".
var knownWebRTC = []string{"stun_servers", "turn"}

// knownTURN is the set of recognised keys inside "webrtc.turn".
var knownTURN = []string{"enabled", "server", "username", "credential"}

// knownAuth is the set of recognised keys inside "auth".
var knownAuth = []string{"session_secret", "webrtc_token_lifetime", "broadcast_token_lifetime"}

// knownWeb is the set of recognised keys inside "web".
var knownWeb = []string{"dev_mode", "dev_proxy_url"}

// warnUnknownFields decodes raw JSON into a generic map and logs warnings for
// any keys not present in the known field sets.
func warnUnknownFields(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	warnUnknownKeys("", raw, knownTopLevel)

	// Check nested objects.
	if webrtcRaw, ok := raw["webrtc"]; ok {
		var webrtc map[string]json.RawMessage
		if err := json.Unmarshal(webrtcRaw, &webrtc); err == nil {
			warnUnknownKeys("webrtc", webrtc, knownWebRTC)

			if turnRaw, ok := webrtc["turn"]; ok {
				var turn map[string]json.RawMessage
				if err := json.Unmarshal(turnRaw, &turn); err == nil {
					warnUnknownKeys("webrtc.turn", turn, knownTURN)
				}
			}
		}
	}

	if authRaw, ok := raw["auth"]; ok {
		var auth map[string]json.RawMessage
		if err := json.Unmarshal(authRaw, &auth); err == nil {
			warnUnknownKeys("auth", auth, knownAuth)
		}
	}

	if webRaw, ok := raw["web"]; ok {
		var web map[string]json.RawMessage
		if err := json.Unmarshal(webRaw, &web); err == nil {
			warnUnknownKeys("web", web, knownWeb)
		}
	}

	return nil
}

// warnUnknownKeys logs a warning for each key in raw that is not in known.
func warnUnknownKeys(prefix string, raw map[string]json.RawMessage, known []string) {
	for key := range raw {
		if !slices.Contains(known, key) {
			fullKey := key
			if prefix != "" {
				fullKey = prefix + "." + key
			}
			slog.Warn("unknown config field", "key", fullKey)
		}
	}
}

type jsonType string

const (
	jsonString jsonType = "string"
	jsonBool   jsonType = "bool"
	jsonObject jsonType = "object"
	jsonArray  jsonType = "array"
)

var fieldTypes = map[string]jsonType{
	"listen":                        jsonString,
	"db_path":                       jsonString,
	"recording_path":                jsonString,
	"log_level":                     jsonString,
	"webrtc":                        jsonObject,
	"auth":                          jsonObject,
	"web":                           jsonObject,
	"webrtc.stun_servers":           jsonArray,
	"webrtc.turn":                   jsonObject,
	"webrtc.turn.enabled":           jsonBool,
	"webrtc.turn.server":            jsonString,
	"webrtc.turn.username":          jsonString,
	"webrtc.turn.credential":        jsonString,
	"auth.session_secret":              jsonString,
	"auth.webrtc_token_lifetime":       jsonString,
	"auth.broadcast_token_lifetime":    jsonString,
	"web.dev_mode":                  jsonBool,
	"web.dev_proxy_url":             jsonString,
}

func detectJSONType(raw json.RawMessage) jsonType {
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 {
		return ""
	}
	switch raw[0] {
	case '"':
		return jsonString
	case 't', 'f':
		return jsonBool
	case '{':
		return jsonObject
	case '[':
		return jsonArray
	default:
		return "number"
	}
}

func warnTypeMismatches(data []byte) []byte {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return data
	}

	changed := false
	checkAndRemove := func(prefix string, m map[string]json.RawMessage) {
		for key, val := range m {
			fullKey := key
			if prefix != "" {
				fullKey = prefix + "." + key
			}
			expected, ok := fieldTypes[fullKey]
			if !ok {
				continue
			}
			actual := detectJSONType(val)
			if actual != "" && actual != expected {
				slog.Warn("config field type mismatch, using default",
					"key", fullKey,
					"expected", string(expected),
					"got", string(actual))
				delete(m, key)
				changed = true
			}
		}
	}

	checkAndRemove("", raw)

	if webrtcRaw, ok := raw["webrtc"]; ok {
		var webrtc map[string]json.RawMessage
		if err := json.Unmarshal(webrtcRaw, &webrtc); err == nil {
			checkAndRemove("webrtc", webrtc)
			if turnRaw, ok := webrtc["turn"]; ok {
				var turn map[string]json.RawMessage
				if err := json.Unmarshal(turnRaw, &turn); err == nil {
					checkAndRemove("webrtc.turn", turn)
					if changed {
						if b, err := json.Marshal(turn); err == nil {
							webrtc["turn"] = b
						}
					}
				}
			}
			if changed {
				if b, err := json.Marshal(webrtc); err == nil {
					raw["webrtc"] = b
				}
			}
		}
	}
	if authRaw, ok := raw["auth"]; ok {
		var auth map[string]json.RawMessage
		if err := json.Unmarshal(authRaw, &auth); err == nil {
			checkAndRemove("auth", auth)
			if changed {
				if b, err := json.Marshal(auth); err == nil {
					raw["auth"] = b
				}
			}
		}
	}
	if webRaw, ok := raw["web"]; ok {
		var web map[string]json.RawMessage
		if err := json.Unmarshal(webRaw, &web); err == nil {
			checkAndRemove("web", web)
			if changed {
				if b, err := json.Marshal(web); err == nil {
					raw["web"] = b
				}
			}
		}
	}

	if changed {
		if b, err := json.Marshal(raw); err == nil {
			return b
		}
	}
	return data
}

// validLogLevels enumerates the accepted values for log_level.
var validLogLevels = []string{"debug", "info", "warn", "error"}

// validate checks that required fields are present and enum values are valid.
func (c *Config) validate() error {
	if c.Auth.SessionSecret == "" {
		return &ConfigError{Field: "auth.session_secret", Message: "required field is missing or empty"}
	}

	if !slices.Contains(validLogLevels, c.LogLevel) {
		return &ConfigError{
			Field:   "log_level",
			Message: fmt.Sprintf("must be one of %s, got %q", strings.Join(validLogLevels, ", "), c.LogLevel),
		}
	}

	return nil
}

// ConfigError represents a configuration validation failure.
type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return "config: " + e.Field + ": " + e.Message
}

// ParseLogLevel converts a log_level config string to a [slog.Level].
func ParseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
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
