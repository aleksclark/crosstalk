package play

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeAPI struct {
	loginErr    error
	sessions    []Session
	channels    map[string][]Channel
	tickets     []*MediaTicket
	ticketCalls int
	listCalls   int
}

func (f *fakeAPI) Login(ctx context.Context, username, password string) (string, error) {
	if f.loginErr != nil {
		return "", f.loginErr
	}
	return "tok", nil
}

func (f *fakeAPI) ListSessions(ctx context.Context, accessToken string) ([]Session, error) {
	f.listCalls++
	return append([]Session(nil), f.sessions...), nil
}

func (f *fakeAPI) ListChannels(ctx context.Context, accessToken, sessionID string) ([]Channel, error) {
	return append([]Channel(nil), f.channels[sessionID]...), nil
}

func (f *fakeAPI) IssueMediaTicket(ctx context.Context, accessToken, sessionID string, produceChannelIDs []string) (*MediaTicket, error) {
	if f.ticketCalls >= len(f.tickets) {
		return nil, errors.New("no more tickets")
	}
	t := f.tickets[f.ticketCalls]
	f.ticketCalls++
	return t, nil
}

type fakeSink struct {
	connectErr error
	packets    int
	closed     bool
}

func (f *fakeSink) Connect(ctx context.Context, host, sessionID, ticket string) error {
	return f.connectErr
}
func (f *fakeSink) WriteRTP(packet []byte) error {
	f.packets++
	return nil
}
func (f *fakeSink) Close() error {
	f.closed = true
	return nil
}

type fakeFiles struct {
	packets int
}

func (f *fakeFiles) Stream(ctx context.Context, path string, write func([]byte) error) error {
	for i := 0; i < f.packets; i++ {
		if err := write([]byte{0x80, 0x6f}); err != nil {
			return err
		}
	}
	return nil
}

func TestListSessionsSorted(t *testing.T) {
	var out bytes.Buffer
	svc := &Service{
		API: &fakeAPI{sessions: []Session{
			{ID: "2", Name: "Beta"},
			{ID: "1", Name: "Alpha"},
			{ID: "3", Name: "Alpha"},
		}},
		Stdout: &out,
		Stderr: ioDiscard{},
	}
	err := svc.ListSessions(context.Background(), Config{
		Host: "http://x", Username: "u", Password: "p",
	})
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	require.Len(t, lines, 4)
	assert.Equal(t, "ID\tNAME", lines[0])
	assert.Equal(t, "1\tAlpha", lines[1])
	assert.Equal(t, "3\tAlpha", lines[2])
	assert.Equal(t, "2\tBeta", lines[3])
}

func TestListChannelsRequiresSession(t *testing.T) {
	svc := &Service{API: &fakeAPI{}}
	err := svc.ListChannels(context.Background(), Config{
		Host: "http://x", Username: "u", Password: "p",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ct-play session")
}

func TestResolveSelectionCrossSession(t *testing.T) {
	svc := &Service{API: &fakeAPI{
		sessions: []Session{{ID: "s1", Name: "A"}},
		channels: map[string][]Channel{
			"s1": {{ID: "c1", SessionID: "s1", Name: "EN", Type: "broadcast"}},
		},
	}}
	_, err := svc.ResolveSelection(context.Background(), "tok", Config{
		SessionID: "s1", ChannelID: "other",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestPlayRejectsUnauthorizedProduce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tone.wav")
	require.NoError(t, os.WriteFile(path, []byte("RIFF...."), 0o600))

	svc := &Service{
		API: &fakeAPI{
			sessions: []Session{{ID: "s1", Name: "A"}},
			channels: map[string][]Channel{
				"s1": {{ID: "feed1", SessionID: "s1", Name: "Floor", Type: "feed"}},
			},
			tickets: []*MediaTicket{{
				Token:             "t1",
				SessionID:         "s1",
				ProduceChannelIDs: []string{}, // not authorized
			}},
		},
		Files: &fakeFiles{packets: 1},
		Media: func() MediaSink { return &fakeSink{} },
		LookPath: func(string) (string, error) { return "/usr/bin/ffmpeg", nil },
		Stderr:   ioDiscard{},
	}
	err := svc.Play(context.Background(), Config{
		Host: "http://x", Username: "u", Password: "p",
		SessionID: "s1", ChannelID: "feed1",
	}, path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not authorized to publish")
}

func TestPlaySuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tone.mp3")
	require.NoError(t, os.WriteFile(path, []byte("ID3...."), 0o600))

	sink := &fakeSink{}
	svc := &Service{
		API: &fakeAPI{
			sessions: []Session{{ID: "s1", Name: "A"}},
			channels: map[string][]Channel{
				"s1": {{ID: "c1", SessionID: "s1", Name: "EN", Type: "broadcast"}},
			},
			tickets: []*MediaTicket{{
				Token: "t1", SessionID: "s1", ProduceChannelIDs: []string{"c1"},
			}},
		},
		Files:    &fakeFiles{packets: 3},
		Media:    func() MediaSink { return sink },
		LookPath: func(string) (string, error) { return "/usr/bin/ffmpeg", nil },
		Stderr:   ioDiscard{},
	}
	err := svc.Play(context.Background(), Config{
		Host: "http://x", Username: "u", Password: "p",
		SessionID: "s1", ChannelID: "c1",
	}, path)
	require.NoError(t, err)
	assert.Equal(t, 3, sink.packets)
	assert.True(t, sink.closed)
}

func TestValidateAudioFile(t *testing.T) {
	dir := t.TempDir()
	wav := filepath.Join(dir, "a.WAV")
	require.NoError(t, os.WriteFile(wav, []byte("x"), 0o600))
	require.NoError(t, ValidateAudioFile(wav))

	require.Error(t, ValidateAudioFile(filepath.Join(dir, "nope.txt")))
	require.Error(t, ValidateAudioFile(dir))
	require.Error(t, ValidateAudioFile(filepath.Join(dir, "missing.mp3")))
}

func TestMissingFFmpeg(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.wav")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))
	svc := &Service{
		API: &fakeAPI{
			sessions: []Session{{ID: "s1", Name: "A"}},
			channels: map[string][]Channel{"s1": {{ID: "c1", SessionID: "s1", Name: "EN", Type: "broadcast"}}},
			tickets:  []*MediaTicket{{Token: "t", ProduceChannelIDs: []string{"c1"}}},
		},
		Files:    &fakeFiles{packets: 1},
		Media:    func() MediaSink { return &fakeSink{} },
		LookPath: func(string) (string, error) { return "", errors.New("missing") },
	}
	err := svc.Play(context.Background(), Config{
		Host: "http://x", Username: "u", Password: "p", SessionID: "s1", ChannelID: "c1",
	}, path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ffmpeg")
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
