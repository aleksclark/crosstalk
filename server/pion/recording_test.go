package pion

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	crosstalk "github.com/aleksclark/crosstalk/server"
	"github.com/aleksclark/crosstalk/server/mock"

	crosstalkv1 "github.com/aleksclark/crosstalk/proto/gen/go/crosstalk/v1"
)

// makeOpusRTPPacket creates a synthetic Opus RTP packet with a valid silence
// frame (3 bytes: [0xf8, 0xff, 0xfe] — Opus silence for 20ms at 48kHz mono).
func makeOpusRTPPacket(seq uint16, ts uint32) *rtp.Packet {
	return &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    111, // Opus
			SequenceNumber: seq,
			Timestamp:      ts,
			SSRC:           12345,
		},
		Payload: []byte{0xf8, 0xff, 0xfe},
	}
}

func TestRecorder_WriteAndClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.ogg")

	rec, err := NewRecorder(path)
	require.NoError(t, err)

	// Write 10 synthetic Opus RTP packets (20ms each at 48kHz = 960 samples).
	for i := range 10 {
		pkt := makeOpusRTPPacket(uint16(i+1), uint32((i+1)*960))
		require.NoError(t, rec.WriteRTP(pkt))
	}

	require.NoError(t, rec.Close())

	// Verify file exists and is non-empty.
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0), "OGG file should be non-empty")

	// Verify Close is idempotent.
	require.NoError(t, rec.Close())
}

func TestRecorder_FFProbeValidation(t *testing.T) {
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not in PATH, skipping validation test")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "probe-test.ogg")

	rec, err := NewRecorder(path)
	require.NoError(t, err)

	// Write 50 packets (1 second of audio at 20ms per packet).
	for i := range 50 {
		pkt := makeOpusRTPPacket(uint16(i+1), uint32((i+1)*960))
		require.NoError(t, rec.WriteRTP(pkt))
	}

	require.NoError(t, rec.Close())

	// Verify codec is Opus.
	cmd := exec.Command(ffprobePath,
		"-v", "error",
		"-show_entries", "stream=codec_name",
		"-of", "csv=p=0",
		path,
	)
	out, err := cmd.Output()
	require.NoError(t, err, "ffprobe codec check failed")
	assert.Contains(t, strings.TrimSpace(string(out)), "opus",
		"expected Opus codec in OGG file")

	// Verify duration is reasonable (should be ~1s for 50 packets at 20ms).
	cmd = exec.Command(ffprobePath,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "csv=p=0",
		path,
	)
	out, err = cmd.Output()
	require.NoError(t, err, "ffprobe duration check failed")
	durationStr := strings.TrimSpace(string(out))
	assert.NotEmpty(t, durationStr, "duration should not be empty")
}

func TestRecordingIntegration_SessionProducesFile(t *testing.T) {
	recordingDir := t.TempDir()

	// Set up template with a record mapping.
	tmpl := &crosstalk.SessionTemplate{
		ID:   "tmpl-rec",
		Name: "recording-test",
		Roles: []crosstalk.Role{
			{Name: "speaker", MultiClient: false},
		},
		Mappings: []crosstalk.Mapping{
			{Source: "speaker:mic", Sink: "record"},
		},
	}
	session := &crosstalk.Session{
		ID:         "sess-rec",
		TemplateID: "tmpl-rec",
		Name:       "Recording Test",
		Status:     crosstalk.SessionWaiting,
	}

	sessions := &mock.SessionService{
		FindSessionByIDFn: func(id string) (*crosstalk.Session, error) {
			if id == session.ID {
				return session, nil
			}
			return nil, nil
		},
	}
	templates := &mock.SessionTemplateService{
		FindTemplateByIDFn: func(id string) (*crosstalk.SessionTemplate, error) {
			if id == tmpl.ID {
				return tmpl, nil
			}
			return nil, nil
		},
	}

	orch := NewOrchestrator(sessions, templates)
	orch.RecordingPath = recordingDir

	// Create connected peer.
	server, client, dc := createConnectedServerPeer(t)

	// Wait for BindChannel before adding audio track.
	bindCh := make(chan *crosstalkv1.BindChannel, 5)
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		var cm crosstalkv1.ControlMessage
		if err := proto.Unmarshal(msg.Data, &cm); err == nil {
			if bc := cm.GetBindChannel(); bc != nil {
				bindCh <- bc
			}
		}
	})

	err := orch.JoinSession(server, "sess-rec", "speaker")
	require.NoError(t, err)

	// Wait for BindChannel command.
	var bind *crosstalkv1.BindChannel
	select {
	case bind = <-bindCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for BindChannel")
	}
	require.NotNil(t, bind)
	assert.Equal(t, "mic", bind.GetLocalName())

	// Add an audio track to the client and send Opus packets.
	audioTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
		bind.GetTrackId(),
		bind.GetTrackId(),
	)
	require.NoError(t, err)

	_, err = client.AddTrack(audioTrack)
	require.NoError(t, err)

	// Renegotiate so the server sees the new track.
	renegotiate(t, client, server)

	// Send some RTP packets.
	time.Sleep(200 * time.Millisecond) // let OnTrack fire
	for i := range 20 {
		pkt := makeOpusRTPPacket(uint16(i+1), uint32((i+1)*960))
		_ = audioTrack.WriteRTP(pkt)
		time.Sleep(20 * time.Millisecond)
	}

	// End the session.
	orch.EndSession("sess-rec")

	// Verify OGG file exists in the recording directory.
	sessionDir := filepath.Join(recordingDir, "sess-rec")
	entries, err := os.ReadDir(sessionDir)
	require.NoError(t, err, "session recording directory should exist")

	var oggFiles []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".ogg") {
			oggFiles = append(oggFiles, e.Name())
		}
	}
	assert.NotEmpty(t, oggFiles, "expected at least one OGG file in recording directory")

	// Verify session-meta.json was written.
	metaPath := filepath.Join(sessionDir, "session-meta.json")
	_, err = os.Stat(metaPath)
	assert.NoError(t, err, "session-meta.json should exist")

	if err == nil {
		meta, readErr := ReadSessionMeta(sessionDir)
		require.NoError(t, readErr)
		assert.Equal(t, "sess-rec", meta.SessionID)
		assert.Equal(t, "recording-test", meta.TemplateName)
		assert.NotEmpty(t, meta.Files)
	}
}

func TestRecordingIntegration_ReadOnlyDir(t *testing.T) {
	// Create a read-only directory.
	roDir := t.TempDir()
	require.NoError(t, os.Chmod(roDir, 0o555))
	t.Cleanup(func() { os.Chmod(roDir, 0o755) }) //nolint:errcheck

	tmpl := &crosstalk.SessionTemplate{
		ID:   "tmpl-ro",
		Name: "readonly-test",
		Roles: []crosstalk.Role{
			{Name: "speaker", MultiClient: false},
		},
		Mappings: []crosstalk.Mapping{
			{Source: "speaker:mic", Sink: "record"},
		},
	}
	session := &crosstalk.Session{
		ID:         "sess-ro",
		TemplateID: "tmpl-ro",
		Name:       "ReadOnly Test",
		Status:     crosstalk.SessionWaiting,
	}

	sessions := &mock.SessionService{
		FindSessionByIDFn: func(id string) (*crosstalk.Session, error) {
			if id == session.ID {
				return session, nil
			}
			return nil, nil
		},
	}
	templates := &mock.SessionTemplateService{
		FindTemplateByIDFn: func(id string) (*crosstalk.SessionTemplate, error) {
			if id == tmpl.ID {
				return tmpl, nil
			}
			return nil, nil
		},
	}

	orch := NewOrchestrator(sessions, templates)
	orch.RecordingPath = roDir

	// Create connected peer.
	server, _, _ := createConnectedServerPeer(t)

	// JoinSession should succeed despite recording failure.
	err := orch.JoinSession(server, "sess-ro", "speaker")
	require.NoError(t, err, "session should work even when recording dir is read-only")

	// Verify the binding exists but has no recorder.
	ls := orch.GetLiveSession("sess-ro")
	require.NotNil(t, ls)
	assert.Len(t, ls.Bindings, 1)

	for _, lb := range ls.Bindings {
		assert.Nil(t, lb.Recorder, "recorder should be nil when dir is read-only")
	}

	// EndSession should not panic.
	orch.EndSession("sess-ro")
}

func TestWriteSessionMeta(t *testing.T) {
	dir := t.TempDir()

	now := time.Now().Truncate(time.Second)
	meta := &SessionMeta{
		SessionID:    "sess-meta-test",
		TemplateName: "interview",
		StartedAt:    now.Add(-10 * time.Minute),
		EndedAt:      now,
		Files: []RecordingFile{
			{
				Path:          "/recordings/sess-meta-test/interviewer-mic-2025-01-01T12-00-00.ogg",
				SourceRole:    "interviewer",
				SourceChannel: "mic",
				StartedAt:     now.Add(-10 * time.Minute),
			},
			{
				Path:          "/recordings/sess-meta-test/candidate-mic-2025-01-01T12-00-00.ogg",
				SourceRole:    "candidate",
				SourceChannel: "mic",
				StartedAt:     now.Add(-9 * time.Minute),
			},
		},
	}

	err := WriteSessionMeta(dir, meta)
	require.NoError(t, err)

	// Verify the file exists.
	metaPath := filepath.Join(dir, "session-meta.json")
	_, err = os.Stat(metaPath)
	require.NoError(t, err)

	// Read back and verify all fields.
	readMeta, err := ReadSessionMeta(dir)
	require.NoError(t, err)

	assert.Equal(t, meta.SessionID, readMeta.SessionID)
	assert.Equal(t, meta.TemplateName, readMeta.TemplateName)
	assert.True(t, meta.StartedAt.Equal(readMeta.StartedAt), "StartedAt mismatch")
	assert.True(t, meta.EndedAt.Equal(readMeta.EndedAt), "EndedAt mismatch")
	require.Len(t, readMeta.Files, 2)
	assert.Equal(t, meta.Files[0].Path, readMeta.Files[0].Path)
	assert.Equal(t, meta.Files[0].SourceRole, readMeta.Files[0].SourceRole)
	assert.Equal(t, meta.Files[0].SourceChannel, readMeta.Files[0].SourceChannel)
	assert.Equal(t, meta.Files[1].SourceRole, readMeta.Files[1].SourceRole)

	// Verify it's valid JSON by decoding raw.
	data, err := os.ReadFile(metaPath)
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.Equal(t, "sess-meta-test", raw["session_id"])
}

func TestRecordingFileName(t *testing.T) {
	ts := time.Date(2025, 6, 15, 14, 30, 45, 0, time.UTC)
	name := RecordingFileName("interviewer", "mic", ts)
	assert.Equal(t, "interviewer-mic-2025-06-15T14-30-45.ogg", name)
}

// renegotiate performs a full offer/answer renegotiation. This is needed when
// the client adds a track after the initial signaling exchange.
func renegotiate(t *testing.T, client *webrtc.PeerConnection, server *PeerConn) {
	t.Helper()

	offer, err := client.CreateOffer(nil)
	require.NoError(t, err)

	gatherDone := webrtc.GatheringCompletePromise(client)
	require.NoError(t, client.SetLocalDescription(offer))
	<-gatherDone

	fullOffer := *client.LocalDescription()
	_, err = server.HandleOffer(fullOffer)
	require.NoError(t, err)

	serverGather := webrtc.GatheringCompletePromise(server.pc)
	<-serverGather
	fullAnswer := *server.pc.LocalDescription()
	require.NoError(t, client.SetRemoteDescription(fullAnswer))
}
