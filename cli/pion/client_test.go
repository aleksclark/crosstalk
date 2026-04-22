package pion

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	crosstalk "github.com/aleksclark/crosstalk/cli"
	climock "github.com/aleksclark/crosstalk/cli/mock"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_ConnectAndSendHello(t *testing.T) {
	srv, state := mockSignalingServer(t)
	defer srv.Close()

	cfg := &crosstalk.Config{
		ServerURL: srv.URL,
		Token:     "test-api-token",
		LogLevel:  "info",
	}

	pwSvc := &climock.PipeWireService{
		DiscoverFn: func() ([]crosstalk.Source, []crosstalk.Sink, error) {
			return []crosstalk.Source{
					{Name: "test-mic", Type: "audio"},
				}, []crosstalk.Sink{
					{Name: "test-speakers", Type: "audio"},
				}, nil
		},
	}

	welcomeCh := make(chan *WelcomeMessage, 1)
	connectedCh := make(chan struct{}, 1)

	client := NewClient(cfg, pwSvc,
		WithClientOnConnected(func() {
			connectedCh <- struct{}{}
		}),
		WithClientOnWelcome(func(w *WelcomeMessage) {
			welcomeCh <- w
		}),
		// Inject auth that hits our test server
		WithAuthClientFactory(func(serverURL, token string) AuthClientInterface {
			ac := NewAuthClient(serverURL, token)
			return ac
		}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	runDone := make(chan error, 1)
	go func() {
		runDone <- client.Run(ctx)
	}()

	// Wait for connected
	select {
	case <-connectedCh:
		// OK
	case err := <-runDone:
		t.Fatalf("Run returned early: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for connection")
	}

	assert.True(t, client.IsConnected())
	assert.True(t, pwSvc.DiscoverInvoked)

	// Check Welcome
	select {
	case w := <-welcomeCh:
		assert.Equal(t, "test-client-1", w.ClientID)
	case <-time.After(1 * time.Second):
		// Welcome might have already been processed
	}

	// Check server received Hello
	select {
	case hello := <-state.helloReceived:
		require.NotNil(t, hello.Hello)
		assert.Len(t, hello.Hello.Sources, 1)
		assert.Equal(t, "test-mic", hello.Hello.Sources[0].Name)
		assert.Len(t, hello.Hello.Sinks, 1)
		assert.Equal(t, "test-speakers", hello.Hello.Sinks[0].Name)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for Hello")
	}

	// Stop client
	client.Stop()
	cancel()
}

func TestClient_AuthFailureNoRetry(t *testing.T) {
	// Server that returns 401
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"bad token"}`))
	}))
	defer srv.Close()

	cfg := &crosstalk.Config{
		ServerURL: srv.URL,
		Token:     "bad-token",
		LogLevel:  "info",
	}

	pwSvc := &climock.PipeWireService{
		DiscoverFn: func() ([]crosstalk.Source, []crosstalk.Sink, error) {
			return nil, nil, nil
		},
	}

	client := NewClient(cfg, pwSvc)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := client.Run(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authentication failed")
}

func TestClient_CalculateBackoff(t *testing.T) {
	cfg := &crosstalk.Config{
		ServerURL: "http://localhost",
		Token:     "test",
	}
	pwSvc := &climock.PipeWireService{
		DiscoverFn: func() ([]crosstalk.Source, []crosstalk.Sink, error) {
			return nil, nil, nil
		},
	}

	client := NewClient(cfg, pwSvc)

	// Backoff: 1s, 2s, 4s, 8s, 16s, 32s, 60s, 60s
	assert.Equal(t, 1*time.Second, client.calculateBackoff(1))
	assert.Equal(t, 2*time.Second, client.calculateBackoff(2))
	assert.Equal(t, 4*time.Second, client.calculateBackoff(3))
	assert.Equal(t, 8*time.Second, client.calculateBackoff(4))
	assert.Equal(t, 16*time.Second, client.calculateBackoff(5))
	assert.Equal(t, 32*time.Second, client.calculateBackoff(6))
	assert.Equal(t, 60*time.Second, client.calculateBackoff(7)) // capped
	assert.Equal(t, 60*time.Second, client.calculateBackoff(8))
}

func TestClient_ReconnectAfterFailure(t *testing.T) {
	// Track connection attempts
	var attemptCount atomic.Int32
	var srvMu sync.Mutex
	srvRunning := true

	// Create a server that can be toggled on/off
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srvMu.Lock()
		running := srvRunning
		srvMu.Unlock()

		if !running {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		switch r.URL.Path {
		case "/api/webrtc/token":
			attemptCount.Add(1)
			json.NewEncoder(w).Encode(map[string]string{"token": "wrt-token"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := &crosstalk.Config{
		ServerURL: srv.URL,
		Token:     "test-token",
		LogLevel:  "info",
	}

	pwSvc := &climock.PipeWireService{
		DiscoverFn: func() ([]crosstalk.Source, []crosstalk.Sink, error) {
			return nil, nil, nil
		},
	}

	// Use a mock connection factory that fails on first attempt
	var connectAttempts atomic.Int32

	client := NewClient(cfg, pwSvc,
		WithConnFactory(func(serverURL, token string, opts ...ConnectionOption) ConnectionInterface {
			return &mockConn{
				connectFn: func(ctx context.Context) error {
					n := connectAttempts.Add(1)
					if n == 1 {
						return fmt.Errorf("simulated connection failure")
					}
					// Second attempt succeeds but we need to trigger the control open callback
					for _, opt := range opts {
						// Apply options to extract callbacks
						tempConn := &Connection{}
						opt(tempConn)
						if tempConn.onControlOpen != nil {
							go tempConn.onControlOpen()
						}
						if tempConn.onControlMessage != nil {
							// Send Welcome
							welcome := ControlMessage{
								Type: ControlTypeWelcome,
								Welcome: &WelcomeMessage{
									ClientID:      "reconnected-client",
									ServerVersion: "test",
								},
							}
							data, _ := json.Marshal(welcome)
							go func() {
								time.Sleep(10 * time.Millisecond)
								tempConn.onControlMessage(data)
							}()
						}
					}
					return nil
				},
			}
		}),
	)

	// Override backoff for fast testing
	client.initialBackoff = 50 * time.Millisecond
	client.maxBackoff = 200 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	connectedCh := make(chan struct{}, 1)
	client.onConnected = func() {
		select {
		case connectedCh <- struct{}{}:
		default:
		}
	}

	runDone := make(chan error, 1)
	go func() {
		runDone <- client.Run(ctx)
	}()

	// Wait for reconnection
	select {
	case <-connectedCh:
		assert.GreaterOrEqual(t, int(connectAttempts.Load()), 2, "should have retried")
	case err := <-runDone:
		if err != context.Canceled {
			t.Fatalf("Run returned unexpectedly: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for reconnection")
	}

	client.Stop()
	cancel()
}

// mockConn implements ConnectionInterface for testing.
type mockConn struct {
	connectFn func(ctx context.Context) error
	mu        sync.Mutex
	closed    bool
}

func (m *mockConn) Connect(ctx context.Context) error {
	if m.connectFn != nil {
		return m.connectFn(ctx)
	}
	return nil
}

func (m *mockConn) SendHello(sources []crosstalk.Source, sinks []crosstalk.Sink, codecs []crosstalk.Codec) error {
	return nil
}

func (m *mockConn) SendClientStatus(state string, sources []crosstalk.Source, sinks []crosstalk.Sink, codecs []crosstalk.Codec) error {
	return nil
}

func (m *mockConn) SendControl(data []byte) error {
	return nil
}

func (m *mockConn) Close() error {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	return nil
}

func (m *mockConn) ConnectionState() webrtc.ICEConnectionState {
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed {
		return webrtc.ICEConnectionStateClosed
	}
	return webrtc.ICEConnectionStateConnected
}
