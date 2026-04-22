package mock

import (
	"testing"

	crosstalk "github.com/aleksclark/crosstalk/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPipeWireService_Mock(t *testing.T) {
	var svc PipeWireService
	svc.DiscoverFn = func() ([]crosstalk.Source, []crosstalk.Sink, error) {
		sources := []crosstalk.Source{
			{Name: "test-mic", Type: "audio"},
			{Name: "test-webcam", Type: "video"},
		}
		sinks := []crosstalk.Sink{
			{Name: "test-speakers", Type: "audio"},
		}
		return sources, sinks, nil
	}

	sources, sinks, err := svc.Discover()
	require.NoError(t, err)

	assert.True(t, svc.DiscoverInvoked)
	assert.Len(t, sources, 2)
	assert.Equal(t, "test-mic", sources[0].Name)
	assert.Len(t, sinks, 1)
	assert.Equal(t, "test-speakers", sinks[0].Name)
}

func TestPipeWireService_ImplementsInterface(t *testing.T) {
	var _ crosstalk.PipeWireService = &PipeWireService{}
}
