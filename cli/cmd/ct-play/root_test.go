package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aleksclark/crosstalk/cli/play"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHelpDocumentsEnvAndConfig(t *testing.T) {
	var out, errb bytes.Buffer
	root := newRootCommand(&deps{Stdout: &out, Stderr: &errb})
	root.SetArgs([]string{"--help"})
	require.NoError(t, root.Execute())
	text := out.String() + errb.String()
	assert.Contains(t, text, "CT_PLAY_HOST")
	assert.Contains(t, text, "CT_PLAY_PASSWORD")
	assert.Contains(t, text, "session")
	assert.Contains(t, text, "channel")
}

func TestConfigPrecedenceFlagOverEnvOverFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "ct-play.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
host: http://from-file
username: file-user
password: file-pass
session_id: file-session
channel_id: file-channel
`), 0o600))

	t.Setenv("CT_PLAY_HOST", "http://from-env")
	t.Setenv("CT_PLAY_USERNAME", "env-user")
	t.Setenv("CT_PLAY_PASSWORD", "env-pass")
	t.Setenv("CT_PLAY_SESSION_ID", "env-session")
	t.Setenv("CT_PLAY_CHANNEL_ID", "env-channel")

	var got play.Config
	var out, errb bytes.Buffer
	root := newRootCommand(&deps{
		Stdout: &out,
		Stderr: &errb,
		OnResolved: func(cfg play.Config) {
			got = cfg
		},
		NewService: func(host string) *play.Service {
			return &play.Service{
				API:    &stubAPI{},
				Stdout: &out,
				Stderr: &errb,
			}
		},
	})
	root.SetArgs([]string{
		"--config", cfgPath,
		"--host", "http://from-flag",
		"--username", "flag-user",
		// password left to env
		"--session", "flag-session",
		// channel left to env
		"session",
	})
	require.NoError(t, root.ExecuteContext(context.Background()))

	assert.Equal(t, "http://from-flag", got.Host)
	assert.Equal(t, "flag-user", got.Username)
	assert.Equal(t, "env-pass", got.Password)
	assert.Equal(t, "flag-session", got.SessionID)
	assert.Equal(t, "env-channel", got.ChannelID)
	assert.NotContains(t, out.String()+errb.String(), "env-pass")
	assert.NotContains(t, out.String()+errb.String(), "file-pass")
}

func TestConfigFileOnly(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "ct-play.json")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`{
  "host": "http://cfg",
  "username": "cfg-user",
  "password": "cfg-pass",
  "session_id": "cfg-sess",
  "channel_id": "cfg-ch"
}`), 0o600))

	for _, k := range []string{"CT_PLAY_HOST", "CT_PLAY_USERNAME", "CT_PLAY_PASSWORD", "CT_PLAY_SESSION_ID", "CT_PLAY_CHANNEL_ID"} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}

	var got play.Config
	var out, errb bytes.Buffer
	root := newRootCommand(&deps{
		Stdout: &out,
		Stderr: &errb,
		OnResolved: func(cfg play.Config) {
			got = cfg
		},
		NewService: func(host string) *play.Service {
			return &play.Service{
				API:    &stubAPI{},
				Stdout: &out,
				Stderr: &errb,
			}
		},
	})
	root.SetArgs([]string{"--config", cfgPath, "session"})
	require.NoError(t, root.ExecuteContext(context.Background()))
	assert.Equal(t, "http://cfg", got.Host)
	assert.Equal(t, "cfg-user", got.Username)
	assert.Equal(t, "cfg-pass", got.Password)
	assert.Equal(t, "cfg-sess", got.SessionID)
	assert.Equal(t, "cfg-ch", got.ChannelID)
}

func TestMissingExplicitConfig(t *testing.T) {
	var out, errb bytes.Buffer
	root := newRootCommand(&deps{Stdout: &out, Stderr: &errb})
	root.SetArgs([]string{"--config", "/no/such/ct-play.yaml", "session"})
	err := root.ExecuteContext(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "/no/such/ct-play.yaml")
}

func TestSessionSubcommandNotTreatedAsFile(t *testing.T) {
	var out, errb bytes.Buffer
	called := false
	root := newRootCommand(&deps{
		Stdout: &out,
		Stderr: &errb,
		NewService: func(host string) *play.Service {
			return &play.Service{
				API: &stubAPI{onSessions: func() {
					called = true
				}},
				Stdout: &out,
				Stderr: &errb,
			}
		},
	})
	root.SetArgs([]string{
		"--host", "http://x",
		"--username", "u",
		"--password", "p",
		"session",
	})
	require.NoError(t, root.ExecuteContext(context.Background()))
	assert.True(t, called)
	assert.True(t, strings.HasPrefix(out.String(), "ID\tNAME"))
}

func TestWorldReadableConfigWarns(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "ct-play.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
host: http://x
username: u
password: secret
`), 0o644))

	var out, errb bytes.Buffer
	root := newRootCommand(&deps{
		Stdout: &out,
		Stderr: &errb,
		NewService: func(host string) *play.Service {
			return &play.Service{
				API:    &stubAPI{},
				Stdout: &out,
				Stderr: &errb,
			}
		},
	})
	root.SetArgs([]string{"--config", cfgPath, "session"})
	_ = root.ExecuteContext(context.Background())
	assert.Contains(t, errb.String(), "group/world readable")
	assert.NotContains(t, errb.String(), "secret")
}

type stubAPI struct {
	onSessions func()
}

func (s *stubAPI) Login(ctx context.Context, username, password string) (string, error) {
	return "tok", nil
}

func (s *stubAPI) ListSessions(ctx context.Context, accessToken string) ([]play.Session, error) {
	if s.onSessions != nil {
		s.onSessions()
	}
	return nil, nil
}

func (s *stubAPI) ListChannels(ctx context.Context, accessToken, sessionID string) ([]play.Channel, error) {
	return nil, nil
}

func (s *stubAPI) IssueMediaTicket(ctx context.Context, accessToken, sessionID string, produceChannelIDs []string) (*play.MediaTicket, error) {
	return nil, nil
}
