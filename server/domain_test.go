package crosstalk_test

import (
	"testing"

	crosstalk "github.com/anthropics/crosstalk/server"
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
