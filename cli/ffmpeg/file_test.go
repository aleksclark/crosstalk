package ffmpeg

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamArgsRealtimeAndCodec(t *testing.T) {
	args := StreamArgs("/tmp/a.wav", "rtp://127.0.0.1:9")
	joined := strings.Join(args, " ")
	assert.Contains(t, args, "-re")
	assert.Contains(t, joined, "libopus")
	assert.Contains(t, joined, "48000")
	assert.Contains(t, joined, "frame_duration")
	assert.Contains(t, joined, "20")
	assert.Contains(t, joined, "/tmp/a.wav")
}

func TestStreamProducesRTP(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	wav := filepath.Join(dir, "tone.wav")
	// 0.4s mono 1kHz tone.
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "sine=frequency=1000:duration=0.4",
		"-ar", "48000", "-ac", "1", wav)
	require.NoError(t, cmd.Run())
	st, err := os.Stat(wav)
	require.NoError(t, err)
	require.Greater(t, st.Size(), int64(100))

	src := &FileSource{}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var n int
	err = src.Stream(ctx, wav, func(packet []byte) error {
		require.GreaterOrEqual(t, len(packet), 12)
		n++
		return nil
	})
	require.NoError(t, err)
	assert.Greater(t, n, 5, "expected multiple RTP packets")
}

func TestStreamCancel(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	wav := filepath.Join(dir, "long.wav")
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=30",
		"-ar", "48000", "-ac", "1", wav)
	require.NoError(t, cmd.Run())

	src := &FileSource{}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()
	err := src.Stream(ctx, wav, func(packet []byte) error { return nil })
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}
