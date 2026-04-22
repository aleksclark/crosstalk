// Package mock provides in-memory mock implementations of domain service
// interfaces for testing. Mocks use function injection: callers set the
// XxxFn field to control behavior, and check XxxInvoked to verify calls.
package mock

import (
	crosstalk "github.com/anthropics/crosstalk/cli"
)

// PipeWireService is a mock implementation of crosstalk.PipeWireService.
type PipeWireService struct {
	DiscoverFn      func() ([]crosstalk.Source, []crosstalk.Sink, error)
	DiscoverInvoked bool
}

func (s *PipeWireService) Discover() ([]crosstalk.Source, []crosstalk.Sink, error) {
	s.DiscoverInvoked = true
	return s.DiscoverFn()
}
