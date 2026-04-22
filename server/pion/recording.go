package pion

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4/pkg/media/oggwriter"
)

// Recorder captures Opus RTP packets to an OGG/Opus file on disk.
type Recorder struct {
	writer *oggwriter.OggWriter
	path   string
	mu     sync.Mutex
	closed bool
}

// NewRecorder creates a new recording file at the given path.
// The parent directory must already exist.
func NewRecorder(path string) (*Recorder, error) {
	w, err := oggwriter.New(path, 48000, 1)
	if err != nil {
		return nil, fmt.Errorf("recording: create ogg writer: %w", err)
	}
	return &Recorder{
		writer: w,
		path:   path,
	}, nil
}

// WriteRTP writes a single RTP packet to the OGG file.
// Errors are returned but callers should typically log and continue.
func (r *Recorder) WriteRTP(pkt *rtp.Packet) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return fmt.Errorf("recording: writer is closed")
	}
	return r.writer.WriteRTP(pkt)
}

// Close finalizes the OGG file and closes it. Safe to call multiple times.
func (r *Recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}
	r.closed = true
	return r.writer.Close()
}

// Path returns the file path of the recording.
func (r *Recorder) Path() string {
	return r.path
}

// RecordingFileName builds the filename for a recording file using the pattern:
// <role>-<channel>-<timestamp>.ogg
func RecordingFileName(role, channel string, startedAt time.Time) string {
	ts := startedAt.Format("2006-01-02T15-04-05")
	return fmt.Sprintf("%s-%s-%s.ogg", role, channel, ts)
}

// SessionMeta contains metadata written alongside recording files.
type SessionMeta struct {
	SessionID    string          `json:"session_id"`
	TemplateName string          `json:"template_name"`
	StartedAt    time.Time       `json:"started_at"`
	EndedAt      time.Time       `json:"ended_at"`
	Files        []RecordingFile `json:"files"`
}

// RecordingFile describes a single recording file within a session.
type RecordingFile struct {
	Path          string    `json:"path"`
	SourceRole    string    `json:"source_role"`
	SourceChannel string    `json:"source_channel"`
	StartedAt     time.Time `json:"started_at"`
}

// WriteSessionMeta writes session-meta.json to the given directory.
func WriteSessionMeta(dir string, meta *SessionMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("recording: marshal session meta: %w", err)
	}
	path := filepath.Join(dir, "session-meta.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("recording: write session meta: %w", err)
	}
	return nil
}

// ReadSessionMeta reads and parses session-meta.json from the given directory.
func ReadSessionMeta(dir string) (*SessionMeta, error) {
	path := filepath.Join(dir, "session-meta.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("recording: read session meta: %w", err)
	}
	var meta SessionMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("recording: unmarshal session meta: %w", err)
	}
	return &meta, nil
}
