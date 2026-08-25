// ct-abc is the Audio Booth Connector client binary.
// It connects to the CrossTalk server via WebSocket signaling, sends a Hello
// with client_type="abc", handles SessionAssignment and RestartCommand messages,
// applies bounded USB mixer AudioControlCommand messages, and auto-reconnects
// with exponential backoff.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/aleksclark/crosstalk/abc"
	crosstalk "github.com/aleksclark/crosstalk/cli"
	"github.com/aleksclark/crosstalk/cli/audioctl"
	"github.com/aleksclark/crosstalk/cli/display"
	"github.com/aleksclark/crosstalk/cli/pipewire"
	"github.com/aleksclark/crosstalk/cli/pion"
)

// managedMarkerName is the atomic marker under StateDirectory after first managed command.
const managedMarkerName = "audio-managed"

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
	cfg            *ABCConfig
	pwSvc          crosstalk.PipeWireService
	disp           *display.Service
	audio          audioctl.Controller
	stateDir       string
	dial          func(ctx context.Context, cfg abc.Config) (*abc.Session, error)
	captureSource func(context.Context, string, abc.RTPWriter) error
	heartbeatEvery time.Duration

	mu               sync.Mutex
	connected        bool
	assignedSession  string
	restartRequested bool
}

// NewABCClient creates a new ABC client.
func NewABCClient(cfg *ABCConfig, pwSvc crosstalk.PipeWireService) *ABCClient {
	return &ABCClient{
		cfg:            cfg,
		pwSvc:          pwSvc,
		captureSource:  pion.CaptureToWriter,
		heartbeatEvery: 15 * time.Second,
	}
}

// SetAudioController injects the USB mixer controller (tests / main).
func (c *ABCClient) SetAudioController(ctrl audioctl.Controller, stateDir string) {
	c.audio = ctrl
	c.stateDir = stateDir
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
			if abc.IsAuthError(err) {
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

	connectionCtx, cancelConnection := context.WithCancel(ctx)
	defer cancelConnection()

	restartCh := make(chan string, 1)
	sessionAssignCh := make(chan string, 1)
	audioCmdCh := make(chan *abc.AudioControlCommand, 4)

	clientName := c.cfg.SourceName
	if clientName == "" {
		clientName = "ct-abc"
	}

	dial := c.dial
	if dial == nil {
		dial = abc.Dial
	}
	connectCtx, connectCancel := context.WithTimeout(ctx, 30*time.Second)
	defer connectCancel()
	sess, err := dial(connectCtx, abc.Config{
		ServerURL:      c.cfg.ServerURL,
		Token:          c.cfg.Token,
		ClientName:     clientName,
		PublishTrackID: "abc-mic",
		WelcomeTimeout: 5 * time.Second,
		Logger:         slog.Default(),
	})
	if err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}
	defer sess.Close()

	welcome := sess.Welcome()
	slog.Info("abc: received Welcome",
		"peer_id", welcome.PeerID,
		"server_version", welcome.ServerVersion,
		"assigned_session", welcome.AssignedSessionID,
		"epoch", welcome.Epoch,
	)
	if codec, ok := sess.NegotiatedCodec(); ok {
		slog.Info("abc: negotiated codec",
			"mime", codec.MimeType,
			"clock_rate", codec.ClockRate,
			"channels", codec.Channels,
			"payload_type", codec.PayloadType,
		)
	}
	if welcome.AssignedSessionID != "" {
		c.mu.Lock()
		c.assignedSession = welcome.AssignedSessionID
		c.mu.Unlock()
	}
	if c.disp != nil {
		c.disp.Status().SetControlState("connected")
		c.disp.Status().SetSession(welcome.AssignedSessionID, "abc", welcome.AssignedSessionID != "")
	}

	sess.OnControl(func(msg abc.ControlMessage) {
		switch {
		case msg.Restart != nil:
			slog.Info("abc: received restart command", "reason", msg.Restart.Reason)
			select {
			case restartCh <- msg.Restart.Reason:
			default:
			}
		case msg.SessionAssignment != nil:
			slog.Info("abc: received session assignment",
				"session_id", msg.SessionAssignment.SessionID,
				"role", msg.SessionAssignment.Role,
			)
			select {
			case sessionAssignCh <- msg.SessionAssignment.SessionID:
			default:
			}
		case msg.AudioControlCommand != nil:
			select {
			case audioCmdCh <- msg.AudioControlCommand:
			default:
				slog.Warn("abc: audio control command dropped (busy)")
			}
		}
	})
	sess.OnTrack(func(track abc.IncomingTrack) {
		if track.Pion == nil {
			return
		}
		go func() {
			if err := pion.PlaybackSink(connectionCtx, c.cfg.SinkName, track.Pion); err != nil && connectionCtx.Err() == nil {
				slog.Error("abc: audio playback failed", "sink", c.cfg.SinkName, "error", err)
			}
		}()
	})

	go func() {
		for connectionCtx.Err() == nil {
			err := c.captureSource(connectionCtx, c.cfg.SourceName, sess.SendTrack())
			if connectionCtx.Err() != nil {
				return
			}
			if err != nil {
				slog.Error("abc: audio capture failed, restarting",
					"source", c.cfg.SourceName, "error", err)
			} else {
				slog.Warn("abc: audio capture ended, restarting", "source", c.cfg.SourceName)
			}
			select {
			case <-connectionCtx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}()

	sendControl := func(data []byte) error {
		return sess.SendControl(data)
	}

	c.mu.Lock()
	c.connected = true
	c.mu.Unlock()

	slog.Info("abc: connected and ready")

	if c.audio != nil {
		c.sendInventoryReport(connectionCtx, sendControl)
		go c.audioHeartbeatLoop(connectionCtx, sendControl)
		go c.audioCommandLoop(connectionCtx, audioCmdCh, sendControl)
	}

	for {
		select {
		case <-ctx.Done():
			c.markDisconnected()
			return ctx.Err()
		case <-sess.Done():
			c.markDisconnected()
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
			return nil
		case sessionID := <-sessionAssignCh:
			c.mu.Lock()
			c.assignedSession = sessionID
			c.mu.Unlock()
			slog.Info("abc: session assigned", "session_id", sessionID)
			if c.disp != nil {
				c.disp.Status().SetSession(sessionID, "abc", sessionID != "")
			}
		}
	}
}

func (c *ABCClient) markDisconnected() {
	c.mu.Lock()
	c.connected = false
	c.mu.Unlock()
	if c.disp != nil {
		c.disp.Status().SetControlState("disconnected")
		c.disp.Status().SetSession("", "", false)
	}
}

func (c *ABCClient) audioHeartbeatLoop(ctx context.Context, send func([]byte) error) {
	// Initial jitter 0-2s so fleets don't align.
	jitter := time.Duration(rand.Int63n(int64(2 * time.Second)))
	timer := time.NewTimer(jitter)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			c.sendReadbackReport(ctx, send)
			// 15s ± ~2s jitter
			next := c.heartbeatEvery
			if next <= 0 {
				next = 15 * time.Second
			}
			j := time.Duration(rand.Int63n(int64(2*time.Second))) - time.Second
			next = next + j
			if next < 5*time.Second {
				next = 5 * time.Second
			}
			timer.Reset(next)
		}
	}
}

func (c *ABCClient) audioCommandLoop(ctx context.Context, ch <-chan *abc.AudioControlCommand, send func([]byte) error) {
	for {
		select {
		case <-ctx.Done():
			return
		case cmd, ok := <-ch:
			if !ok {
				return
			}
			if cmd == nil {
				continue
			}
			c.handleAudioCommand(ctx, cmd, send)
		}
	}
}

func (c *ABCClient) handleAudioCommand(ctx context.Context, cmd *abc.AudioControlCommand, send func([]byte) error) {
	acmd, err := protoCommandToAudioctl(cmd)
	if err != nil {
		slog.Warn("abc: invalid audio control command", "error", err)
		rep := &audioctl.Report{
			CommandID:       cmd.CommandID,
			DesiredRevision: cmd.DesiredRevision,
			ErrorCode:       audioctl.ErrCodeInvalidCommand,
			ErrorDetail:     err.Error(),
			Output:          &audioctl.OutputObserved{VolumeState: audioctl.StateError, MuteState: audioctl.StateError},
			Input:           &audioctl.InputObserved{GainState: audioctl.StateError},
		}
		c.sendAudioReport(send, rep)
		return
	}
	rep, err := c.audio.Apply(ctx, acmd)
	if err != nil {
		slog.Error("abc: audio apply failed", "error", err)
		return
	}
	// After a valid managed command (revision >= 1 with apply attempt), write marker.
	// Marker is NOT desired authority — only prevents boot helper overwrite.
	if cmd.DesiredRevision >= 1 && c.stateDir != "" && rep.ErrorCode != audioctl.ErrCodeInvalidCommand {
		if err := writeManagedMarker(c.stateDir); err != nil {
			slog.Warn("abc: failed to write audio-managed marker", "error", err)
		}
	}
	c.sendAudioReport(send, rep)
}

func (c *ABCClient) sendInventoryReport(ctx context.Context, send func([]byte) error) {
	rep, err := c.audio.Inventory(ctx)
	if err != nil {
		slog.Warn("abc: audio inventory failed", "error", err)
		return
	}
	// Inventory-only: revision 0
	rep.DesiredRevision = 0
	rep.CommandID = ""
	c.sendAudioReport(send, rep)
}

func (c *ABCClient) sendReadbackReport(ctx context.Context, send func([]byte) error) {
	rep, err := c.audio.Readback(ctx)
	if err != nil {
		slog.Warn("abc: audio readback failed", "error", err)
		return
	}
	// Heartbeat inventory/readback uses revision 0 unless reporting prior apply.
	if rep.DesiredRevision == 0 {
		rep.CommandID = ""
	}
	c.sendAudioReport(send, rep)
}

func (c *ABCClient) sendAudioReport(send func([]byte) error, rep *audioctl.Report) {
	if rep == nil {
		return
	}
	preport := audioctlToProtoReport(rep)
	data, err := abc.EncodeAudioControlReport(preport)
	if err != nil {
		slog.Warn("abc: build audio report failed", "error", err)
		return
	}
	if err := send(data); err != nil {
		slog.Warn("abc: send audio report failed", "error", err)
	}
}

func protoCommandToAudioctl(cmd *abc.AudioControlCommand) (audioctl.Command, error) {
	out := audioctl.Command{
		CommandID:       cmd.CommandID,
		DesiredRevision: cmd.DesiredRevision,
	}
	if cmd.Output != nil {
		od := &audioctl.OutputDesired{DeviceUID: cmd.Output.DeviceUID}
		if cmd.Output.VolumePercent != nil {
			v := int(*cmd.Output.VolumePercent)
			od.VolumePercent = &v
		}
		if cmd.Output.Muted != nil {
			m := *cmd.Output.Muted
			od.Muted = &m
		}
		out.Output = od
	}
	if cmd.Input != nil {
		id := &audioctl.InputDesired{DeviceUID: cmd.Input.DeviceUID}
		if cmd.Input.GainPercent != nil {
			g := int(*cmd.Input.GainPercent)
			id.GainPercent = &g
		}
		out.Input = id
	}
	if out.Output == nil && out.Input == nil {
		return out, fmt.Errorf("empty audio command")
	}
	return out, nil
}

func audioctlToProtoReport(rep *audioctl.Report) abc.AudioControlReport {
	pr := abc.AudioControlReport{
		CommandID:       rep.CommandID,
		DesiredRevision: rep.DesiredRevision,
		ErrorCode:       rep.ErrorCode,
		ErrorDetail:     rep.ErrorDetail,
	}
	for _, d := range rep.Devices {
		pr.Devices = append(pr.Devices, abc.AudioDeviceCapability{
			DeviceUID:          d.DeviceUID,
			Direction:          d.Direction,
			Backend:            string(d.Backend),
			VendorID:           d.VendorID,
			ProductID:          d.ProductID,
			Serial:             d.Serial,
			Path:               d.Path,
			CardID:             d.ALSACardID,
			CardName:           d.CardName,
			PCMRoute:           d.PCMRoute,
			SupportsVolume:     d.SupportsVolume,
			SupportsMute:       d.SupportsMute,
			SupportsGain:       d.SupportsGain,
			SupportsAGCDisable: d.SupportsAGCDisable,
		})
	}
	if rep.Output != nil {
		o := &abc.AudioOutputObserved{
			DeviceUID:   rep.Output.DeviceUID,
			VolumeState: applyStateToProto(rep.Output.VolumeState),
			MuteState:   applyStateToProto(rep.Output.MuteState),
		}
		if rep.Output.VolumePercent != nil {
			v := uint32(*rep.Output.VolumePercent)
			o.VolumePercent = &v
		}
		if rep.Output.Muted != nil {
			m := *rep.Output.Muted
			o.Muted = &m
		}
		pr.Output = o
	}
	if rep.Input != nil {
		i := &abc.AudioInputObserved{
			DeviceUID: rep.Input.DeviceUID,
			GainState: applyStateToProto(rep.Input.GainState),
		}
		if rep.Input.GainPercent != nil {
			g := uint32(*rep.Input.GainPercent)
			i.GainPercent = &g
		}
		pr.Input = i
	}
	return pr
}

func applyStateToProto(s audioctl.ApplyState) abc.AudioApplyState {
	switch s {
	case audioctl.StateApplied:
		return abc.AudioApplyApplied
	case audioctl.StateUnsupported:
		return abc.AudioApplyUnsupported
	case audioctl.StateError:
		return abc.AudioApplyError
	case audioctl.StateDeviceMismatch:
		return abc.AudioApplyDeviceMismatch
	case audioctl.StateStaleRevision:
		return abc.AudioApplyStaleRevision
	default:
		return abc.AudioApplyUnspecified
	}
}

// writeManagedMarker atomically writes StateDirectory/audio-managed.
func writeManagedMarker(stateDir string) error {
	if stateDir == "" {
		return fmt.Errorf("empty state dir")
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	final := filepath.Join(stateDir, managedMarkerName)
	tmp := final + ".tmp"
	content := fmt.Sprintf("managed_at=%s\n", time.Now().UTC().Format(time.RFC3339))
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

// ManagedMarkerPath returns the marker path for tests/helpers.
func ManagedMarkerPath(stateDir string) string {
	return filepath.Join(stateDir, managedMarkerName)
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
	stateDirFlag := flag.String("state-dir", "", "writable state directory for managed audio marker")
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

	stateDir := *stateDirFlag
	if stateDir == "" {
		stateDir = os.Getenv("CT_ABC_STATE_DIR")
	}
	if stateDir == "" {
		// systemd StateDirectory=crosstalk → /var/lib/crosstalk
		stateDir = "/var/lib/crosstalk"
	}

	slog.Info("ct-abc starting",
		"server_url", cfg.ServerURL,
		"source_name", cfg.SourceName,
		"sink_name", cfg.SinkName,
		"state_dir", stateDir,
	)

	// Set up PipeWire service.
	pwSvc := pipewire.NewService(cfg.SourceName, cfg.SinkName)

	// Set up signal handling.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Run the ABC client.
	client := NewABCClient(cfg, pwSvc)
	audioCtrl := audioctl.NewController(audioctl.Config{
		SourceRoute: cfg.SourceName,
		SinkRoute:   cfg.SinkName,
	})
	client.SetAudioController(audioCtrl, stateDir)

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
