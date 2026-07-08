// Package mock provides in-memory mock implementations of domain service
// interfaces for testing. Mocks use function injection: callers set the
// XxxFn field to control behavior, and check XxxInvoked to verify calls.
package mock

import (
	"context"

	crosstalk "github.com/aleksclark/crosstalk/cli"
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

// WiFiManager is a mock implementation of crosstalk.WiFiManager.
type WiFiManager struct {
	OnlineFn      func(ctx context.Context) bool
	OnlineInvoked bool

	ScanFn      func(ctx context.Context) ([]crosstalk.WiFiNetwork, error)
	ScanInvoked bool

	ConnectFn      func(ctx context.Context, ssid, passphrase string) error
	ConnectInvoked bool

	StartHotspotFn      func(ctx context.Context, ssid, passphrase string) error
	StartHotspotInvoked bool

	StopHotspotFn      func(ctx context.Context) error
	StopHotspotInvoked bool
}

func (m *WiFiManager) Online(ctx context.Context) bool {
	m.OnlineInvoked = true
	return m.OnlineFn(ctx)
}

func (m *WiFiManager) Scan(ctx context.Context) ([]crosstalk.WiFiNetwork, error) {
	m.ScanInvoked = true
	return m.ScanFn(ctx)
}

func (m *WiFiManager) Connect(ctx context.Context, ssid, passphrase string) error {
	m.ConnectInvoked = true
	return m.ConnectFn(ctx, ssid, passphrase)
}

func (m *WiFiManager) StartHotspot(ctx context.Context, ssid, passphrase string) error {
	m.StartHotspotInvoked = true
	return m.StartHotspotFn(ctx, ssid, passphrase)
}

func (m *WiFiManager) StopHotspot(ctx context.Context) error {
	m.StopHotspotInvoked = true
	return m.StopHotspotFn(ctx)
}

// EthernetManager is a mock implementation of crosstalk.EthernetManager.
type EthernetManager struct {
	EnableDHCPFn      func(ctx context.Context) (string, error)
	EnableDHCPInvoked bool

	EthernetStatusFn      func(ctx context.Context) (string, string, bool)
	EthernetStatusInvoked bool
}

func (m *EthernetManager) EnableDHCP(ctx context.Context) (string, error) {
	m.EnableDHCPInvoked = true
	return m.EnableDHCPFn(ctx)
}

func (m *EthernetManager) EthernetStatus(ctx context.Context) (string, string, bool) {
	m.EthernetStatusInvoked = true
	return m.EthernetStatusFn(ctx)
}
