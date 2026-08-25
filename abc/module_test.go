package abc

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModuleHasNoProtoGenReplace(t *testing.T) {
	data, err := os.ReadFile("go.mod")
	require.NoError(t, err)
	assert.NotContains(t, string(data), "proto/gen/go",
		"abc must not depend on the gitignored proto/gen/go replace")
}

func TestTransportDoesNotStartAudioDevices(t *testing.T) {
	entries, err := os.ReadDir(".")
	require.NoError(t, err)
	banned := []string{"arecord", "aplay", "pw-record", "pw-cat", "ffmpeg", "pipewire"}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		data, err := os.ReadFile(entry.Name())
		require.NoError(t, err)
		body := strings.ToLower(string(data))
		for _, word := range banned {
			assert.NotContains(t, body, word, "%s must not mention %s", entry.Name(), word)
		}
		assert.NotContains(t, body, "alsa/", "%s must not import an alsa package", entry.Name())
		assert.NotContains(t, body, "exec.command", "%s must not start processes", entry.Name())
	}
}

func TestExternalModuleConsumesABCWithoutProtoReplace(t *testing.T) {
	dir := t.TempDir()
	mod := `module example.com/abc-consumer

go 1.25.5

require github.com/aleksclark/crosstalk/abc v0.0.0

replace github.com/aleksclark/crosstalk/abc => ` + filepath.Dir(mustAbs(t, "go.mod")) + `
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644))
	src := `package consumer_test

import (
	"strings"
	"testing"

	"github.com/aleksclark/crosstalk/abc"
)

func TestPublicAPI(t *testing.T) {
	const token = "sentinel-abc-token-do-not-leak"
	got := abc.RedactURL("ws://host/ws/signaling?token=" + token)
	if strings.Contains(got, token) {
		t.Fatalf("token leaked: %s", got)
	}
	if !abc.IsAuthError(&abc.AuthError{StatusCode: 401}) {
		t.Fatal("expected auth error")
	}
	if len(abc.DefaultHelloCapabilities) == 0 {
		t.Fatal("expected advertised opus capabilities")
	}
}
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "consume_test.go"), []byte(src), 0o644))

	cmd := exec.Command("go", "test", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "external consumer test failed:\n%s", out)

	gomod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	require.NoError(t, err)
	assert.NotContains(t, string(gomod), "proto/gen/go")
}

func mustAbs(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	require.NoError(t, err)
	return abs
}
