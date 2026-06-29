package recording_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	crosstalk "github.com/aleksclark/crosstalk/server"
	"github.com/aleksclark/crosstalk/server/recording"
)

func TestRecorder_WriteAndClose(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.ogg")

	rec, err := recording.NewRecorder(filePath)
	require.NoError(t, err)

	// Write a few RTP packets with valid Opus payload
	for i := 0; i < 10; i++ {
		pkt := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    111,
				SequenceNumber: uint16(i),
				Timestamp:      uint32(i * 960),
				SSRC:           12345,
			},
			Payload: makeOpusPayload(),
		}
		err := rec.WriteRTP(pkt)
		require.NoError(t, err)
	}

	err = rec.Close()
	require.NoError(t, err)

	// Verify file exists and has content
	info, err := os.Stat(filePath)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
}

func TestRecorder_WriteAfterClose(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.ogg")

	rec, err := recording.NewRecorder(filePath)
	require.NoError(t, err)

	err = rec.Close()
	require.NoError(t, err)

	// Write after close should fail
	pkt := &rtp.Packet{
		Header: rtp.Header{
			Version:     2,
			PayloadType: 111,
			SSRC:        12345,
		},
		Payload: makeOpusPayload(),
	}
	err = rec.WriteRTP(pkt)
	assert.Error(t, err)
}

func TestRecorder_DoubleClose(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.ogg")

	rec, err := recording.NewRecorder(filePath)
	require.NoError(t, err)

	err = rec.Close()
	require.NoError(t, err)

	// Double close should not error
	err = rec.Close()
	assert.NoError(t, err)
}

func TestManager_StartSourceRecording(t *testing.T) {
	dir := t.TempDir()
	store := &mockRecordingStore{}
	mgr := recording.NewManager(store, dir, testLogger())

	ctx := context.Background()
	recID, rec, err := mgr.StartSourceRecording(ctx, "session-1", "source-1", "Microphone")
	require.NoError(t, err)
	require.NotEmpty(t, recID)
	require.NotNil(t, rec)

	// Write a packet
	pkt := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    111,
			SequenceNumber: 1,
			Timestamp:      960,
			SSRC:           12345,
		},
		Payload: makeOpusPayload(),
	}
	err = rec.WriteRTP(pkt)
	require.NoError(t, err)

	// Stop the recording
	err = mgr.StopRecording(ctx, recID)
	require.NoError(t, err)

	// Verify file exists
	require.Len(t, store.recordings, 1)
	r := store.recordings[0]
	fullPath := filepath.Join(dir, r.FilePath)
	info, err := os.Stat(fullPath)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
	assert.NotNil(t, r.EndedAt)
	assert.Greater(t, r.SizeBytes, int64(0))
}

func TestManager_StartChannelRecording(t *testing.T) {
	dir := t.TempDir()
	store := &mockRecordingStore{}
	mgr := recording.NewManager(store, dir, testLogger())

	ctx := context.Background()
	recID, rec, err := mgr.StartChannelRecording(ctx, "session-1", "channel-1", "Feed English")
	require.NoError(t, err)
	require.NotEmpty(t, recID)
	require.NotNil(t, rec)

	// Write a packet
	pkt := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    111,
			SequenceNumber: 1,
			Timestamp:      960,
			SSRC:           12345,
		},
		Payload: makeOpusPayload(),
	}
	err = rec.WriteRTP(pkt)
	require.NoError(t, err)

	// Stop the recording
	err = mgr.StopRecording(ctx, recID)
	require.NoError(t, err)

	// Verify file in channels subdirectory
	require.Len(t, store.recordings, 1)
	r := store.recordings[0]
	assert.Contains(t, r.FilePath, "channels/")
	assert.Contains(t, r.FilePath, "Feed_English_")

	fullPath := filepath.Join(dir, r.FilePath)
	info, err := os.Stat(fullPath)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
}

func TestManager_StopAll(t *testing.T) {
	dir := t.TempDir()
	store := &mockRecordingStore{}
	mgr := recording.NewManager(store, dir, testLogger())

	ctx := context.Background()

	// Start two recordings in same session
	_, _, err := mgr.StartSourceRecording(ctx, "session-1", "source-1", "Mic1")
	require.NoError(t, err)
	_, _, err = mgr.StartSourceRecording(ctx, "session-1", "source-2", "Mic2")
	require.NoError(t, err)

	// Start one in different session
	_, _, err = mgr.StartSourceRecording(ctx, "session-2", "source-3", "Mic3")
	require.NoError(t, err)

	// Stop all for session-1
	err = mgr.StopAll(ctx, "session-1")
	require.NoError(t, err)

	// session-2 recording should still be active
	var activeCount int
	for _, r := range store.recordings {
		if r.EndedAt == nil {
			activeCount++
		}
	}
	assert.Equal(t, 1, activeCount)
}

func TestManager_GenerateManifest(t *testing.T) {
	dir := t.TempDir()
	store := &mockRecordingStore{}
	mgr := recording.NewManager(store, dir, testLogger())

	ctx := context.Background()

	// Start and stop a recording
	recID, _, err := mgr.StartSourceRecording(ctx, "session-1", "source-1", "Mic1")
	require.NoError(t, err)
	err = mgr.StopRecording(ctx, recID)
	require.NoError(t, err)

	// Generate manifest
	err = mgr.GenerateManifest(ctx, "session-1")
	require.NoError(t, err)

	// Read and verify manifest
	manifestPath := filepath.Join(dir, "session-1", "metadata.json")
	data, err := os.ReadFile(manifestPath)
	require.NoError(t, err)

	var manifest recording.Manifest
	err = json.Unmarshal(data, &manifest)
	require.NoError(t, err)

	assert.Equal(t, "session-1", manifest.SessionID)
	assert.Len(t, manifest.Recordings, 1)
	assert.Equal(t, recID, manifest.Recordings[0].ID)
}

func TestManager_StopRecording_NotActive(t *testing.T) {
	dir := t.TempDir()
	store := &mockRecordingStore{}
	mgr := recording.NewManager(store, dir, testLogger())

	err := mgr.StopRecording(context.Background(), "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not active")
}

// --- Helpers ---

func makeOpusPayload() []byte {
	// A minimal valid Opus packet (TOC byte indicating a mono SILK frame)
	// TOC: config=1 (SILK-only 20ms), s=0 (mono), c=0 (1 frame)
	return []byte{0x08, 0x00, 0x00, 0x00}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// --- Mock Store ---

type mockRecordingStore struct {
	recordings []crosstalk.Recording
}

func (m *mockRecordingStore) Create(_ context.Context, r *crosstalk.Recording) error {
	m.recordings = append(m.recordings, *r)
	return nil
}

func (m *mockRecordingStore) FindBySession(_ context.Context, sessionID string) ([]crosstalk.Recording, error) {
	var result []crosstalk.Recording
	for _, r := range m.recordings {
		if r.SessionID == sessionID {
			result = append(result, r)
		}
	}
	return result, nil
}

func (m *mockRecordingStore) FindByID(_ context.Context, id string) (*crosstalk.Recording, error) {
	for i := range m.recordings {
		if m.recordings[i].ID == id {
			return &m.recordings[i], nil
		}
	}
	return nil, fmt.Errorf("not found: %s", id)
}

func (m *mockRecordingStore) Update(_ context.Context, r *crosstalk.Recording) error {
	for i := range m.recordings {
		if m.recordings[i].ID == r.ID {
			m.recordings[i] = *r
			return nil
		}
	}
	return fmt.Errorf("not found: %s", r.ID)
}

func (m *mockRecordingStore) List(_ context.Context) ([]crosstalk.Recording, error) {
	return m.recordings, nil
}
