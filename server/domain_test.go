package crosstalk_test

import (
	"testing"

	crosstalk "github.com/aleksclark/crosstalk/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionStatusConstants(t *testing.T) {
	assert.Equal(t, crosstalk.SessionStatus("waiting"), crosstalk.SessionWaiting)
	assert.Equal(t, crosstalk.SessionStatus("active"), crosstalk.SessionActive)
	assert.Equal(t, crosstalk.SessionStatus("ended"), crosstalk.SessionEnded)
}

func TestSessionTemplate_Validate_MultiClientSourceRejected(t *testing.T) {
	tmpl := &crosstalk.SessionTemplate{
		Roles: []crosstalk.Role{
			{Name: "translator", MultiClient: false},
			{Name: "audience", MultiClient: true},
		},
		Mappings: []crosstalk.Mapping{
			{Source: "audience:mic", Sink: "translator:speakers"},
		},
	}

	err := tmpl.Validate()
	require.Error(t, err)

	var ve *crosstalk.ValidationError
	require.ErrorAs(t, err, &ve)
	assert.Equal(t, "mappings", ve.Field)
	assert.Contains(t, ve.Message, "audience")
}

func TestSessionTemplate_Validate_SingleClientSourceAllowed(t *testing.T) {
	tmpl := &crosstalk.SessionTemplate{
		Roles: []crosstalk.Role{
			{Name: "translator", MultiClient: false},
			{Name: "studio", MultiClient: false},
		},
		Mappings: []crosstalk.Mapping{
			{Source: "translator:mic", Sink: "studio:output"},
			{Source: "translator:mic", Sink: "record"},
			{Source: "translator:mic", Sink: "broadcast"},
		},
	}

	err := tmpl.Validate()
	assert.NoError(t, err)
}

func TestSplitRoleChannel(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantRole    string
		wantChannel string
	}{
		{
			name:        "valid role:channel",
			input:       "translator:mic",
			wantRole:    "translator",
			wantChannel: "mic",
		},
		{
			name:        "no colon returns empty",
			input:       "record",
			wantRole:    "",
			wantChannel: "",
		},
		{
			name:        "multiple colons keeps rest as channel",
			input:       "a:b:c",
			wantRole:    "a",
			wantChannel: "b:c",
		},
		{
			name:        "empty string",
			input:       "",
			wantRole:    "",
			wantChannel: "",
		},
		{
			name:        "colon at start",
			input:       ":channel",
			wantRole:    "",
			wantChannel: "channel",
		},
		{
			name:        "colon at end",
			input:       "role:",
			wantRole:    "role",
			wantChannel: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			role, channel := crosstalk.SplitRoleChannel(tt.input)
			assert.Equal(t, tt.wantRole, role)
			assert.Equal(t, tt.wantChannel, channel)
		})
	}
}

func TestResolveBindings_BothRolesConnected(t *testing.T) {
	tmpl := &crosstalk.SessionTemplate{
		Roles: []crosstalk.Role{
			{Name: "translator"},
			{Name: "studio"},
		},
		Mappings: []crosstalk.Mapping{
			{Source: "translator:mic", Sink: "studio:output"},
		},
	}
	connected := map[string]bool{"translator": true, "studio": true}

	bindings := crosstalk.ResolveBindings(tmpl, connected)

	require.Len(t, bindings, 1)
	assert.Equal(t, "translator", bindings[0].SourceRole)
	assert.Equal(t, "mic", bindings[0].SourceChannel)
	assert.Equal(t, "studio", bindings[0].SinkRole)
	assert.Equal(t, "output", bindings[0].SinkChannel)
	assert.Equal(t, "role", bindings[0].SinkType)
	assert.Equal(t, tmpl.Mappings[0], bindings[0].Mapping)
}

func TestResolveBindings_PartialRoles(t *testing.T) {
	tmpl := &crosstalk.SessionTemplate{
		Roles: []crosstalk.Role{
			{Name: "translator"},
			{Name: "studio"},
		},
		Mappings: []crosstalk.Mapping{
			{Source: "translator:mic", Sink: "studio:output"},
			{Source: "translator:mic", Sink: "record"},
		},
	}
	connected := map[string]bool{"translator": true}

	bindings := crosstalk.ResolveBindings(tmpl, connected)

	require.Len(t, bindings, 1)
	assert.Equal(t, "translator", bindings[0].SourceRole)
	assert.Equal(t, "mic", bindings[0].SourceChannel)
	assert.Equal(t, "record", bindings[0].SinkType)
	assert.Empty(t, bindings[0].SinkRole)
	assert.Empty(t, bindings[0].SinkChannel)
}

func TestResolveBindings_RecordAndBroadcast(t *testing.T) {
	tmpl := &crosstalk.SessionTemplate{
		Roles: []crosstalk.Role{
			{Name: "translator"},
		},
		Mappings: []crosstalk.Mapping{
			{Source: "translator:mic", Sink: "record"},
			{Source: "translator:video", Sink: "broadcast"},
		},
	}
	connected := map[string]bool{"translator": true}

	bindings := crosstalk.ResolveBindings(tmpl, connected)

	require.Len(t, bindings, 2)

	assert.Equal(t, "record", bindings[0].SinkType)
	assert.Equal(t, "translator", bindings[0].SourceRole)
	assert.Equal(t, "mic", bindings[0].SourceChannel)
	assert.Empty(t, bindings[0].SinkRole)
	assert.Empty(t, bindings[0].SinkChannel)

	assert.Equal(t, "broadcast", bindings[1].SinkType)
	assert.Equal(t, "translator", bindings[1].SourceRole)
	assert.Equal(t, "video", bindings[1].SourceChannel)
	assert.Empty(t, bindings[1].SinkRole)
	assert.Empty(t, bindings[1].SinkChannel)
}

func TestResolveBindings_NoRolesConnected(t *testing.T) {
	tmpl := &crosstalk.SessionTemplate{
		Roles: []crosstalk.Role{
			{Name: "translator"},
			{Name: "studio"},
		},
		Mappings: []crosstalk.Mapping{
			{Source: "translator:mic", Sink: "studio:output"},
			{Source: "translator:mic", Sink: "record"},
			{Source: "studio:feed", Sink: "broadcast"},
		},
	}
	connected := map[string]bool{}

	bindings := crosstalk.ResolveBindings(tmpl, connected)

	assert.Empty(t, bindings)
}

func TestResolveBindings_MultipleBindings(t *testing.T) {
	tmpl := &crosstalk.SessionTemplate{
		Roles: []crosstalk.Role{
			{Name: "translator"},
			{Name: "studio"},
			{Name: "monitor"},
		},
		Mappings: []crosstalk.Mapping{
			{Source: "translator:mic", Sink: "studio:output"},     // active: both connected
			{Source: "translator:mic", Sink: "record"},            // active: source connected
			{Source: "translator:mic", Sink: "monitor:speakers"},  // NOT active: monitor not connected
			{Source: "studio:feed", Sink: "broadcast"},            // active: source connected
			{Source: "monitor:aux", Sink: "translator:headphone"}, // NOT active: monitor not connected
		},
	}
	connected := map[string]bool{"translator": true, "studio": true}

	bindings := crosstalk.ResolveBindings(tmpl, connected)

	require.Len(t, bindings, 3)

	// translator:mic → studio:output
	assert.Equal(t, "translator", bindings[0].SourceRole)
	assert.Equal(t, "mic", bindings[0].SourceChannel)
	assert.Equal(t, "studio", bindings[0].SinkRole)
	assert.Equal(t, "output", bindings[0].SinkChannel)
	assert.Equal(t, "role", bindings[0].SinkType)

	// translator:mic → record
	assert.Equal(t, "translator", bindings[1].SourceRole)
	assert.Equal(t, "mic", bindings[1].SourceChannel)
	assert.Equal(t, "record", bindings[1].SinkType)

	// studio:feed → broadcast
	assert.Equal(t, "studio", bindings[2].SourceRole)
	assert.Equal(t, "feed", bindings[2].SourceChannel)
	assert.Equal(t, "broadcast", bindings[2].SinkType)
}
