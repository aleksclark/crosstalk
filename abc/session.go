package abc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"nhooyr.io/websocket"
)

var (
	errTrackClosed     = errors.New("abc: track closed")
	errSessionClosed   = errors.New("abc: session closed")
	errControlNotReady = errors.New("abc: control data channel not open")
	errNoSendTrack     = errors.New("abc: send track not established")
)

var nextEpoch atomic.Uint64

// Session is one ABC connection epoch.
type Session struct {
	cfg    Config
	logger *slog.Logger

	mu     sync.Mutex
	state  State
	welcome Welcome
	codec  Codec
	hasCodec bool
	closeErr error
	closeReason string

	wsConn    *websocket.Conn
	peerConn  *webrtc.PeerConnection
	controlDC *webrtc.DataChannel
	sendTrack *webrtc.TrackLocalStaticRTP

	onTrack   func(IncomingTrack)
	onControl func(ControlMessage)
	onClose   func(reason string)

	pendingTracks   []IncomingTrack
	pendingControls []ControlMessage

	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	answerCh  chan struct{}
	welcomeCh chan struct{}
	gotWelcome atomic.Bool

	closedOnce sync.Once
}

// Dial connects to Crosstalk as an ABC, performs signaling, and waits for
// Welcome. If Welcome does not arrive, Dial fails when RequireWelcome is set
// and otherwise returns the session after a warning.
func Dial(ctx context.Context, cfg Config) (*Session, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	cfg = applyDefaults(cfg)
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	sessCtx, cancel := context.WithCancel(context.Background())
	s := &Session{
		cfg:       cfg,
		logger:    logger,
		state:     StateDialing,
		ctx:       sessCtx,
		cancel:    cancel,
		done:      make(chan struct{}),
		answerCh:  make(chan struct{}, 1),
		welcomeCh: make(chan struct{}, 1),
	}
	s.welcome.Epoch = nextEpoch.Add(1)

	if err := s.connect(ctx); err != nil {
		s.cleanup()
		return nil, err
	}
	s.setState(StateConnected)
	return s, nil
}

func validateConfig(cfg Config) error {
	if cfg.ServerURL == "" {
		return fmt.Errorf("abc: ServerURL is required")
	}
	if cfg.Token == "" {
		return fmt.Errorf("abc: Token is required")
	}
	return nil
}

func applyDefaults(cfg Config) Config {
	if cfg.ClientName == "" {
		cfg.ClientName = "abc"
	}
	if cfg.PublishTrackID == "" {
		cfg.PublishTrackID = "abc-mic"
	}
	if cfg.WelcomeTimeout <= 0 {
		cfg.WelcomeTimeout = 5 * time.Second
	}
	return cfg
}

func (s *Session) connect(ctx context.Context) error {
	wsURL, err := signalingURL(s.cfg.ServerURL, s.cfg.Token)
	if err != nil {
		return err
	}
	s.logger.Info("abc: connecting", "url", RedactURL(wsURL), "host", hostOf(s.cfg.ServerURL))

	wsConn, resp, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		if resp != nil && (resp.StatusCode == 401 || resp.StatusCode == 403) {
			return authErrorFromStatus(resp.StatusCode)
		}
		return fmt.Errorf("abc: opening signaling websocket: %w", redactError(err, s.cfg.Token))
	}
	s.mu.Lock()
	s.wsConn = wsConn
	s.mu.Unlock()

	pc, err := newPeerAPI(s.cfg.DisableMDNS).NewPeerConnection(webrtc.Configuration{
		ICEServers: iceServers(s.cfg),
	})
	if err != nil {
		return fmt.Errorf("abc: creating peer connection: %w", err)
	}
	s.mu.Lock()
	s.peerConn = pc
	s.mu.Unlock()

	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		s.logger.Info("abc: ICE state changed", "state", state.String())
		switch state {
		case webrtc.ICEConnectionStateFailed, webrtc.ICEConnectionStateClosed:
			s.fail(fmt.Errorf("ice %s", state.String()), "ice "+state.String())
		}
	})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		switch state {
		case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed:
			s.fail(fmt.Errorf("peer %s", state.String()), "peer "+state.String())
		}
	})
	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		incoming := IncomingTrack{
			ID:       track.ID(),
			StreamID: track.StreamID(),
			Codec:    codecFromRTP(track.Codec()),
			Pion:     track,
		}
		s.logger.Info("abc: remote track received",
			"track_id", incoming.ID,
			"codec", incoming.Codec.MimeType,
			"clock_rate", incoming.Codec.ClockRate,
			"channels", incoming.Codec.Channels,
		)
		s.dispatchTrack(incoming)
	})
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}
		candidateJSON := candidate.ToJSON()
		candidateBytes, err := jsonMarshal(candidateJSON)
		if err != nil {
			s.logger.Error("abc: failed to marshal ICE candidate", "error", err)
			return
		}
		if err := s.sendSignaling(s.ctx, signalingMessage{Type: "ice", Candidate: candidateBytes}); err != nil {
			s.logger.Error("abc: failed to send ICE candidate", "error", err)
		}
	})

	track, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
		s.cfg.PublishTrackID, s.cfg.PublishTrackID,
	)
	if err != nil {
		return fmt.Errorf("abc: creating publish track: %w", err)
	}
	sender, err := pc.AddTrack(track)
	if err != nil {
		return fmt.Errorf("abc: adding publish track: %w", err)
	}
	s.mu.Lock()
	s.sendTrack = track
	s.mu.Unlock()
	go drainRTCP(sender)

	dc, err := pc.CreateDataChannel("control", &webrtc.DataChannelInit{Ordered: boolPtr(true)})
	if err != nil {
		return fmt.Errorf("abc: creating control data channel: %w", err)
	}
	s.mu.Lock()
	s.controlDC = dc
	s.mu.Unlock()

	controlOpened := make(chan struct{})
	var controlOnce sync.Once
	dc.OnOpen(func() {
		s.logger.Info("abc: control data channel opened")
		controlOnce.Do(func() { close(controlOpened) })
	})
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		s.handleControl(msg.Data)
	})

	go s.readSignalingLoop(s.ctx)

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return fmt.Errorf("abc: creating SDP offer: %w", err)
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		return fmt.Errorf("abc: setting local description: %w", err)
	}
	if err := s.sendSignaling(ctx, signalingMessage{Type: "offer", SDP: offer.SDP}); err != nil {
		return fmt.Errorf("abc: sending SDP offer: %w", err)
	}

	select {
	case <-controlOpened:
	case <-s.done:
		return s.errOr(errSessionClosed)
	case <-ctx.Done():
		return ctx.Err()
	}

	hello, err := encodeHello("abc", s.cfg.ClientName, DefaultHelloCapabilities)
	if err != nil {
		return fmt.Errorf("abc: encode Hello: %w", err)
	}
	if err := s.SendControl(hello); err != nil {
		return fmt.Errorf("abc: sending Hello: %w", err)
	}
	s.logger.Info("abc: Hello sent", "client_name", s.cfg.ClientName)

	if err := s.waitWelcome(ctx); err != nil {
		if s.cfg.RequireWelcome {
			return err
		}
		s.logger.Warn("abc: timeout waiting for Welcome")
	}
	return nil
}

func (s *Session) waitWelcome(ctx context.Context) error {
	if s.gotWelcome.Load() {
		return nil
	}
	deadline, cancel := context.WithTimeout(ctx, s.cfg.WelcomeTimeout)
	defer cancel()
	select {
	case <-s.welcomeCh:
		return nil
	case <-s.done:
		return s.errOr(errSessionClosed)
	case <-deadline.Done():
		if s.gotWelcome.Load() {
			return nil
		}
		return fmt.Errorf("abc: timeout waiting for Welcome")
	}
}

func (s *Session) handleControl(data []byte) {
	msg, err := decodeControlMessage(data)
	if err != nil {
		s.fail(err, "protocol error")
		return
	}
	if msg.Welcome != nil {
		s.mu.Lock()
		s.welcome.PeerID = msg.Welcome.PeerID
		s.welcome.ServerVersion = msg.Welcome.ServerVersion
		s.welcome.AssignedSessionID = msg.Welcome.AssignedSessionID
		welcome := s.welcome
		s.mu.Unlock()
		if s.gotWelcome.CompareAndSwap(false, true) {
			select {
			case s.welcomeCh <- struct{}{}:
			default:
			}
		}
		s.logger.Info("abc: received Welcome",
			"peer_id", welcome.PeerID,
			"server_version", welcome.ServerVersion,
			"assigned_session", welcome.AssignedSessionID,
			"epoch", welcome.Epoch,
		)
	}
	out := *msg
	if out.Welcome != nil {
		s.mu.Lock()
		w := s.welcome
		s.mu.Unlock()
		cp := w
		out.Welcome = &cp
	}
	s.dispatchControl(out)
}

func (s *Session) dispatchTrack(track IncomingTrack) {
	s.mu.Lock()
	fn := s.onTrack
	if fn == nil {
		s.pendingTracks = append(s.pendingTracks, track)
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	fn(track)
}

func (s *Session) dispatchControl(msg ControlMessage) {
	s.mu.Lock()
	fn := s.onControl
	if fn == nil {
		s.pendingControls = append(s.pendingControls, msg)
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	fn(msg)
}

// Welcome returns the Welcome for this epoch.
func (s *Session) Welcome() Welcome {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.welcome
}

// State returns the coarse connection lifecycle state.
func (s *Session) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// NegotiatedCodec returns the SDP-selected send-track codec when known.
func (s *Session) NegotiatedCodec() (Codec, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.codec, s.hasCodec
}

// SendTrack returns the local RTP writer.
func (s *Session) SendTrack() RTPWriter {
	return sendTrackWriter{s: s}
}

type sendTrackWriter struct{ s *Session }

func (w sendTrackWriter) WriteRTP(pkt *rtp.Packet) error {
	if w.s == nil {
		return errNoSendTrack
	}
	w.s.mu.Lock()
	track := w.s.sendTrack
	w.s.mu.Unlock()
	if track == nil {
		return errNoSendTrack
	}
	return track.WriteRTP(pkt)
}

// SendControl sends a raw protobuf-v2 ControlMessage on the reliable channel.
func (s *Session) SendControl(data []byte) error {
	if _, err := decodeControlMessage(data); err != nil {
		s.fail(err, "protocol error")
		return err
	}
	s.mu.Lock()
	dc := s.controlDC
	s.mu.Unlock()
	if dc == nil {
		return errControlNotReady
	}
	deadline := time.Now().Add(5 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		if dc.ReadyState() == webrtc.DataChannelStateOpen {
			last = dc.Send(data)
			if last == nil {
				return nil
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if last != nil {
		return last
	}
	return errControlNotReady
}

// SendAudioControlReport encodes and sends a mixer inventory/apply report.
func (s *Session) SendAudioControlReport(r AudioControlReport) error {
	data, err := encodeControlReport(r)
	if err != nil {
		return fmt.Errorf("abc: encode audio control report: %w", err)
	}
	return s.SendControl(data)
}

// OnTrack registers a callback for authorized remote audio tracks.
func (s *Session) OnTrack(fn func(IncomingTrack)) {
	s.mu.Lock()
	s.onTrack = fn
	pending := s.pendingTracks
	s.pendingTracks = nil
	s.mu.Unlock()
	for _, track := range pending {
		fn(track)
	}
}

// OnControl registers a callback for decoded control messages.
func (s *Session) OnControl(fn func(ControlMessage)) {
	s.mu.Lock()
	s.onControl = fn
	pending := s.pendingControls
	s.pendingControls = nil
	s.mu.Unlock()
	for _, msg := range pending {
		fn(msg)
	}
}

// OnClose registers a callback invoked once when the epoch ends.
func (s *Session) OnClose(fn func(reason string)) {
	s.mu.Lock()
	s.onClose = fn
	s.mu.Unlock()
}

// Done is closed when the session epoch ends.
func (s *Session) Done() <-chan struct{} { return s.done }

// CloseReason is the terminal reason for this epoch, if any.
func (s *Session) CloseReason() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeReason
}

// Err is the terminal error for this epoch, if any.
func (s *Session) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeErr
}

// ICEState returns the current ICE connection state.
func (s *Session) ICEState() ICEState {
	s.mu.Lock()
	pc := s.peerConn
	s.mu.Unlock()
	if pc == nil {
		return ICEStateNew
	}
	return iceStateFromPion(pc.ICEConnectionState())
}

// PeerConnectionState returns the Pion peer connection state. Tests use this
// to observe assignment-change teardown.
func (s *Session) PeerConnectionState() webrtc.PeerConnectionState {
	s.mu.Lock()
	pc := s.peerConn
	s.mu.Unlock()
	if pc == nil {
		return webrtc.PeerConnectionStateClosed
	}
	return pc.ConnectionState()
}

// Close ends the session epoch and releases peer resources.
func (s *Session) Close() error {
	s.fail(nil, "closed")
	return nil
}

func (s *Session) setState(st State) {
	s.mu.Lock()
	s.state = st
	s.mu.Unlock()
}

func (s *Session) fail(err error, reason string) {
	s.closedOnce.Do(func() {
		s.mu.Lock()
		if err != nil {
			s.closeErr = redactError(err, s.cfg.Token)
			s.state = StateFailed
		} else {
			s.state = StateClosed
		}
		if reason == "" {
			reason = "closed"
		}
		s.closeReason = reason
		onClose := s.onClose
		s.mu.Unlock()
		s.cleanup()
		close(s.done)
		if onClose != nil {
			onClose(reason)
		}
	})
}

func (s *Session) cleanup() {
	s.cancel()
	s.mu.Lock()
	dc := s.controlDC
	pc := s.peerConn
	ws := s.wsConn
	s.controlDC = nil
	s.peerConn = nil
	s.wsConn = nil
	s.sendTrack = nil
	s.mu.Unlock()
	if dc != nil {
		_ = dc.Close()
	}
	if pc != nil {
		_ = pc.Close()
	}
	if ws != nil {
		_ = ws.Close(websocket.StatusNormalClosure, "closing")
	}
}

func (s *Session) errOr(fallback error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closeErr != nil {
		return s.closeErr
	}
	return fallback
}

func (s *Session) captureNegotiatedCodec() {
	s.mu.Lock()
	pc := s.peerConn
	s.mu.Unlock()
	if pc == nil {
		return
	}
	for _, sender := range pc.GetSenders() {
		params := sender.GetParameters()
		if len(params.Codecs) == 0 {
			continue
		}
		c := params.Codecs[0]
		s.mu.Lock()
		s.codec = Codec{
			MimeType:    c.MimeType,
			ClockRate:   c.ClockRate,
			Channels:    c.Channels,
			PayloadType: uint8(c.PayloadType),
			SDPFmtpLine: c.SDPFmtpLine,
		}
		s.hasCodec = true
		s.mu.Unlock()
		return
	}
}

func drainRTCP(sender *webrtc.RTPSender) {
	buf := make([]byte, 1500)
	for {
		if _, _, err := sender.Read(buf); err != nil {
			return
		}
	}
}

func codecFromRTP(c webrtc.RTPCodecParameters) Codec {
	return Codec{
		MimeType:    c.MimeType,
		ClockRate:   c.ClockRate,
		Channels:    c.Channels,
		PayloadType: uint8(c.PayloadType),
		SDPFmtpLine: c.SDPFmtpLine,
	}
}

func iceStateFromPion(s webrtc.ICEConnectionState) ICEState {
	switch s {
	case webrtc.ICEConnectionStateNew:
		return ICEStateNew
	case webrtc.ICEConnectionStateChecking:
		return ICEStateChecking
	case webrtc.ICEConnectionStateConnected:
		return ICEStateConnected
	case webrtc.ICEConnectionStateCompleted:
		return ICEStateCompleted
	case webrtc.ICEConnectionStateDisconnected:
		return ICEStateDisconnected
	case webrtc.ICEConnectionStateFailed:
		return ICEStateFailed
	case webrtc.ICEConnectionStateClosed:
		return ICEStateClosed
	default:
		return ICEStateUnknown
	}
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	return u.Host
}

func boolPtr(b bool) *bool { return &b }

func jsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}
