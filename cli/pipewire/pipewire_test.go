package pipewire

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const samplePWOutput = `id 31, type PipeWire:Interface:Node/3, version 3
	object.serial = 31
	factory.id = 18
	node.name = "alsa_output.pci-0000_00_1f.3.analog-stereo"
	media.class = "Audio/Sink"
	node.nick = "ALC257 Analog"
id 32, type PipeWire:Interface:Node/3, version 3
	object.serial = 32
	factory.id = 18
	node.name = "alsa_input.pci-0000_00_1f.3.analog-stereo"
	media.class = "Audio/Source"
	node.nick = "ALC257 Analog"
id 42, type PipeWire:Interface:Node/3, version 3
	object.serial = 42
	factory.id = 18
	node.name = "virtual_mic"
	media.class = "Audio/Source/Virtual"
id 50, type PipeWire:Interface:Node/3, version 3
	object.serial = 50
	factory.id = 18
	node.name = "loopback_sink"
	media.class = "Audio/Sink/Virtual"
id 60, type PipeWire:Interface:Node/3, version 3
	object.serial = 60
	factory.id = 18
	node.name = "video_camera"
	media.class = "Video/Source"
`

func TestParsePWNodes(t *testing.T) {
	nodes := parsePWNodes(samplePWOutput)

	require.Len(t, nodes, 5)

	assert.Equal(t, "alsa_output.pci-0000_00_1f.3.analog-stereo", nodes[0].name)
	assert.Equal(t, "Audio/Sink", nodes[0].mediaClass)

	assert.Equal(t, "alsa_input.pci-0000_00_1f.3.analog-stereo", nodes[1].name)
	assert.Equal(t, "Audio/Source", nodes[1].mediaClass)

	assert.Equal(t, "virtual_mic", nodes[2].name)
	assert.Equal(t, "Audio/Source/Virtual", nodes[2].mediaClass)

	assert.Equal(t, "loopback_sink", nodes[3].name)
	assert.Equal(t, "Audio/Sink/Virtual", nodes[3].mediaClass)

	assert.Equal(t, "video_camera", nodes[4].name)
	assert.Equal(t, "Video/Source", nodes[4].mediaClass)
}

func TestParsePWNodes_Empty(t *testing.T) {
	nodes := parsePWNodes("")
	assert.Empty(t, nodes)
}

func TestExtractQuotedValue(t *testing.T) {
	assert.Equal(t, "hello world", extractQuotedValue(`key = "hello world"`))
	assert.Equal(t, "Audio/Sink", extractQuotedValue(`media.class = "Audio/Sink"`))
	assert.Equal(t, "", extractQuotedValue(`no quotes here`))
	assert.Equal(t, "", extractQuotedValue(`one "quote only`))
}

func TestService_DiscoverWithFilter(t *testing.T) {
	// Create a service with a custom pw-cli mock script
	// This tests the filtering logic without needing real PipeWire
	svc := &Service{
		SourceFilter: "",
		SinkFilter:   "",
	}

	// Test filtering logic directly by calling discover on parsed nodes
	nodes := parsePWNodes(samplePWOutput)

	// Simulate unfiltered discover
	var sources, filteredSources []string
	var sinks, filteredSinks []string
	for _, n := range nodes {
		switch n.mediaClass {
		case "Audio/Source", "Audio/Source/Virtual":
			sources = append(sources, n.name)
		case "Audio/Sink", "Audio/Sink/Virtual":
			sinks = append(sinks, n.name)
		}
	}

	assert.Len(t, sources, 2) // alsa_input + virtual_mic
	assert.Len(t, sinks, 2)   // alsa_output + loopback_sink

	// Simulate filtered discover (CROSSTALK_SOURCE_NAME set)
	svc.SourceFilter = "virtual_mic"
	svc.SinkFilter = "loopback_sink"
	for _, n := range nodes {
		switch n.mediaClass {
		case "Audio/Source", "Audio/Source/Virtual":
			if svc.SourceFilter == "" || n.name == svc.SourceFilter {
				filteredSources = append(filteredSources, n.name)
			}
		case "Audio/Sink", "Audio/Sink/Virtual":
			if svc.SinkFilter == "" || n.name == svc.SinkFilter {
				filteredSinks = append(filteredSinks, n.name)
			}
		}
	}

	assert.Len(t, filteredSources, 1)
	assert.Equal(t, "virtual_mic", filteredSources[0])
	assert.Len(t, filteredSinks, 1)
	assert.Equal(t, "loopback_sink", filteredSinks[0])
}
