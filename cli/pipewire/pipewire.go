// Package pipewire integrates with PipeWire for audio source/sink discovery.
// This package wraps the PipeWire dependency (D-Bus or pw-cli).
//
// The real PipeWire implementation uses pw-cli to enumerate audio nodes.
// For testing without PipeWire, use the mock in cli/mock/.
package pipewire

import (
	"bufio"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	crosstalk "github.com/anthropics/crosstalk/cli"
)

// Service implements crosstalk.PipeWireService using pw-cli.
type Service struct {
	// SourceFilter limits discovery to sources matching this name.
	// Empty string means discover all sources.
	SourceFilter string

	// SinkFilter limits discovery to sinks matching this name.
	// Empty string means discover all sinks.
	SinkFilter string

	// pwCliPath is the path to the pw-cli binary. Defaults to "pw-cli".
	pwCliPath string
}

// NewService creates a new PipeWire service.
func NewService(sourceFilter, sinkFilter string) *Service {
	return &Service{
		SourceFilter: sourceFilter,
		SinkFilter:   sinkFilter,
		pwCliPath:    "pw-cli",
	}
}

// Discover enumerates audio sources and sinks using pw-cli.
func (s *Service) Discover() ([]crosstalk.Source, []crosstalk.Sink, error) {
	nodes, err := s.listNodes()
	if err != nil {
		return nil, nil, fmt.Errorf("listing PipeWire nodes: %w", err)
	}

	var sources []crosstalk.Source
	var sinks []crosstalk.Sink

	for _, node := range nodes {
		switch node.mediaClass {
		case "Audio/Source", "Audio/Source/Virtual":
			src := crosstalk.Source{Name: node.name, Type: "audio"}
			if s.SourceFilter == "" || node.name == s.SourceFilter {
				sources = append(sources, src)
			}
		case "Audio/Sink", "Audio/Sink/Virtual":
			sink := crosstalk.Sink{Name: node.name, Type: "audio"}
			if s.SinkFilter == "" || node.name == s.SinkFilter {
				sinks = append(sinks, sink)
			}
		}
	}

	slog.Info("PipeWire discovery complete",
		"sources", len(sources),
		"sinks", len(sinks),
	)

	return sources, sinks, nil
}

type pwNode struct {
	name       string
	mediaClass string
}

// listNodes runs `pw-cli list-objects Node` and parses the output.
func (s *Service) listNodes() ([]pwNode, error) {
	cmd := exec.Command(s.pwCliPath, "list-objects", "Node")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("running pw-cli: %w", err)
	}

	return parsePWNodes(string(output)), nil
}

// parsePWNodes parses the output of `pw-cli list-objects Node`.
// The output format looks like:
//
//	id 31, type PipeWire:Interface:Node/3, ...
//	  node.name = "alsa_output.pci-0000_00_1f.3.analog-stereo"
//	  media.class = "Audio/Sink"
func parsePWNodes(output string) []pwNode {
	var nodes []pwNode
	var current *pwNode

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "id ") {
			// New node entry
			if current != nil && current.name != "" {
				nodes = append(nodes, *current)
			}
			current = &pwNode{}
			continue
		}

		if current == nil {
			continue
		}

		// Parse key-value properties
		if strings.Contains(line, "node.name") {
			current.name = extractQuotedValue(line)
		}
		if strings.Contains(line, "media.class") {
			current.mediaClass = extractQuotedValue(line)
		}
	}

	// Don't forget last node
	if current != nil && current.name != "" {
		nodes = append(nodes, *current)
	}

	return nodes
}

// extractQuotedValue extracts a quoted value from a line like:
// key = "value"
func extractQuotedValue(line string) string {
	start := strings.Index(line, "\"")
	if start == -1 {
		return ""
	}
	end := strings.LastIndex(line, "\"")
	if end <= start {
		return ""
	}
	return line[start+1 : end]
}
