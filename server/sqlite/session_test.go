package sqlite_test

import (
	"database/sql"
	"testing"
	"time"

	crosstalk "github.com/aleksclark/crosstalk/server"
	"github.com/aleksclark/crosstalk/server/sqlite"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestSession creates a session template and returns a session referencing it.
func newTestSession(t *testing.T, db *sqlite.DB) *crosstalk.Session {
	t.Helper()
	tmplSvc := &sqlite.SessionTemplateService{DB: db.DB}

	now := time.Now().UTC().Truncate(time.Second)
	tmpl := &crosstalk.SessionTemplate{
		ID:        ulid.Make().String(),
		Name:      "test-template",
		Roles:     []crosstalk.Role{{Name: "agent", MultiClient: false}},
		Mappings:  []crosstalk.Mapping{},
		CreatedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, tmplSvc.CreateTemplate(tmpl))

	return &crosstalk.Session{
		ID:         ulid.Make().String(),
		TemplateID: tmpl.ID,
		Name:       "test-session",
		Status:     crosstalk.SessionWaiting,
		CreatedAt:  now,
	}
}

func TestSessionService_CreateAndFindByID(t *testing.T) {
	db := openTestDB(t)
	svc := &sqlite.SessionService{DB: db.DB}
	session := newTestSession(t, db)

	require.NoError(t, svc.CreateSession(session))

	got, err := svc.FindSessionByID(session.ID)
	require.NoError(t, err)
	assert.Equal(t, session.ID, got.ID)
	assert.Equal(t, session.TemplateID, got.TemplateID)
	assert.Equal(t, session.Name, got.Name)
	assert.Equal(t, crosstalk.SessionWaiting, got.Status)
	assert.Equal(t, session.CreatedAt, got.CreatedAt)
	assert.Nil(t, got.EndedAt)
}

func TestSessionService_FindByID_NotFound(t *testing.T) {
	db := openTestDB(t)
	svc := &sqlite.SessionService{DB: db.DB}

	_, err := svc.FindSessionByID("nonexistent")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestSessionService_ListSessions(t *testing.T) {
	db := openTestDB(t)
	svc := &sqlite.SessionService{DB: db.DB}

	s1 := newTestSession(t, db)
	s2 := newTestSession(t, db)
	s2.CreatedAt = s1.CreatedAt.Add(time.Second)

	require.NoError(t, svc.CreateSession(s1))
	require.NoError(t, svc.CreateSession(s2))

	sessions, err := svc.ListSessions()
	require.NoError(t, err)
	require.Len(t, sessions, 2)
	assert.Equal(t, s1.ID, sessions[0].ID)
	assert.Equal(t, s2.ID, sessions[1].ID)
}

func TestSessionService_ListSessions_Empty(t *testing.T) {
	db := openTestDB(t)
	svc := &sqlite.SessionService{DB: db.DB}

	sessions, err := svc.ListSessions()
	require.NoError(t, err)
	assert.Empty(t, sessions)
}

func TestSessionService_EndSession(t *testing.T) {
	db := openTestDB(t)
	svc := &sqlite.SessionService{DB: db.DB}
	session := newTestSession(t, db)

	require.NoError(t, svc.CreateSession(session))
	require.NoError(t, svc.EndSession(session.ID))

	got, err := svc.FindSessionByID(session.ID)
	require.NoError(t, err)
	assert.Equal(t, crosstalk.SessionEnded, got.Status)
	assert.NotNil(t, got.EndedAt)
}

func TestSessionService_EndSession_NotFound(t *testing.T) {
	db := openTestDB(t)
	svc := &sqlite.SessionService{DB: db.DB}

	err := svc.EndSession("nonexistent")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestSessionService_UpdateSessionStatus(t *testing.T) {
	db := openTestDB(t)
	svc := &sqlite.SessionService{DB: db.DB}
	session := newTestSession(t, db)

	require.NoError(t, svc.CreateSession(session))

	require.NoError(t, svc.UpdateSessionStatus(session.ID, crosstalk.SessionActive))

	got, err := svc.FindSessionByID(session.ID)
	require.NoError(t, err)
	assert.Equal(t, crosstalk.SessionActive, got.Status)
	assert.Nil(t, got.EndedAt)
}

func TestSessionService_UpdateSessionStatus_NotFound(t *testing.T) {
	db := openTestDB(t)
	svc := &sqlite.SessionService{DB: db.DB}

	err := svc.UpdateSessionStatus("nonexistent", crosstalk.SessionActive)
	assert.ErrorIs(t, err, sql.ErrNoRows)
}
