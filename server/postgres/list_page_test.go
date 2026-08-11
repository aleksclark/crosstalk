package postgres_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	crosstalk "github.com/aleksclark/crosstalk/server"
	"github.com/aleksclark/crosstalk/server/pgtest"
	"github.com/aleksclark/crosstalk/server/postgres"
)

func seedSessions(t *testing.T, store *postgres.SessionStore, n int, nameFn func(i int) string) []crosstalk.Session {
	t.Helper()
	ctx := context.Background()
	out := make([]crosstalk.Session, 0, n)
	base := time.Now().UTC().Add(-time.Duration(n) * time.Minute)
	for i := 0; i < n; i++ {
		s := &crosstalk.Session{
			ID:        ulid.Make().String(),
			Name:      nameFn(i),
			CreatedAt: base.Add(time.Duration(i) * time.Minute),
			UpdatedAt: base.Add(time.Duration(i) * time.Minute),
		}
		require.NoError(t, store.Create(ctx, s))
		out = append(out, *s)
	}
	return out
}

func TestSessionStore_ListPage_MultiPageStableOrder(t *testing.T) {
	db := pgtest.New(t)
	store := postgres.NewSessionStore(db)
	ctx := context.Background()

	seedSessions(t, store, 5, func(i int) string {
		return fmt.Sprintf("Session %02d", i)
	})

	// Default sort created_at desc: highest created_at first.
	page1, err := store.ListPage(ctx, crosstalk.ListQuery{Limit: 2})
	require.NoError(t, err)
	require.Len(t, page1.Items, 2)
	require.NotEmpty(t, page1.NextCursor)
	require.NotNil(t, page1.Total)
	assert.Equal(t, int64(5), *page1.Total)
	assert.True(t, !page1.Items[0].CreatedAt.Before(page1.Items[1].CreatedAt))

	page2, err := store.ListPage(ctx, crosstalk.ListQuery{Limit: 2, Cursor: page1.NextCursor})
	require.NoError(t, err)
	require.Len(t, page2.Items, 2)
	require.NotEmpty(t, page2.NextCursor)

	page3, err := store.ListPage(ctx, crosstalk.ListQuery{Limit: 2, Cursor: page2.NextCursor})
	require.NoError(t, err)
	require.Len(t, page3.Items, 1)
	assert.Empty(t, page3.NextCursor)

	seen := map[string]struct{}{}
	for _, p := range []crosstalk.SessionPage{page1, page2, page3} {
		for _, s := range p.Items {
			_, dup := seen[s.ID]
			assert.False(t, dup, "duplicate id %s", s.ID)
			seen[s.ID] = struct{}{}
		}
	}
	assert.Len(t, seen, 5)
}

func TestSessionStore_ListPage_QueryAndMaxLimit(t *testing.T) {
	db := pgtest.New(t)
	store := postgres.NewSessionStore(db)
	ctx := context.Background()

	seedSessions(t, store, 3, func(i int) string {
		if i == 1 {
			return "Alpha Service"
		}
		return fmt.Sprintf("Other %d", i)
	})

	page, err := store.ListPage(ctx, crosstalk.ListQuery{Q: "alpha", Limit: 25})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "Alpha Service", page.Items[0].Name)
	require.NotNil(t, page.Total)
	assert.Equal(t, int64(1), *page.Total)

	// Hard max 100 even if caller asks for more.
	big, err := store.ListPage(ctx, crosstalk.ListQuery{Limit: 500})
	require.NoError(t, err)
	assert.LessOrEqual(t, len(big.Items), crosstalk.MaxListLimit)
}

func TestSessionStore_ListPage_InvalidSort(t *testing.T) {
	db := pgtest.New(t)
	store := postgres.NewSessionStore(db)
	ctx := context.Background()

	_, err := store.ListPage(ctx, crosstalk.ListQuery{Sort: "password_hash"})
	require.Error(t, err)
	assert.ErrorIs(t, err, crosstalk.ErrInvalidListQuery)
}

func TestSessionStore_ListPage_RestrictToIDsBeforePagination(t *testing.T) {
	db := pgtest.New(t)
	store := postgres.NewSessionStore(db)
	ctx := context.Background()

	all := seedSessions(t, store, 6, func(i int) string {
		return fmt.Sprintf("S%d", i)
	})
	// Assign only 3 of 6.
	allowed := []string{all[0].ID, all[2].ID, all[4].ID}
	page, err := store.ListPage(ctx, crosstalk.ListQuery{
		Limit:         2,
		RestrictToIDs: &allowed,
	})
	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	require.NotNil(t, page.Total)
	assert.Equal(t, int64(3), *page.Total, "total must reflect scoped set only")
	for _, s := range page.Items {
		assert.Contains(t, allowed, s.ID)
	}

	// Empty restrict set → empty page, no leak.
	empty := []string{}
	emptyPage, err := store.ListPage(ctx, crosstalk.ListQuery{Limit: 10, RestrictToIDs: &empty})
	require.NoError(t, err)
	assert.Empty(t, emptyPage.Items)
	require.NotNil(t, emptyPage.Total)
	assert.Equal(t, int64(0), *emptyPage.Total)
	assert.Empty(t, emptyPage.NextCursor)
}

func TestABCStore_ListPage_SessionNameAndScope(t *testing.T) {
	db := pgtest.New(t)
	sessStore := postgres.NewSessionStore(db)
	abcStore := postgres.NewABCStore(db)
	ctx := context.Background()

	s1 := &crosstalk.Session{ID: ulid.Make().String(), Name: "Morning Service"}
	s2 := &crosstalk.Session{ID: ulid.Make().String(), Name: "Evening Service"}
	require.NoError(t, sessStore.Create(ctx, s1))
	require.NoError(t, sessStore.Create(ctx, s2))

	base := time.Now().UTC().Add(-5 * time.Minute)
	for i := 0; i < 5; i++ {
		sid := s1.ID
		if i%2 == 1 {
			sid = s2.ID
		}
		abc := &crosstalk.ABC{
			ID:        ulid.Make().String(),
			Name:      fmt.Sprintf("ABC-%d", i),
			TokenHash: fmt.Sprintf("hash-%d", i),
			SessionID: &sid,
			CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}
		require.NoError(t, abcStore.Create(ctx, abc))
	}
	// Unassigned ABC should never appear under session scope.
	require.NoError(t, abcStore.Create(ctx, &crosstalk.ABC{
		ID: ulid.Make().String(), Name: "Unassigned", TokenHash: "u",
	}))

	// Full admin page includes session_name without N+1 (single call).
	page, err := abcStore.ListPage(ctx, crosstalk.ListQuery{Limit: 10})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(page.Items), 5)
	for _, item := range page.Items {
		if item.SessionID == nil {
			assert.Empty(t, item.SessionName)
			continue
		}
		assert.NotEmpty(t, item.SessionName)
		if *item.SessionID == s1.ID {
			assert.Equal(t, "Morning Service", item.SessionName)
		}
		if *item.SessionID == s2.ID {
			assert.Equal(t, "Evening Service", item.SessionName)
		}
	}

	// Translator scope: only ABCs on s1.
	scope := []string{s1.ID}
	scoped, err := abcStore.ListPage(ctx, crosstalk.ListQuery{Limit: 10, RestrictToIDs: &scope})
	require.NoError(t, err)
	require.NotNil(t, scoped.Total)
	assert.Equal(t, int64(3), *scoped.Total) // i=0,2,4
	for _, item := range scoped.Items {
		require.NotNil(t, item.SessionID)
		assert.Equal(t, s1.ID, *item.SessionID)
		assert.Equal(t, "Morning Service", item.SessionName)
	}
}

func TestABCStore_ListPage_InvalidSortAndPaging(t *testing.T) {
	db := pgtest.New(t)
	store := postgres.NewABCStore(db)
	ctx := context.Background()

	_, err := store.ListPage(ctx, crosstalk.ListQuery{Sort: "token_hash"})
	require.ErrorIs(t, err, crosstalk.ErrInvalidListQuery)

	base := time.Now().UTC().Add(-4 * time.Minute)
	for i := 0; i < 4; i++ {
		require.NoError(t, store.Create(ctx, &crosstalk.ABC{
			ID: ulid.Make().String(), Name: fmt.Sprintf("A%d", i),
			TokenHash: fmt.Sprintf("t%d", i), CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}))
	}
	p1, err := store.ListPage(ctx, crosstalk.ListQuery{Limit: 2})
	require.NoError(t, err)
	require.Len(t, p1.Items, 2)
	require.NotEmpty(t, p1.NextCursor)
	p2, err := store.ListPage(ctx, crosstalk.ListQuery{Limit: 2, Cursor: p1.NextCursor})
	require.NoError(t, err)
	require.Len(t, p2.Items, 2)
	assert.NotEqual(t, p1.Items[0].ID, p2.Items[0].ID)
}

func TestUserStore_ListTranslatorsPage_NamesAndPaging(t *testing.T) {
	db := pgtest.New(t)
	users := postgres.NewUserStore(db)
	sessions := postgres.NewSessionStore(db)
	ctx := context.Background()

	sA := &crosstalk.Session{ID: ulid.Make().String(), Name: "Alpha"}
	sB := &crosstalk.Session{ID: ulid.Make().String(), Name: "Beta"}
	require.NoError(t, sessions.Create(ctx, sA))
	require.NoError(t, sessions.Create(ctx, sB))

	// Admin must not appear in translator page.
	require.NoError(t, users.Create(ctx, &crosstalk.User{
		ID: ulid.Make().String(), Username: "admin", PasswordHash: "x", Role: "admin",
	}))

	base := time.Now().UTC().Add(-5 * time.Minute)
	var trIDs []string
	for i := 0; i < 5; i++ {
		u := &crosstalk.User{
			ID: ulid.Make().String(), Username: fmt.Sprintf("tr%02d", i),
			PasswordHash: "x", Role: "translator",
			CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}
		require.NoError(t, users.Create(ctx, u))
		trIDs = append(trIDs, u.ID)
		if i%2 == 0 {
			require.NoError(t, users.AssignSessions(ctx, u.ID, []string{sA.ID, sB.ID}))
		} else {
			require.NoError(t, users.AssignSessions(ctx, u.ID, []string{sB.ID}))
		}
	}

	page1, err := users.ListTranslatorsPage(ctx, crosstalk.ListQuery{Limit: 2})
	require.NoError(t, err)
	require.Len(t, page1.Items, 2)
	require.NotEmpty(t, page1.NextCursor)
	require.NotNil(t, page1.Total)
	assert.Equal(t, int64(5), *page1.Total)

	for _, item := range page1.Items {
		assert.Equal(t, "translator", item.Role)
		assert.NotEmpty(t, item.SessionIDs)
		for _, sid := range item.SessionIDs {
			name, ok := item.SessionNames[sid]
			require.True(t, ok, "session name missing for %s", sid)
			assert.NotEmpty(t, name)
		}
	}

	// Search by username.
	qPage, err := users.ListTranslatorsPage(ctx, crosstalk.ListQuery{Q: "tr01", Limit: 10})
	require.NoError(t, err)
	require.Len(t, qPage.Items, 1)
	assert.Equal(t, "tr01", qPage.Items[0].Username)

	_, err = users.ListTranslatorsPage(ctx, crosstalk.ListQuery{Sort: "password_hash"})
	require.ErrorIs(t, err, crosstalk.ErrInvalidListQuery)

	// Walk remaining pages.
	seen := map[string]struct{}{}
	for _, it := range page1.Items {
		seen[it.ID] = struct{}{}
	}
	cur := page1.NextCursor
	for cur != "" {
		p, err := users.ListTranslatorsPage(ctx, crosstalk.ListQuery{Limit: 2, Cursor: cur})
		require.NoError(t, err)
		for _, it := range p.Items {
			_, dup := seen[it.ID]
			assert.False(t, dup)
			seen[it.ID] = struct{}{}
		}
		cur = p.NextCursor
	}
	assert.Len(t, seen, 5)
	_ = trIDs
}
