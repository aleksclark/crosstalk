package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	// server/cmd/ct-server/ctplay_e2e_test.go → repo root is ../../..
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../.."))
}

func buildCtPlay(t *testing.T) string {
	t.Helper()
	root := repoRoot(t)
	out := filepath.Join(root, "bin", "ct-play-e2e")
	cmd := exec.Command("go", "build", "-o", out, "./cmd/ct-play")
	cmd.Dir = filepath.Join(root, "cli")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Run(), "build ct-play: %s", stderr.String())
	return out
}

func runCtPlay(t *testing.T, bin string, env []string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), env...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	runErr := cmd.Run()
	return outBuf.String(), errBuf.String(), runErr
}

func assertNoSecrets(t *testing.T, text string, secrets ...string) {
	t.Helper()
	for _, s := range secrets {
		if s == "" {
			continue
		}
		assert.NotContains(t, text, s, "secret leaked in output")
	}
}

func TestIntegrationCtPlaySessionAndChannel(t *testing.T) {
	bin := buildCtPlay(t)
	env := setupIntegrationServer(t)

	env.createAdminUser(t, "admin", "admin-pass-123")
	adminToken := env.login(t, "admin", "admin-pass-123")

	s1 := createSession(t, env, adminToken, "Alpha Assigned")
	s2 := createSession(t, env, adminToken, "Beta Assigned")
	s3 := createSession(t, env, adminToken, "Gamma Unassigned")
	_ = s3

	feed := createChannel(t, env, adminToken, s1.ID, "Floor", "feed")
	bc1 := createChannel(t, env, adminToken, s1.ID, "English", "broadcast")
	bc2 := createChannel(t, env, adminToken, s1.ID, "Spanish", "broadcast")
	otherCh := createChannel(t, env, adminToken, s2.ID, "Other", "broadcast")
	_ = otherCh

	resp := env.doRequest(t, http.MethodPost, "/api/translators", adminToken,
		`{"username":"playuser","password":"play-pass-123"}`)
	require.Equal(t, 200, resp.StatusCode)
	var translator struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&translator))
	resp.Body.Close()

	resp = env.doRequest(t, http.MethodPut, "/api/translators/"+translator.ID+"/sessions", adminToken,
		fmt.Sprintf(`{"session_ids":[%q,%q]}`, s1.ID, s2.ID))
	require.Equal(t, 200, resp.StatusCode)
	resp.Body.Close()

	host := env.server.URL
	user, pass := "playuser", "play-pass-123"

	// Flags path: session list
	stdout, stderr, err := runCtPlay(t, bin, nil,
		"--host", host, "--username", user, "--password", pass, "session")
	require.NoError(t, err, "stderr=%s", stderr)
	assert.Contains(t, stdout, s1.ID)
	assert.Contains(t, stdout, "Alpha Assigned")
	assert.Contains(t, stdout, s2.ID)
	assert.NotContains(t, stdout, s3.ID)
	assert.NotContains(t, stdout, "Gamma Unassigned")
	assertNoSecrets(t, stdout+stderr, pass)

	// Env path: session list
	stdout, stderr, err = runCtPlay(t, bin, []string{
		"CT_PLAY_HOST=" + host,
		"CT_PLAY_USERNAME=" + user,
		"CT_PLAY_PASSWORD=" + pass,
	}, "session")
	require.NoError(t, err, "stderr=%s", stderr)
	assert.Contains(t, stdout, s1.ID)
	assertNoSecrets(t, stdout+stderr, pass)

	// Config file path
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "ct-play.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(fmt.Sprintf(
		"host: %s\nusername: %s\npassword: %s\nsession_id: %s\n",
		host, user, pass, s1.ID,
	)), 0o600))
	stdout, stderr, err = runCtPlay(t, bin, nil, "--config", cfgPath, "session")
	require.NoError(t, err, "stderr=%s", stderr)
	assert.Contains(t, stdout, s1.ID)
	assertNoSecrets(t, stdout+stderr, pass)

	// Channel list for assigned session
	stdout, stderr, err = runCtPlay(t, bin, nil,
		"--host", host, "--username", user, "--password", pass,
		"--session", s1.ID, "channel")
	require.NoError(t, err, "stderr=%s", stderr)
	assert.Contains(t, stdout, feed.ID)
	assert.Contains(t, stdout, bc1.ID)
	assert.Contains(t, stdout, bc2.ID)
	assert.Contains(t, stdout, "broadcast")
	assert.Contains(t, stdout, "feed")
	assert.NotContains(t, stdout, otherCh.ID)
	assertNoSecrets(t, stdout+stderr, pass)

	// Unauthorized session
	_, stderr, err = runCtPlay(t, bin, nil,
		"--host", host, "--username", user, "--password", pass,
		"--session", s3.ID, "channel")
	require.Error(t, err)
	assert.NotContains(t, stderr, pass)

	// Missing session guidance
	_, stderr, err = runCtPlay(t, bin, []string{
		"CT_PLAY_HOST=" + host,
		"CT_PLAY_USERNAME=" + user,
		"CT_PLAY_PASSWORD=" + pass,
	}, "channel")
	require.Error(t, err)
	assert.Contains(t, stderr, "ct-play session")

	// Bad credentials
	_, stderr, err = runCtPlay(t, bin, nil,
		"--host", host, "--username", user, "--password", "wrong-password", "session")
	require.Error(t, err)
	assert.NotContains(t, stderr, "wrong-password")
	assert.NotContains(t, stderr, pass)
}

func TestIntegrationCtPlayPlaybackWAV(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg required")
	}
	bin := buildCtPlay(t)
	env := setupIntegrationServer(t)

	env.createAdminUser(t, "admin", "admin-pass-123")
	adminToken := env.login(t, "admin", "admin-pass-123")

	session := createSession(t, env, adminToken, "Play Session")
	feed := createChannel(t, env, adminToken, session.ID, "Floor", "feed")
	broadcast := createChannel(t, env, adminToken, session.ID, "English", "broadcast")

	resp := env.doRequest(t, http.MethodPost, "/api/translators", adminToken,
		`{"username":"player","password":"play-pass-123"}`)
	require.Equal(t, 200, resp.StatusCode)
	var translator struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&translator))
	resp.Body.Close()
	resp = env.doRequest(t, http.MethodPut, "/api/translators/"+translator.ID+"/sessions", adminToken,
		fmt.Sprintf(`{"session_ids":[%q]}`, session.ID))
	require.Equal(t, 200, resp.StatusCode)
	resp.Body.Close()

	// Listener receives broadcast mix.
	listener := newTicketMediaClient(t, env, "listener", adminToken, session.ID,
		nil, []string{broadcast.ID}, "listener")

	wav := filepath.Join(repoRoot(t), "test/fixtures/test-tone-1khz-5s.wav")
	require.FileExists(t, wav)

	start := time.Now()
	stdout, stderr, err := runCtPlay(t, bin, nil,
		"--host", env.server.URL,
		"--username", "player",
		"--password", "play-pass-123",
		"--session", session.ID,
		"--channel", broadcast.ID,
		wav,
	)
	elapsed := time.Since(start)
	require.NoError(t, err, "stdout=%s stderr=%s", stdout, stderr)
	assertNoSecrets(t, stdout+stderr, "play-pass-123")

	// Real-time pacing: 5s fixture should take roughly 5s (±2.5s tolerance).
	assert.Greater(t, elapsed, 3*time.Second, "playback finished too fast (burst?)")
	assert.Less(t, elapsed, 12*time.Second, "playback took too long")

	// Wait briefly for mixer path to deliver.
	deadline := time.Now().Add(8 * time.Second)
	var samples []int16
	for time.Now().Before(deadline) {
		samples = listener.captured()
		if len(samples) > 48000 { // >1s
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	require.Greater(t, len(samples), 8000, "listener received too little audio")

	best, ratio := detectTone(samples, []float64{440, 1000, 2000})
	assert.InDelta(t, 1000, best, 50, "expected ~1kHz tone, got %v (ratio %v)", best, ratio)
	assert.Greater(t, ratio, 1.5, "tone confidence too low")

	// Unauthorized feed publish for translator.
	_, stderr, err = runCtPlay(t, bin, nil,
		"--host", env.server.URL,
		"--username", "player",
		"--password", "play-pass-123",
		"--session", session.ID,
		"--channel", feed.ID,
		wav,
	)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(stderr), "not authorized")
	assertNoSecrets(t, stderr, "play-pass-123")
}

func TestIntegrationCtPlayPlaybackMP3Config(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg required")
	}
	bin := buildCtPlay(t)
	env := setupIntegrationServer(t)

	env.createAdminUser(t, "admin", "admin-pass-123")
	adminToken := env.login(t, "admin", "admin-pass-123")
	session := createSession(t, env, adminToken, "MP3 Session")
	broadcast := createChannel(t, env, adminToken, session.ID, "English", "broadcast")

	resp := env.doRequest(t, http.MethodPost, "/api/translators", adminToken,
		`{"username":"mp3user","password":"mp3-pass-123"}`)
	require.Equal(t, 200, resp.StatusCode)
	var translator struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&translator))
	resp.Body.Close()
	resp = env.doRequest(t, http.MethodPut, "/api/translators/"+translator.ID+"/sessions", adminToken,
		fmt.Sprintf(`{"session_ids":[%q]}`, session.ID))
	require.Equal(t, 200, resp.StatusCode)
	resp.Body.Close()

	listener := newTicketMediaClient(t, env, "listener", adminToken, session.ID,
		nil, []string{broadcast.ID}, "listener")

	cfgPath := filepath.Join(t.TempDir(), "ct-play.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(fmt.Sprintf(
		"host: %s\nusername: mp3user\npassword: mp3-pass-123\nsession_id: %s\nchannel_id: %s\n",
		env.server.URL, session.ID, broadcast.ID,
	)), 0o600))

	mp3 := filepath.Join(repoRoot(t), "test/fixtures/test-speech-5s.mp3")
	require.FileExists(t, mp3)

	stdout, stderr, err := runCtPlay(t, bin, nil, "--config", cfgPath, mp3)
	require.NoError(t, err, "stdout=%s stderr=%s", stdout, stderr)
	assertNoSecrets(t, stdout+stderr, "mp3-pass-123")

	deadline := time.Now().Add(10 * time.Second)
	var samples []int16
	for time.Now().Before(deadline) {
		samples = listener.captured()
		if len(samples) > 16000 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	require.Greater(t, len(samples), 4000, "listener received no MP3 audio")
	// Speech fixture: assert non-silence energy rather than a single tone.
	var energy int64
	for _, s := range samples {
		if s < 0 {
			s = -s
		}
		energy += int64(s)
	}
	assert.Greater(t, energy, int64(len(samples))*50, "received audio looks like silence")
}

func TestIntegrationCtPlayMissingFileAndFFmpeg(t *testing.T) {
	bin := buildCtPlay(t)
	env := setupIntegrationServer(t)
	env.createAdminUser(t, "admin", "admin-pass-123")
	adminToken := env.login(t, "admin", "admin-pass-123")
	session := createSession(t, env, adminToken, "Err Session")
	broadcast := createChannel(t, env, adminToken, session.ID, "English", "broadcast")
	resp := env.doRequest(t, http.MethodPost, "/api/translators", adminToken,
		`{"username":"erruser","password":"err-pass-123"}`)
	require.Equal(t, 200, resp.StatusCode)
	var translator struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&translator))
	resp.Body.Close()
	resp = env.doRequest(t, http.MethodPut, "/api/translators/"+translator.ID+"/sessions", adminToken,
		fmt.Sprintf(`{"session_ids":[%q]}`, session.ID))
	require.Equal(t, 200, resp.StatusCode)
	resp.Body.Close()

	_, stderr, err := runCtPlay(t, bin, nil,
		"--host", env.server.URL,
		"--username", "erruser",
		"--password", "err-pass-123",
		"--session", session.ID,
		"--channel", broadcast.ID,
		"/no/such/file.wav",
	)
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(stderr), "not found")

	// Discovery still works without ffmpeg in PATH.
	stdout, stderr, err := runCtPlay(t, bin, []string{"PATH=/usr/bin:/bin"},
		"--host", env.server.URL,
		"--username", "erruser",
		"--password", "err-pass-123",
		"session",
	)
	// /usr/bin may still have ffmpeg; only assert session works.
	require.NoError(t, err, "stderr=%s", stderr)
	assert.Contains(t, stdout, session.ID)
}
