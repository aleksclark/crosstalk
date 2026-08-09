package crosstalk_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	crosstalk "github.com/aleksclark/crosstalk/server"
)

func TestCanTransitionSession(t *testing.T) {
	legal := [][2]crosstalk.SessionState{
		{crosstalk.SessionWaiting, crosstalk.SessionWaiting},
		{crosstalk.SessionWaiting, crosstalk.SessionActive},
		{crosstalk.SessionActive, crosstalk.SessionActive},
		{crosstalk.SessionActive, crosstalk.SessionDraining},
		{crosstalk.SessionActive, crosstalk.SessionFailed},
		{crosstalk.SessionDraining, crosstalk.SessionDraining},
		{crosstalk.SessionDraining, crosstalk.SessionEnded},
		{crosstalk.SessionDraining, crosstalk.SessionFailed},
		{crosstalk.SessionEnded, crosstalk.SessionEnded},
		{crosstalk.SessionEnded, crosstalk.SessionArchived},
		{crosstalk.SessionArchived, crosstalk.SessionArchived},
		{crosstalk.SessionFailed, crosstalk.SessionFailed},
	}
	for _, pair := range legal {
		require.NoError(t, crosstalk.ValidateSessionTransition(pair[0], pair[1]), "%s -> %s", pair[0], pair[1])
	}

	illegal := [][2]crosstalk.SessionState{
		{crosstalk.SessionWaiting, crosstalk.SessionDraining},
		{crosstalk.SessionWaiting, crosstalk.SessionEnded},
		{crosstalk.SessionWaiting, crosstalk.SessionFailed},
		{crosstalk.SessionWaiting, crosstalk.SessionArchived},
		{crosstalk.SessionActive, crosstalk.SessionWaiting},
		{crosstalk.SessionActive, crosstalk.SessionEnded},
		{crosstalk.SessionActive, crosstalk.SessionArchived},
		{crosstalk.SessionDraining, crosstalk.SessionWaiting},
		{crosstalk.SessionDraining, crosstalk.SessionActive},
		{crosstalk.SessionDraining, crosstalk.SessionArchived},
		{crosstalk.SessionEnded, crosstalk.SessionActive},
		{crosstalk.SessionEnded, crosstalk.SessionFailed},
		{crosstalk.SessionArchived, crosstalk.SessionEnded},
		{crosstalk.SessionFailed, crosstalk.SessionActive},
		{crosstalk.SessionFailed, crosstalk.SessionArchived},
	}
	for _, pair := range illegal {
		err := crosstalk.ValidateSessionTransition(pair[0], pair[1])
		require.Error(t, err, "%s -> %s", pair[0], pair[1])
		assert.True(t, errors.Is(err, crosstalk.ErrInvalidSessionTransition), "%v", err)
	}
}
