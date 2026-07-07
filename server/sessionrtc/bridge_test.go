package sessionrtc

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	crosstalk "github.com/aleksclark/crosstalk/server"
)

// fakeSourceStore is an in-memory SourceService that enforces the real
// UNIQUE(session_id, name) constraint so the concurrency fallback is exercised.
type fakeSourceStore struct {
	mu      sync.Mutex
	byID    map[string]*crosstalk.Source
	creates int
}

func newFakeSourceStore() *fakeSourceStore {
	return &fakeSourceStore{byID: make(map[string]*crosstalk.Source)}
}

func (s *fakeSourceStore) List(_ context.Context, sessionID string) ([]crosstalk.Source, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []crosstalk.Source
	for _, src := range s.byID {
		if src.SessionID == sessionID {
			out = append(out, *src)
		}
	}
	return out, nil
}

func (s *fakeSourceStore) Get(_ context.Context, id string) (*crosstalk.Source, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	src, ok := s.byID[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	cp := *src
	return &cp, nil
}

func (s *fakeSourceStore) Create(_ context.Context, src *crosstalk.Source) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ex := range s.byID {
		if ex.SessionID == src.SessionID && ex.Name == src.Name {
			return fmt.Errorf("UNIQUE constraint: (%s, %s)", src.SessionID, src.Name)
		}
	}
	cp := *src
	s.byID[src.ID] = &cp
	s.creates++
	return nil
}

func (s *fakeSourceStore) Update(_ context.Context, src *crosstalk.Source) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[src.ID]; !ok {
		return fmt.Errorf("not found: %s", src.ID)
	}
	cp := *src
	s.byID[src.ID] = &cp
	return nil
}

func (s *fakeSourceStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byID, id)
	return nil
}

func newTestFwd(store crosstalk.SourceService) *sessionFwd {
	return &sessionFwd{
		sessionID: "sess-1",
		stores:    Stores{Sources: store},
		channels:  make(map[string]*channelHub),
	}
}

// A translator reconnecting with the same identity reuses one durable source
// rather than spawning a new one each time.
func TestEnsureSource_ReusesByIdentity(t *testing.T) {
	store := newFakeSourceStore()
	f := newTestFwd(store)
	ctx := context.Background()

	opts := BridgeOpts{Role: "translator", Identity: "user-123", Label: "alice (translator)"}

	// First connect creates the source (connected).
	first, err := f.ensureSource(ctx, "peer-A", opts)
	require.NoError(t, err)
	assert.True(t, first.Connected)
	assert.Equal(t, "alice (translator)", first.Name)

	// Simulate disconnect marking it offline.
	first.Connected = false
	require.NoError(t, store.Update(ctx, first))

	// Reconnect with a *different* ephemeral peer but the same identity.
	second, err := f.ensureSource(ctx, "peer-B", opts)
	require.NoError(t, err)
	assert.Equal(t, first.ID, second.ID, "same identity must reuse the same source")
	assert.True(t, second.Connected, "reconnect reactivates the source")

	sources, err := store.List(ctx, "sess-1")
	require.NoError(t, err)
	assert.Len(t, sources, 1, "no duplicate sources accumulate across reconnects")
	assert.Equal(t, 1, store.creates)
}

// Distinct identities (different users) get distinct sources.
func TestEnsureSource_DistinctIdentities(t *testing.T) {
	store := newFakeSourceStore()
	f := newTestFwd(store)
	ctx := context.Background()

	a, err := f.ensureSource(ctx, "peer-A", BridgeOpts{Role: "translator", Identity: "user-1", Label: "alice"})
	require.NoError(t, err)
	b, err := f.ensureSource(ctx, "peer-B", BridgeOpts{Role: "translator", Identity: "user-2", Label: "bob"})
	require.NoError(t, err)

	assert.NotEqual(t, a.ID, b.ID)
	sources, err := store.List(ctx, "sess-1")
	require.NoError(t, err)
	assert.Len(t, sources, 2)
}

// With no stable identity the code falls back to per-connection sources,
// preserving the previous behavior for unauthenticated/legacy peers.
func TestEnsureSource_FallsBackToPeerID(t *testing.T) {
	store := newFakeSourceStore()
	f := newTestFwd(store)
	ctx := context.Background()

	a, err := f.ensureSource(ctx, "peer-A", BridgeOpts{Role: "translator"})
	require.NoError(t, err)
	b, err := f.ensureSource(ctx, "peer-B", BridgeOpts{Role: "translator"})
	require.NoError(t, err)

	assert.NotEqual(t, a.ID, b.ID, "distinct peers without identity produce distinct sources")
	assert.Equal(t, "translator peer-A", a.Name)
}

// Concurrent connects sharing one identity must converge on a single source,
// even when a create loses the UNIQUE(session, name) race.
func TestEnsureSource_ConcurrentSameIdentity(t *testing.T) {
	store := newFakeSourceStore()
	f := newTestFwd(store)
	ctx := context.Background()
	opts := BridgeOpts{Role: "translator", Identity: "user-x", Label: "carol"}

	const n = 8
	var wg sync.WaitGroup
	ids := make([]string, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			src, err := f.ensureSource(ctx, fmt.Sprintf("peer-%d", i), opts)
			errs[i] = err
			if src != nil {
				ids[i] = src.ID
			}
		}(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		require.NoError(t, errs[i])
	}
	sources, err := store.List(ctx, "sess-1")
	require.NoError(t, err)
	assert.Len(t, sources, 1, "concurrent connects converge on one source")
}
