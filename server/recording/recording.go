// Package recording implements audio recording for CrossTalk sessions.
// It supports per-source and per-channel OGG/Opus recording.
package recording

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4/pkg/media/oggwriter"

	crosstalk "github.com/aleksclark/crosstalk/server"
)

// Recorder wraps an OGG writer with thread-safe RTP packet writing.
type Recorder struct {
	mu       sync.Mutex
	writer   *oggwriter.OggWriter
	filePath string
	closed   bool
}

// NewRecorder creates a new Recorder writing to the given file path.
// The file is created immediately with OGG/Opus headers.
func NewRecorder(filePath string) (*Recorder, error) {
	w, err := oggwriter.New(filePath, 48000, 2)
	if err != nil {
		return nil, fmt.Errorf("create ogg writer: %w", err)
	}
	return &Recorder{
		writer:   w,
		filePath: filePath,
	}, nil
}

// WriteRTP writes an RTP packet to the OGG file. Thread-safe.
func (r *Recorder) WriteRTP(packet *rtp.Packet) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return fmt.Errorf("recorder is closed")
	}
	return r.writer.WriteRTP(packet)
}

// Close finalizes the OGG file. Thread-safe.
func (r *Recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	return r.writer.Close()
}

// FilePath returns the file path of the recording.
func (r *Recorder) FilePath() string {
	return r.filePath
}

// activeRecording holds state for a single in-progress recording.
type activeRecording struct {
	recording *crosstalk.Recording
	recorder  *Recorder
}

// Manager manages active recordings per session.
type Manager struct {
	mu             sync.Mutex
	recordings     map[string]*activeRecording // keyed by recording ID
	store          crosstalk.RecordingService
	recordingsPath string
	log            *slog.Logger
}

// NewManager creates a new recording manager.
func NewManager(store crosstalk.RecordingService, recordingsPath string, log *slog.Logger) *Manager {
	return &Manager{
		recordings:     make(map[string]*activeRecording),
		store:          store,
		recordingsPath: recordingsPath,
		log:            log,
	}
}

// StartSourceRecording begins recording a source's audio to an OGG file.
func (m *Manager) StartSourceRecording(ctx context.Context, sessionID, sourceID, sourceName string) (string, *Recorder, error) {
	timestamp := time.Now().UTC().Format("20060102T150405Z")
	relPath := filepath.Join(sessionID, "sources", fmt.Sprintf("%s_%s.ogg", sanitizeName(sourceName), timestamp))
	fullPath := filepath.Join(m.recordingsPath, relPath)

	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return "", nil, fmt.Errorf("create recording dir: %w", err)
	}

	recorder, err := NewRecorder(fullPath)
	if err != nil {
		return "", nil, err
	}

	rec := &crosstalk.Recording{
		ID:        ulid.Make().String(),
		SessionID: sessionID,
		SourceID:  &sourceID,
		FilePath:  relPath,
		StartedAt: time.Now().UTC(),
	}

	if err := m.store.Create(ctx, rec); err != nil {
		_ = recorder.Close()
		_ = os.Remove(fullPath)
		return "", nil, fmt.Errorf("save recording metadata: %w", err)
	}

	m.mu.Lock()
	m.recordings[rec.ID] = &activeRecording{recording: rec, recorder: recorder}
	m.mu.Unlock()

	m.log.Info("started source recording",
		"recording_id", rec.ID,
		"session_id", sessionID,
		"source_id", sourceID,
		"file", relPath)

	return rec.ID, recorder, nil
}

// StartChannelRecording begins recording a channel's mixed output to an OGG file.
func (m *Manager) StartChannelRecording(ctx context.Context, sessionID, channelID, channelName string) (string, *Recorder, error) {
	timestamp := time.Now().UTC().Format("20060102T150405Z")
	relPath := filepath.Join(sessionID, "channels", fmt.Sprintf("%s_%s.ogg", sanitizeName(channelName), timestamp))
	fullPath := filepath.Join(m.recordingsPath, relPath)

	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return "", nil, fmt.Errorf("create recording dir: %w", err)
	}

	recorder, err := NewRecorder(fullPath)
	if err != nil {
		return "", nil, err
	}

	rec := &crosstalk.Recording{
		ID:        ulid.Make().String(),
		SessionID: sessionID,
		ChannelID: &channelID,
		FilePath:  relPath,
		StartedAt: time.Now().UTC(),
	}

	if err := m.store.Create(ctx, rec); err != nil {
		_ = recorder.Close()
		_ = os.Remove(fullPath)
		return "", nil, fmt.Errorf("save recording metadata: %w", err)
	}

	m.mu.Lock()
	m.recordings[rec.ID] = &activeRecording{recording: rec, recorder: recorder}
	m.mu.Unlock()

	m.log.Info("started channel recording",
		"recording_id", rec.ID,
		"session_id", sessionID,
		"channel_id", channelID,
		"file", relPath)

	return rec.ID, recorder, nil
}

// StopRecording stops a single recording by ID.
func (m *Manager) StopRecording(ctx context.Context, recordingID string) error {
	m.mu.Lock()
	active, ok := m.recordings[recordingID]
	if ok {
		delete(m.recordings, recordingID)
	}
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("recording not active: %s", recordingID)
	}

	if err := active.recorder.Close(); err != nil {
		m.log.Error("error closing recorder", "recording_id", recordingID, "error", err)
	}

	// Get file size
	fullPath := filepath.Join(m.recordingsPath, active.recording.FilePath)
	var sizeBytes int64
	if info, err := os.Stat(fullPath); err == nil {
		sizeBytes = info.Size()
	}

	now := time.Now().UTC()
	active.recording.EndedAt = &now
	active.recording.SizeBytes = sizeBytes

	if err := m.store.Update(ctx, active.recording); err != nil {
		return fmt.Errorf("update recording metadata: %w", err)
	}

	m.log.Info("stopped recording",
		"recording_id", recordingID,
		"size_bytes", sizeBytes)

	return nil
}

// StopAll stops all active recordings for a session.
func (m *Manager) StopAll(ctx context.Context, sessionID string) error {
	m.mu.Lock()
	var toStop []string
	for id, active := range m.recordings {
		if active.recording.SessionID == sessionID {
			toStop = append(toStop, id)
		}
	}
	m.mu.Unlock()

	var lastErr error
	for _, id := range toStop {
		if err := m.StopRecording(ctx, id); err != nil {
			lastErr = err
			m.log.Error("error stopping recording", "recording_id", id, "error", err)
		}
	}
	return lastErr
}

// GetRecorder returns the active recorder for a recording ID, if it exists.
func (m *Manager) GetRecorder(recordingID string) (*Recorder, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	active, ok := m.recordings[recordingID]
	if !ok {
		return nil, false
	}
	return active.recorder, true
}

// RecordingsPath returns the configured base path for recordings.
func (m *Manager) RecordingsPath() string {
	return m.recordingsPath
}

// ManifestEntry represents a single recording in the session manifest.
type ManifestEntry struct {
	ID        string     `json:"id"`
	SourceID  *string    `json:"source_id,omitempty"`
	ChannelID *string    `json:"channel_id,omitempty"`
	FilePath  string     `json:"file_path"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	SizeBytes int64      `json:"size_bytes"`
}

// Manifest represents the metadata.json for a session's recordings.
type Manifest struct {
	SessionID  string          `json:"session_id"`
	GeneratedAt time.Time     `json:"generated_at"`
	Recordings []ManifestEntry `json:"recordings"`
}

// GenerateManifest creates a metadata.json file for a session's recordings.
func (m *Manager) GenerateManifest(ctx context.Context, sessionID string) error {
	recordings, err := m.store.FindBySession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("find recordings: %w", err)
	}

	manifest := Manifest{
		SessionID:   sessionID,
		GeneratedAt: time.Now().UTC(),
		Recordings:  make([]ManifestEntry, len(recordings)),
	}

	for i, r := range recordings {
		manifest.Recordings[i] = ManifestEntry{
			ID:        r.ID,
			SourceID:  r.SourceID,
			ChannelID: r.ChannelID,
			FilePath:  r.FilePath,
			StartedAt: r.StartedAt,
			EndedAt:   r.EndedAt,
			SizeBytes: r.SizeBytes,
		}
	}

	manifestPath := filepath.Join(m.recordingsPath, sessionID, "metadata.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		return fmt.Errorf("create manifest dir: %w", err)
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	m.log.Info("generated manifest", "session_id", sessionID, "path", manifestPath)
	return nil
}

// sanitizeName replaces problematic characters in file names.
func sanitizeName(name string) string {
	result := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			result = append(result, c)
		} else {
			result = append(result, '_')
		}
	}
	if len(result) == 0 {
		return "unnamed"
	}
	return string(result)
}
