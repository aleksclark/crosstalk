// ct-abc is the Audio Booth Connector client binary.
// It connects to the CrossTalk server via WebSocket signaling, sends a Hello
// with client_type="abc", handles SessionAssignment and RestartCommand messages,
// and auto-reconnects with exponential backoff.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	crosstalk "github.com/aleksclark/crosstalk/cli"
	"github.com/aleksclark/crosstalk/cli/display"
	"github.com/aleksclark/crosstalk/cli/pipewire"
	"github.com/aleksclark/crosstalk/cli/pion"
	"github.com/aleksclark/crosstalk/cli/protov2"
	"github.com/pion/webrtc/v4"
)

// ABCConfig holds the ABC client configuration.
type ABCConfig struct {
	ServerURL  string `json:"server_url"`
	Token      string `json:"token"`
	SourceName string `json:"source_name,omitempty"`
	SinkName   string `json:"sink_name,omitempty"`
	LogLevel   string `json:"log_level,omitempty"`
}

// LoadConfig loads ABC configuration from a JSON file.
func LoadConfig(path string) (*ABCConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}

	cfg := &ABCConfig{
		LogLevel: "info",
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %s: %w", path, err)
	}

	return cfg, nil
}

// Validate checks that required fields are present.
func (c *ABCConfig) Validate() error {
	if c.ServerURL == "" {
		return &crosstalk.ValidationError{Field: "server_url", Message: "required"}
	}
	if c.Token == "" {
		return &crosstalk.ValidationError{Field: "token", Message: "required"}
	}
	return nil
}

// ToCLIConfig converts to the cli module's Config type.
func (c *ABCConfig) ToCLIConfig() *crosstalk.Config {
	return &crosstalk.Config{
		ServerURL:  c.ServerURL,
		Token:      c.Token,
		SourceName: c.SourceName,
		SinkName:   c.SinkName,
		LogLevel:   c.LogLevel,
	}
}

// ABCClient wraps the reconnection logic for the ABC client.
type ABCClient struct {
	cfg         *ABCConfig
	pwSvc       crosstalk.PipeWireService
	disp        *display.Service
	connFactory func(serverURL, token string, opts ...pion.ConnectionOption) pion.ConnectionInterface

	mu               sync.Mutex
	connected        bool
	assignedSession  string
	restartRequested bool
}

// NewABCClient creates a new ABC client.
func NewABCClient(cfg *ABCConfig, pwSvc crosstalk.PipeWireService) *ABCClient {
	return &ABCClient{
		cfg:   cfg,
		pwSvc: pwSvc,
	}
}

// Run starts the ABC client with auto-reconnect and exponential backoff.
func (c *ABCClient) Run(ctx context.Context) error {
	attempt := 0
	const (
		initialBackoff = 1 * time.Second
		maxBackoff     = 60 * time.Second
		backoffFactor  = 2.0
	)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := c.connectOnce(ctx)
		if err != nil {
			if pion.IsAuthError(err) {
				slog.Error("abc: authentication failed, not retrying", "error", err)
				return fmt.Errorf("authentication failed: %w", err)
			}

			attempt++
			backoff := CalculateBackoff(attempt, initialBackoff, maxBackoff, backoffFactor)
			slog.Warn("abc: connection failed, will retry",
				"error", err,
				"attempt", attempt,
				"backoff", backoff,
			)

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
				continue
			}
		}

		// Connected successfully, reset attempt counter.
		attempt = 0

		// Check if we got a restart request.
		c.mu.Lock()
		restarting := c.restartRequested
		c.restartRequested = false
		c.mu.Unlock()

		if restarting {
			slog.Info("abc: restart requested, reconnecting immediately")
			continue
		}

		// Normal disconnection, retry with backoff.
		attempt++
		backoff := CalculateBackoff(attempt, initialBackoff, maxBackoff, backoffFactor)
		slog.Info("abc: connection lost, reconnecting",
			"attempt", attempt,
			"backoff", backoff,
		)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}
}

func (c *ABCClient) connectOnce(ctx context.Context) error {
	slog.Info("abc: connecting", "server", c.cfg.ServerURL)

	controlOpened := make(chan struct{}, 1)
	welcomeReceived := make(chan *protov2.WelcomeV2, 1)
	disconnected := make(chan struct{}, 1)
	restartCh := make(chan string, 1)
	sessionAssignCh := make(chan string, 1)

	connOpts := []pion.ConnectionOption{
		pion.WithOnControlOpen(func() {
			select {
			case controlOpened <- struct{}{}:
			default:
			}
		}),
		pion.WithOnControlMessage(func(data []byte) {
			// Parse as v2 control message.
			msg, err := protov2.ParseControlMessageV2(data)
			if err != nil {
				slog.Warn("abc: unparseable control message", "error", err)
				return
			}

			switch msg.Type {
			case protov2.PayloadWelcome:
				if msg.Welcome != nil {
					select {
					case welcomeReceived <- msg.Welcome:
					default:
					}
				}
			case protov2.PayloadRestart:
				if msg.Restart != nil {
					slog.Info("abc: received restart command", "reason", msg.Restart.Reason)
					select {
					case restartCh <- msg.Restart.Reason:
					default:
					}
				}
			case protov2.PayloadSessionAssignment:
				if msg.SessionAssignment != nil {
					slog.Info("abc: received session assignment",
						"session_id", msg.SessionAssignment.SessionID,
						"role", msg.SessionAssignment.Role,
					)
					select {
					case sessionAssignCh <- msg.SessionAssignment.SessionID:
					default:
					}
				}
			}
		}),
		pion.WithOnConnectionStateChange(func(state webrtc.ICEConnectionState) {
			slog.Info("abc: ICE connection state changed", "state", state.String())
			switch state {
			case webrtc.ICEConnectionStateDisconnected,
				webrtc.ICEConnectionStateFailed,
				webrtc.ICEConnectionStateClosed:
				select {
				case disconnected <- struct{}{}:
				default:
				}
			}
		}),
		// Publish the booth microphone up-front. The server (SFU) forwards this
		// track into the ABC's assigned session feed channel.
		pion.WithPublishAudio("abc-mic", func(track *webrtc.TrackLocalStaticRTP) {
			go func() {
				if err := pion.CaptureSource(ctx, c.cfg.SourceName, track); err != nil && ctx.Err() == nil {
					slog.Error("abc: audio capture failed", "source", c.cfg.SourceName, "error", err)
				}
			}()
		}),
		// Play booth return audio (the broadcast mix) to the sink.
		pion.WithOnTrack(func(remote *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
			if remote.Kind() != webrtc.RTPCodecTypeAudio {
				return
			}
			go func() {
				if err := pion.PlaybackSink(ctx, c.cfg.SinkName, remote); err != nil && ctx.Err() == nil {
					slog.Error("abc: audio playback failed", "sink", c.cfg.SinkName, "error", err)
				}
			}()
		}),
	}
	var conn pion.ConnectionInterface
	if c.connFactory != nil {
		conn = c.connFactory(c.cfg.ServerURL, c.cfg.Token, connOpts...)
	} else {
		conn = pion.NewConnection(c.cfg.ServerURL, c.cfg.Token, connOpts...)
	}

	// Connect (WebSocket + WebRTC).
	connectCtx, connectCancel := context.WithTimeout(ctx, 30*time.Second)
	defer connectCancel()

	connectDone := make(chan error, 1)
	go func() {
		connectDone <- conn.Connect(connectCtx)
	}()

	// Wait for control channel or connection error.
	select {
	case <-controlOpened:
		slog.Info("abc: control channel established")
	case err := <-connectDone:
		if err != nil {
			return fmt.Errorf("connect failed: %w", err)
		}
		select {
		case <-controlOpened:
		case <-time.After(5 * time.Second):
			conn.Close()
			return fmt.Errorf("timeout waiting for control channel")
		}
	case <-ctx.Done():
		conn.Close()
		return ctx.Err()
	}

	// Send Hello with client_type="abc" using v2 proto format.
	clientName := c.cfg.SourceName
	if clientName == "" {
		clientName = "ct-abc"
	}
	helloData := protov2.BuildHelloV2("abc", clientName)
	if err := conn.SendControl(helloData); err != nil {
		conn.Close()
		return fmt.Errorf("sending Hello: %w", err)
	}
	slog.Info("abc: Hello sent", "client_type", "abc", "client_name", clientName)

	// Wait for Welcome.
	select {
	case welcome := <-welcomeReceived:
		slog.Info("abc: received Welcome",
			"peer_id", welcome.PeerID,
			"server_version", welcome.ServerVersion,
			"assigned_session", welcome.AssignedSessionID,
		)
		// If session pre-assigned in Welcome, use it.
		if welcome.AssignedSessionID != "" {
			c.mu.Lock()
			c.assignedSession = welcome.AssignedSessionID
			c.mu.Unlock()
		}
		if c.disp != nil {
			c.disp.Status().SetControlState("connected")
			c.disp.Status().SetSession(welcome.AssignedSessionID, "abc", welcome.AssignedSessionID != "")
		}
	case <-time.After(5 * time.Second):
		slog.Warn("abc: timeout waiting for Welcome message")
	case <-ctx.Done():
		conn.Close()
		return ctx.Err()
	}

	c.mu.Lock()
	c.connected = true
	c.mu.Unlock()

	slog.Info("abc: connected and ready")

	// Main event loop: wait for disconnection or restart command.
	for {
		select {
		case <-ctx.Done():
			conn.Close()
			return ctx.Err()
		case <-disconnected:
			c.mu.Lock()
			c.connected = false
			c.mu.Unlock()
			if c.disp != nil {
				c.disp.Status().SetControlState("disconnected")
				c.disp.Status().SetSession("", "", false)
			}
			conn.Close()
			return nil
		case reason := <-restartCh:
			slog.Info("abc: executing restart", "reason", reason)
			c.mu.Lock()
			c.connected = false
			c.restartRequested = true
			c.mu.Unlock()
			if c.disp != nil {
				c.disp.Status().SetControlState("connecting")
			}
			conn.Close()
			return nil
		case sessionID := <-sessionAssignCh:
			c.mu.Lock()
			c.assignedSession = sessionID
			c.mu.Unlock()
			slog.Info("abc: session assigned, joining", "session_id", sessionID)
			if c.disp != nil {
				c.disp.Status().SetSession(sessionID, "abc", sessionID != "")
			}
			// Join the session via v1 proto (JoinSession is field 4 in both v1).
			if err := conn.SendJoinSession(sessionID, "abc"); err != nil {
				slog.Error("abc: failed to join session", "session_id", sessionID, "error", err)
			}
		}
	}
}

// CalculateBackoff computes exponential backoff duration.
func CalculateBackoff(attempt int, initial, max time.Duration, factor float64) time.Duration {
	backoff := time.Duration(float64(initial) * math.Pow(factor, float64(attempt-1)))
	if backoff > max {
		backoff = max
	}
	return backoff
}

// parseSlogLevel converts a string log level to slog.Level.
func parseSlogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func main() {
	configPath := flag.String("config", "", "path to config file")
	flag.Parse()

	// Determine config path.
	path := *configPath
	if path == "" {
		path = os.Getenv("CT_ABC_CONFIG")
	}
	if path == "" {
		path = os.Getenv("CROSSTALK_CONFIG")
	}
	if path == "" {
		path = "ct-abc.json"
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Apply env var overrides.
	if v := os.Getenv("CT_ABC_SERVER"); v != "" {
		cfg.ServerURL = v
	}
	if v := os.Getenv("CT_ABC_TOKEN"); v != "" {
		cfg.Token = v
	}
	if v := os.Getenv("CT_ABC_SOURCE"); v != "" {
		cfg.SourceName = v
	}
	if v := os.Getenv("CT_ABC_SINK"); v != "" {
		cfg.SinkName = v
	}
	if v := os.Getenv("CT_ABC_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Config validation error: %v\n", err)
		os.Exit(1)
	}

	// Configure structured logging.
	level := parseSlogLevel(cfg.LogLevel)
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(handler))

	slog.Info("ct-abc starting",
		"server_url", cfg.ServerURL,
		"source_name", cfg.SourceName,
		"sink_name", cfg.SinkName,
	)

	// Set up PipeWire service.
	pwSvc := pipewire.NewService(cfg.SourceName, cfg.SinkName)

	// Set up signal handling.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Run the ABC client.
	client := NewABCClient(cfg, pwSvc)

	// Set up the SPI status display if enabled.
	if useDisplay() {
		spiPath := os.Getenv("DISPLAY_SPI_DEVICE")
		if spiPath == "" {
			spiPath = "/dev/spidev0.1"
		}
		disp := display.NewService(spiPath, 71, 76) // DC=PC7(71), RST=PC12(76)
		disp.SetLevelMeters(pion.InputMeter, pion.OutputMeter)
		disp.Status().SetServer(cfg.ServerURL, "connecting")
		client.disp = disp

		// The booth mic is published continuously (WithPublishAudio → CaptureSource),
		// which feeds InputMeter on every frame, so the input VU tracks the mic
		// without a separate idle monitor. A second reader would contend with the
		// single-open ALSA capture device and break the publish, so none is used.

		go func() {
			if err := disp.Run(ctx); err != nil {
				slog.Error("display service failed", "error", err)
			}
		}()
	}

	if err := client.Run(ctx); err != nil {
		if ctx.Err() != nil {
			slog.Info("ct-abc shutting down")
		} else {
			slog.Error("ct-abc fatal error", "error", err)
			os.Exit(1)
		}
	}
}

// useDisplay reports whether the SPI status display should be driven.
func useDisplay() bool {
	v := os.Getenv("USE_DISPLAY")
	return strings.EqualFold(v, "true") || v == "1"
}
