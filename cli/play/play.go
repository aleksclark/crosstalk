// Package play orchestrates ct-play authentication, selection, and playback.
// It depends only on interfaces so adapters stay in sibling packages.
package play

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

// Session is a listed session.
type Session struct {
	ID   string
	Name string
}

// Channel is a listed channel.
type Channel struct {
	ID        string
	SessionID string
	Name      string
	Type      string
}

// MediaTicket is a one-time admission ticket.
type MediaTicket struct {
	Token             string
	ExpiresAt         time.Time
	SessionID         string
	Role              string
	ProduceChannelIDs []string
	ListenChannelIDs  []string
	OwnerGeneration   uint64
}

// API is the REST surface play needs.
type API interface {
	Login(ctx context.Context, username, password string) (accessToken string, err error)
	ListSessions(ctx context.Context, accessToken string) ([]Session, error)
	ListChannels(ctx context.Context, accessToken, sessionID string) ([]Channel, error)
	IssueMediaTicket(ctx context.Context, accessToken, sessionID string, produceChannelIDs []string) (*MediaTicket, error)
}

// RTPPacket is a parsed RTP packet from the file source.
type RTPPacket struct {
	Payload []byte
	Header  []byte // full marshaled RTP including header; optional if Writer accepts Payload-only
	Raw     []byte // complete packet bytes when available
}

// FileSource streams RTP packets from an audio file.
type FileSource interface {
	// Stream reads path and invokes write for each RTP packet until EOF or ctx cancel.
	Stream(ctx context.Context, path string, write func(packet []byte) error) error
}

// MediaSink publishes Opus RTP into a session channel.
type MediaSink interface {
	// Connect dials session media with the one-time ticket and becomes ready to accept RTP.
	Connect(ctx context.Context, host, sessionID, ticket string) error
	// WriteRTP writes one complete RTP packet.
	WriteRTP(packet []byte) error
	// Close releases transport resources.
	Close() error
}

// Config is resolved runtime configuration for an invocation.
type Config struct {
	Host      string
	Username  string
	Password  string
	SessionID string
	ChannelID string
}

// Service is the ct-play application service.
type Service struct {
	API        API
	Files      FileSource
	Media      func() MediaSink
	FFmpegPath string // optional override; empty means "ffmpeg" on PATH
	LookPath   func(file string) (string, error)
	Stdout     io.Writer
	Stderr     io.Writer
	Logger     *slog.Logger
}

// ValidateCredentials ensures host/username/password are present without echoing values.
func ValidateCredentials(cfg Config) error {
	if strings.TrimSpace(cfg.Host) == "" {
		return fmt.Errorf("host is required (set --host or CT_PLAY_HOST)")
	}
	if strings.TrimSpace(cfg.Username) == "" {
		return fmt.Errorf("username is required (set --username or CT_PLAY_USERNAME)")
	}
	if strings.TrimSpace(cfg.Password) == "" {
		return fmt.Errorf("password is required (set --password or CT_PLAY_PASSWORD)")
	}
	return nil
}

// ListSessions authenticates and prints assigned sessions as ID\tNAME.
func (s *Service) ListSessions(ctx context.Context, cfg Config) error {
	if err := ValidateCredentials(cfg); err != nil {
		return err
	}
	token, err := s.API.Login(ctx, cfg.Username, cfg.Password)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", redactErr(err))
	}
	sessions, err := s.API.ListSessions(ctx, token)
	if err != nil {
		return fmt.Errorf("list sessions: %w", redactErr(err))
	}
	sort.SliceStable(sessions, func(i, j int) bool {
		if sessions[i].Name != sessions[j].Name {
			return sessions[i].Name < sessions[j].Name
		}
		return sessions[i].ID < sessions[j].ID
	})
	out := s.stdout()
	fmt.Fprintln(out, "ID\tNAME")
	for _, sess := range sessions {
		fmt.Fprintf(out, "%s\t%s\n", sess.ID, sess.Name)
	}
	if len(sessions) == 0 {
		fmt.Fprintln(s.stderr(), "no assigned sessions")
	}
	return nil
}

// ListChannels authenticates and prints channels for the resolved session.
func (s *Service) ListChannels(ctx context.Context, cfg Config) error {
	if err := ValidateCredentials(cfg); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.SessionID) == "" {
		return fmt.Errorf("session is required: run ct-play session, then set --session or CT_PLAY_SESSION_ID")
	}
	token, err := s.API.Login(ctx, cfg.Username, cfg.Password)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", redactErr(err))
	}
	if err := s.verifySessionAssigned(ctx, token, cfg.SessionID); err != nil {
		return err
	}
	channels, err := s.API.ListChannels(ctx, token, cfg.SessionID)
	if err != nil {
		return fmt.Errorf("list channels: %w", redactErr(err))
	}
	sort.SliceStable(channels, func(i, j int) bool {
		if channels[i].Type != channels[j].Type {
			return channels[i].Type < channels[j].Type
		}
		if channels[i].Name != channels[j].Name {
			return channels[i].Name < channels[j].Name
		}
		return channels[i].ID < channels[j].ID
	})
	out := s.stdout()
	fmt.Fprintln(out, "ID\tNAME\tTYPE")
	for _, ch := range channels {
		fmt.Fprintf(out, "%s\t%s\t%s\n", ch.ID, ch.Name, ch.Type)
	}
	if len(channels) == 0 {
		fmt.Fprintln(s.stderr(), "session has no available channels")
	}
	return nil
}

// Selection is a verified session/channel pair.
type Selection struct {
	Session Session
	Channel Channel
}

// ResolveSelection verifies session assignment and that channel belongs to it.
func (s *Service) ResolveSelection(ctx context.Context, token string, cfg Config) (*Selection, error) {
	if strings.TrimSpace(cfg.SessionID) == "" {
		return nil, fmt.Errorf("session is required: run ct-play session, then set --session or CT_PLAY_SESSION_ID")
	}
	if strings.TrimSpace(cfg.ChannelID) == "" {
		return nil, fmt.Errorf("channel is required: run ct-play channel, then set --channel or CT_PLAY_CHANNEL_ID")
	}
	sessions, err := s.API.ListSessions(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", redactErr(err))
	}
	var sess *Session
	for i := range sessions {
		if sessions[i].ID == cfg.SessionID {
			sess = &sessions[i]
			break
		}
	}
	if sess == nil {
		return nil, fmt.Errorf("not authorized for session %s (run ct-play session)", cfg.SessionID)
	}
	channels, err := s.API.ListChannels(ctx, token, cfg.SessionID)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", redactErr(err))
	}
	var ch *Channel
	for i := range channels {
		if channels[i].ID == cfg.ChannelID {
			ch = &channels[i]
			break
		}
	}
	if ch == nil {
		return nil, fmt.Errorf("channel %s not found in session %s (run ct-play channel)", cfg.ChannelID, cfg.SessionID)
	}
	return &Selection{Session: *sess, Channel: *ch}, nil
}

// Play streams an audio file into the selected channel.
func (s *Service) Play(ctx context.Context, cfg Config, filePath string) error {
	if err := ValidateCredentials(cfg); err != nil {
		return err
	}
	if err := s.ensureFFmpeg(); err != nil {
		return err
	}
	if err := ValidateAudioFile(filePath); err != nil {
		return err
	}
	if s.Files == nil {
		return fmt.Errorf("file source is not configured")
	}
	if s.Media == nil {
		return fmt.Errorf("media sink is not configured")
	}

	token, err := s.API.Login(ctx, cfg.Username, cfg.Password)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", redactErr(err))
	}
	sel, err := s.ResolveSelection(ctx, token, cfg)
	if err != nil {
		return err
	}

	// Bounded pre-media retry: at most one remint after a connect failure.
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		ticket, terr := s.API.IssueMediaTicket(ctx, token, sel.Session.ID, []string{sel.Channel.ID})
		if terr != nil {
			return fmt.Errorf("issue media ticket: %w", redactErr(terr))
		}
		if !slices.Contains(ticket.ProduceChannelIDs, sel.Channel.ID) {
			return fmt.Errorf("authenticated role is not authorized to publish to channel %s (%s, type %s)",
				sel.Channel.ID, sel.Channel.Name, sel.Channel.Type)
		}

		sink := s.Media()
		if sink == nil {
			return fmt.Errorf("media sink factory returned nil")
		}
		cerr := sink.Connect(ctx, cfg.Host, sel.Session.ID, ticket.Token)
		if cerr != nil {
			_ = sink.Close()
			lastErr = fmt.Errorf("connect media: %w", redactErr(cerr))
			s.logger().Warn("media connect failed; will remint ticket once if unused",
				"attempt", attempt+1, "error", lastErr.Error())
			continue
		}

		start := time.Now()
		s.logger().Info("playback started",
			"session_id", sel.Session.ID,
			"channel_id", sel.Channel.ID,
			"channel_name", sel.Channel.Name,
			"file", filepath.Base(filePath),
		)
		streamErr := s.Files.Stream(ctx, filePath, func(packet []byte) error {
			return sink.WriteRTP(packet)
		})
		closeErr := sink.Close()
		elapsed := time.Since(start)
		if streamErr != nil {
			return fmt.Errorf("playback failed after %s: %w", elapsed.Round(time.Millisecond), redactErr(streamErr))
		}
		if closeErr != nil {
			return fmt.Errorf("media close: %w", redactErr(closeErr))
		}
		s.logger().Info("playback finished",
			"session_id", sel.Session.ID,
			"channel_id", sel.Channel.ID,
			"elapsed", elapsed.Round(time.Millisecond).String(),
		)
		fmt.Fprintf(s.stderr(), "played %s into %s (%s) in %s\n",
			filepath.Base(filePath), sel.Channel.Name, sel.Channel.ID, elapsed.Round(time.Millisecond))
		return nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("media connection failed")
	}
	return lastErr
}

// ValidateAudioFile checks path is a regular readable .wav or .mp3 file.
func ValidateAudioFile(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("audio file path is required")
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".wav" && ext != ".mp3" {
		return fmt.Errorf("unsupported audio format %q (supported: .wav, .mp3)", ext)
	}
	st, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("audio file not found: %s", path)
		}
		return fmt.Errorf("audio file: %w", err)
	}
	if st.IsDir() {
		return fmt.Errorf("audio path is a directory: %s", path)
	}
	if st.Size() == 0 {
		return fmt.Errorf("audio file is empty: %s", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("audio file not readable: %w", err)
	}
	_ = f.Close()
	return nil
}

func (s *Service) verifySessionAssigned(ctx context.Context, token, sessionID string) error {
	sessions, err := s.API.ListSessions(ctx, token)
	if err != nil {
		return fmt.Errorf("list sessions: %w", redactErr(err))
	}
	for _, sess := range sessions {
		if sess.ID == sessionID {
			return nil
		}
	}
	return fmt.Errorf("not authorized for session %s (run ct-play session)", sessionID)
}

func (s *Service) ensureFFmpeg() error {
	look := s.LookPath
	if look == nil {
		look = defaultLookPath
	}
	name := s.FFmpegPath
	if name == "" {
		name = "ffmpeg"
	}
	if _, err := look(name); err != nil {
		return fmt.Errorf("ffmpeg not found in PATH; install ffmpeg to play audio files")
	}
	return nil
}

func (s *Service) stdout() io.Writer {
	if s.Stdout != nil {
		return s.Stdout
	}
	return os.Stdout
}

func (s *Service) stderr() io.Writer {
	if s.Stderr != nil {
		return s.Stderr
	}
	return os.Stderr
}

func (s *Service) logger() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

// redactErr returns err unchanged but documents the redaction contract for callers.
// API adapters already strip tokens; this keeps message surfaces consistent.
func redactErr(err error) error {
	return err
}

func defaultLookPath(file string) (string, error) {
	return exec.LookPath(file)
}
