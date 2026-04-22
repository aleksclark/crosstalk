package sqlite_test

import (
	"database/sql"
	"testing"
	"time"

	crosstalk "github.com/anthropics/crosstalk/server"
	"github.com/anthropics/crosstalk/server/sqlite"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemplateService_CreateAndFindByID(t *testing.T) {
	db := openTestDB(t)
	svc := &sqlite.SessionTemplateService{DB: db.DB}

	now := time.Now().UTC().Truncate(time.Second)
	tmpl := &crosstalk.SessionTemplate{
		ID:        ulid.Make().String(),
		Name:      "two-party",
		IsDefault: false,
		Roles: []crosstalk.Role{
			{Name: "caller", MultiClient: false},
			{Name: "callee", MultiClient: false},
		},
		Mappings: []crosstalk.Mapping{
			{Source: "caller:audio", Sink: "callee:audio"},
			{Source: "callee:audio", Sink: "caller:audio"},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	require.NoError(t, svc.CreateTemplate(tmpl))

	got, err := svc.FindTemplateByID(tmpl.ID)
	require.NoError(t, err)
	assert.Equal(t, tmpl.ID, got.ID)
	assert.Equal(t, tmpl.Name, got.Name)
	assert.Equal(t, tmpl.IsDefault, got.IsDefault)
	assert.Equal(t, tmpl.Roles, got.Roles)
	assert.Equal(t, tmpl.Mappings, got.Mappings)
	assert.Equal(t, now, got.CreatedAt)
	assert.Equal(t, now, got.UpdatedAt)
}

func TestTemplateService_FindByID_NotFound(t *testing.T) {
	db := openTestDB(t)
	svc := &sqlite.SessionTemplateService{DB: db.DB}

	_, err := svc.FindTemplateByID("nonexistent")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestTemplateService_ListTemplates(t *testing.T) {
	db := openTestDB(t)
	svc := &sqlite.SessionTemplateService{DB: db.DB}

	now := time.Now().UTC().Truncate(time.Second)
	t1 := &crosstalk.SessionTemplate{
		ID:        ulid.Make().String(),
		Name:      "template-1",
		Roles:     []crosstalk.Role{{Name: "host", MultiClient: false}},
		Mappings:  []crosstalk.Mapping{},
		CreatedAt: now,
		UpdatedAt: now,
	}
	t2 := &crosstalk.SessionTemplate{
		ID:        ulid.Make().String(),
		Name:      "template-2",
		Roles:     []crosstalk.Role{{Name: "guest", MultiClient: true}},
		Mappings:  []crosstalk.Mapping{},
		CreatedAt: now.Add(time.Second),
		UpdatedAt: now.Add(time.Second),
	}
	require.NoError(t, svc.CreateTemplate(t1))
	require.NoError(t, svc.CreateTemplate(t2))

	templates, err := svc.ListTemplates()
	require.NoError(t, err)
	require.Len(t, templates, 2)
	assert.Equal(t, t1.ID, templates[0].ID)
	assert.Equal(t, t2.ID, templates[1].ID)
}

func TestTemplateService_UpdateTemplate(t *testing.T) {
	db := openTestDB(t)
	svc := &sqlite.SessionTemplateService{DB: db.DB}

	now := time.Now().UTC().Truncate(time.Second)
	tmpl := &crosstalk.SessionTemplate{
		ID:        ulid.Make().String(),
		Name:      "original",
		IsDefault: false,
		Roles:     []crosstalk.Role{{Name: "host", MultiClient: false}},
		Mappings:  []crosstalk.Mapping{},
		CreatedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, svc.CreateTemplate(tmpl))

	tmpl.Name = "updated"
	tmpl.IsDefault = true
	tmpl.Roles = []crosstalk.Role{
		{Name: "host", MultiClient: false},
		{Name: "guest", MultiClient: true},
	}
	tmpl.Mappings = []crosstalk.Mapping{
		{Source: "host:video", Sink: "broadcast"},
	}
	tmpl.UpdatedAt = now.Add(time.Minute)

	require.NoError(t, svc.UpdateTemplate(tmpl))

	got, err := svc.FindTemplateByID(tmpl.ID)
	require.NoError(t, err)
	assert.Equal(t, "updated", got.Name)
	assert.True(t, got.IsDefault)
	assert.Len(t, got.Roles, 2)
	assert.Len(t, got.Mappings, 1)
	assert.Equal(t, now.Add(time.Minute), got.UpdatedAt)
	// CreatedAt should be unchanged.
	assert.Equal(t, now, got.CreatedAt)
}

func TestTemplateService_UpdateTemplate_NotFound(t *testing.T) {
	db := openTestDB(t)
	svc := &sqlite.SessionTemplateService{DB: db.DB}

	now := time.Now().UTC().Truncate(time.Second)
	tmpl := &crosstalk.SessionTemplate{
		ID:        "nonexistent",
		Name:      "ghost",
		Roles:     []crosstalk.Role{},
		Mappings:  []crosstalk.Mapping{},
		UpdatedAt: now,
	}
	err := svc.UpdateTemplate(tmpl)
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestTemplateService_DeleteTemplate(t *testing.T) {
	db := openTestDB(t)
	svc := &sqlite.SessionTemplateService{DB: db.DB}

	now := time.Now().UTC().Truncate(time.Second)
	tmpl := &crosstalk.SessionTemplate{
		ID:        ulid.Make().String(),
		Name:      "doomed",
		Roles:     []crosstalk.Role{},
		Mappings:  []crosstalk.Mapping{},
		CreatedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, svc.CreateTemplate(tmpl))
	require.NoError(t, svc.DeleteTemplate(tmpl.ID))

	_, err := svc.FindTemplateByID(tmpl.ID)
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestTemplateService_DeleteTemplate_NotFound(t *testing.T) {
	db := openTestDB(t)
	svc := &sqlite.SessionTemplateService{DB: db.DB}

	err := svc.DeleteTemplate("nonexistent")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestTemplateService_FindDefaultTemplate(t *testing.T) {
	db := openTestDB(t)
	svc := &sqlite.SessionTemplateService{DB: db.DB}

	now := time.Now().UTC().Truncate(time.Second)
	nonDefault := &crosstalk.SessionTemplate{
		ID:        ulid.Make().String(),
		Name:      "non-default",
		IsDefault: false,
		Roles:     []crosstalk.Role{},
		Mappings:  []crosstalk.Mapping{},
		CreatedAt: now,
		UpdatedAt: now,
	}
	defaultTmpl := &crosstalk.SessionTemplate{
		ID:        ulid.Make().String(),
		Name:      "default",
		IsDefault: true,
		Roles:     []crosstalk.Role{{Name: "host", MultiClient: false}},
		Mappings:  []crosstalk.Mapping{},
		CreatedAt: now.Add(time.Second),
		UpdatedAt: now.Add(time.Second),
	}
	require.NoError(t, svc.CreateTemplate(nonDefault))
	require.NoError(t, svc.CreateTemplate(defaultTmpl))

	got, err := svc.FindDefaultTemplate()
	require.NoError(t, err)
	assert.Equal(t, defaultTmpl.ID, got.ID)
	assert.True(t, got.IsDefault)
}

func TestTemplateService_FindDefaultTemplate_NotFound(t *testing.T) {
	db := openTestDB(t)
	svc := &sqlite.SessionTemplateService{DB: db.DB}

	_, err := svc.FindDefaultTemplate()
	assert.ErrorIs(t, err, sql.ErrNoRows)
}
