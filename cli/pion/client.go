package pion

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	crosstalk "github.com/anthropics/crosstalk/cli"
	"github.com/pion/webrtc/v4"
)

// Client manages the full lifecycle of connecting to a CrossTalk server,
// including authentication, WebRTC connection, and reconnection with
// exponential backoff.
type Client struct {
	mu sync.Mutex

	config   *crosstalk.Config
	pwSvc    crosstalk.PipeWireService
	conn     *Connection
	clientID string

	// Backoff settings
	initialBackoff time.Duration
	maxBackoff     time.Duration
	backoffFactor  float64

	// State
	connected bool
	stopped   bool

	// Callbacks
	onConnected    func()
	onDisconnected func()
	onWelcome      func(*WelcomeMessage)

	// For testing: allow injecting auth client and connection factory
	authClientFactory func(serverURL, token string) AuthClientInterface
	connFactory       func(serverURL, token string, opts ...ConnectionOption) ConnectionInterface
}

// AuthClientInterface abstracts the auth client for testing.
type AuthClientInterface interface {
	RequestWebRTCToken() (string, error)
}

// ConnectionInterface abstracts the WebRTC connection for testing.
type ConnectionInterface interface {
	Connect(ctx context.Context) error
	SendHello(sources []crosstalk.Source, sinks []crosstalk.Sink, codecs []crosstalk.Codec) error
	SendClientStatus(state string, sources []crosstalk.Source, sinks []crosstalk.Sink, codecs []crosstalk.Codec) error
	SendControl(data []byte) error
	Close() error
	ConnectionState() webrtc.ICEConnectionState
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithOnConnected sets the callback for when the client connects.
func WithClientOnConnected(fn func()) ClientOption {
	return func(c *Client) {
		c.onConnected = fn
	}
}

// WithOnDisconnected sets the callback for when the client disconnects.
func WithClientOnDisconnected(fn func()) ClientOption {
	return func(c *Client) {
		c.onDisconnected = fn
	}
}

// WithOnWelcome sets the callback for when a Welcome message is received.
func WithClientOnWelcome(fn func(*WelcomeMessage)) ClientOption {
	return func(c *Client) {
		c.onWelcome = fn
	}
}

// WithAuthClientFactory allows injecting a custom auth client for testing.
func WithAuthClientFactory(fn func(serverURL, token string) AuthClientInterface) ClientOption {
	return func(c *Client) {
		c.authClientFactory = fn
	}
}

// WithConnFactory allows injecting a custom connection factory for testing.
func WithConnFactory(fn func(serverURL, token string, opts ...ConnectionOption) ConnectionInterface) ClientOption {
	return func(c *Client) {
		c.connFactory = fn
	}
}

// NewClient creates a new Client with the given configuration.
func NewClient(cfg *crosstalk.Config, pwSvc crosstalk.PipeWireService, opts ...ClientOption) *Client {
	c := &Client{
		config:         cfg,
		pwSvc:          pwSvc,
		initialBackoff: 1 * time.Second,
		maxBackoff:     60 * time.Second,
		backoffFactor:  2.0,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Run starts the client and blocks until the context is cancelled.
// It handles connection, reconnection with exponential backoff, and
// sending capabilities.
func (c *Client) Run(ctx context.Context) error {
	attempt := 0

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := c.connectOnce(ctx)
		if err != nil {
			// Auth errors should not be retried
			if IsAuthError(err) {
				slog.Error("authentication failed, not retrying", "error", err)
				return fmt.Errorf("authentication failed: %w", err)
			}

			attempt++
			backoff := c.calculateBackoff(attempt)
			slog.Warn("connection failed, will retry",
				"error", err,
				"attempt", attempt,
				"backoff", backoff,
			)

			c.mu.Lock()
			c.connected = false
			c.mu.Unlock()
			if c.onDisconnected != nil {
				c.onDisconnected()
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
				continue
			}
		}

		// Connected successfully, reset attempt counter
		attempt = 0

		// Wait for disconnection
		c.waitForDisconnection(ctx)

		c.mu.Lock()
		stopped := c.stopped
		c.mu.Unlock()
		if stopped {
			return nil
		}

		c.mu.Lock()
		c.connected = false
		c.mu.Unlock()
		if c.onDisconnected != nil {
			c.onDisconnected()
		}

		// Reconnect after brief delay
		attempt++
		backoff := c.calculateBackoff(attempt)
		slog.Info("connection lost, reconnecting",
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

// Stop cleanly stops the client.
func (c *Client) Stop() {
	c.mu.Lock()
	c.stopped = true
	conn := c.conn
	c.mu.Unlock()

	if conn != nil {
		conn.Close()
	}
}

// IsConnected returns whether the client is currently connected.
func (c *Client) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

// ClientID returns the client ID assigned by the server.
func (c *Client) ClientID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.clientID
}

func (c *Client) connectOnce(ctx context.Context) error {
	// 1. Request WebRTC token
	slog.Info("connecting with API token", "server", c.config.ServerURL)

	// Use the API token directly for WebSocket auth (the server validates
	// API tokens on the WS signaling endpoint via FindTokenByHash).
	wsToken := c.config.Token

	// 2. Discover PipeWire devices
	sources, sinks, err := c.pwSvc.Discover()
	if err != nil {
		slog.Warn("PipeWire discovery failed, connecting without devices", "error", err)
		sources = nil
		sinks = nil
	}

	// Default codecs
	codecs := []crosstalk.Codec{
		{Name: "opus/48000/2", MediaType: "audio"},
	}

	// 3. Create connection
	controlOpened := make(chan struct{}, 1)
	welcomeReceived := make(chan *WelcomeMessage, 1)
	disconnected := make(chan struct{}, 1)

	connOpts := []ConnectionOption{
		WithOnControlOpen(func() {
			controlOpened <- struct{}{}
		}),
		WithOnControlMessage(func(data []byte) {
			msg, err := ParseControlMessage(data)
			if err != nil {
				slog.Warn("unparseable control message", "error", err)
				return
			}
			if msg.Type == ControlTypeWelcome && msg.Welcome != nil {
				welcomeReceived <- msg.Welcome
			}
		}),
		WithOnConnectionStateChange(func(state webrtc.ICEConnectionState) {
			slog.Info("connection state changed", "state", state.String())
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
	}

	var conn ConnectionInterface
	if c.connFactory != nil {
		conn = c.connFactory(c.config.ServerURL, wsToken, connOpts...)
	} else {
		conn = NewConnection(c.config.ServerURL, wsToken, connOpts...)
	}

	c.mu.Lock()
	// Store as *Connection if it's the real type, otherwise store the interface
	if realConn, ok := conn.(*Connection); ok {
		c.conn = realConn
	}
	c.mu.Unlock()

	// 4. Connect (WebSocket + WebRTC)
	connectCtx, connectCancel := context.WithTimeout(ctx, 30*time.Second)
	defer connectCancel()

	connectDone := make(chan error, 1)
	go func() {
		connectDone <- conn.Connect(connectCtx)
	}()

	// Wait for control channel to open or connection error
	select {
	case <-controlOpened:
		slog.Info("control channel established")
	case err := <-connectDone:
		if err != nil {
			return fmt.Errorf("connect failed: %w", err)
		}
		// Connect returned without error but control didn't open yet
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

	// 5. Send Hello
	if err := conn.SendHello(sources, sinks, codecs); err != nil {
		conn.Close()
		return fmt.Errorf("sending Hello: %w", err)
	}

	// 6. Wait for Welcome (optional, don't block forever)
	select {
	case welcome := <-welcomeReceived:
		c.mu.Lock()
		c.clientID = welcome.ClientID
		c.mu.Unlock()
		slog.Info("received Welcome",
			"client_id", welcome.ClientID,
			"server_version", welcome.ServerVersion,
		)
		if c.onWelcome != nil {
			c.onWelcome(welcome)
		}
	case <-time.After(5 * time.Second):
		slog.Warn("timeout waiting for Welcome message")
	case <-ctx.Done():
		conn.Close()
		return ctx.Err()
	}

	c.mu.Lock()
	c.connected = true
	c.mu.Unlock()

	if c.onConnected != nil {
		c.onConnected()
	}

	slog.Info("client connected and ready")
	return nil
}

func (c *Client) waitForDisconnection(ctx context.Context) {
	// Poll connection state
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.mu.Lock()
			conn := c.conn
			stopped := c.stopped
			c.mu.Unlock()

			if stopped {
				return
			}

			if conn != nil {
				state := conn.ConnectionState()
				switch state {
				case webrtc.ICEConnectionStateDisconnected,
					webrtc.ICEConnectionStateFailed,
					webrtc.ICEConnectionStateClosed:
					return
				}
			}
		}
	}
}

func (c *Client) calculateBackoff(attempt int) time.Duration {
	backoff := float64(c.initialBackoff) * math.Pow(c.backoffFactor, float64(attempt-1))
	if backoff > float64(c.maxBackoff) {
		backoff = float64(c.maxBackoff)
	}
	return time.Duration(backoff)
}

// Ensure *Connection implements ConnectionInterface
var _ ConnectionInterface = (*Connection)(nil)

// Ensure Connection exposes the right methods - add missing ones

// Connect satisfies ConnectionInterface (already defined in connection.go)
// SendHello satisfies ConnectionInterface (already defined in control.go)
// SendClientStatus satisfies ConnectionInterface (already defined in control.go)
// SendControl satisfies ConnectionInterface (already defined in connection.go)
// Close satisfies ConnectionInterface (already defined in connection.go)
// ConnectionState satisfies ConnectionInterface (already defined in connection.go)

// DefaultCodecs returns the default supported codecs.
func DefaultCodecs() []crosstalk.Codec {
	return []crosstalk.Codec{
		{Name: "opus/48000/2", MediaType: "audio"},
	}
}

// marshalControlMessage marshals a control message to JSON.
func marshalControlMessage(msg *ControlMessage) ([]byte, error) {
	return json.Marshal(msg)
}
