// Package orchestrator manages live session state and coordinates
// mixer instances with source connections and channel routing.
package orchestrator

import (
	"context"
	"log/slog"
	"sync"

	crosstalk "github.com/aleksclark/crosstalk/server"
	"github.com/aleksclark/crosstalk/server/mixer"
)

// MixStore provides database access for mix configuration.
// This interface allows testing without real SQLite.
type MixStore interface {
	// GetMix returns mix entries for a channel.
	GetMix(ctx context.Context, channelID string) ([]crosstalk.MixEntry, error)
	// SetMix persists mix entries for a channel.
	SetMix(ctx context.Context, channelID string, entries []crosstalk.MixEntry) error
}

// ChannelStore provides database access for channels.
type ChannelStore interface {
	// List returns all channels for a session.
	List(ctx context.Context, sessionID string) ([]crosstalk.Channel, error)
}

// SourceStore provides database access for sources.
type SourceStore interface {
	// List returns all sources for a session.
	List(ctx context.Context, sessionID string) ([]crosstalk.Source, error)
	// Update persists source state changes.
	Update(ctx context.Context, s *crosstalk.Source) error
}

// SinkWriter is called to deliver mixed audio frames to output peers.
type SinkWriter func(channelID string, frame []int16)

// SourceState tracks the live state of a source within a session.
type SourceState struct {
	Source    crosstalk.Source
	Active   bool
	InputIDs map[string]string // channelID -> mixer input ID
}

// ChannelState tracks the live state of a channel's mixer and sinks.
type ChannelState struct {
	Channel crosstalk.Channel
	Mixer   *mixer.Mixer
	Sinks   map[string]SinkWriter // sinkID -> writer
	mu      sync.RWMutex
}

// AddSink adds a sink writer for this channel's output.
func (cs *ChannelState) AddSink(id string, writer SinkWriter) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.Sinks[id] = writer
}

// RemoveSink removes a sink writer.
func (cs *ChannelState) RemoveSink(id string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	delete(cs.Sinks, id)
}

// forwardFrame sends a frame to all sinks for this channel.
func (cs *ChannelState) forwardFrame(frame []int16) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	for _, writer := range cs.Sinks {
		writer(cs.Channel.ID, frame)
	}
}

// SessionOrchestrator manages a single live session.
type SessionOrchestrator struct {
	SessionID string

	mu       sync.RWMutex
	sources  map[string]*SourceState  // sourceID -> state
	channels map[string]*ChannelState // channelID -> state

	mixStore    MixStore
	channelStore ChannelStore
	sourceStore  SourceStore

	logger *slog.Logger
}

// Config holds dependencies for creating a SessionOrchestrator.
type Config struct {
	SessionID    string
	MixStore     MixStore
	ChannelStore ChannelStore
	SourceStore  SourceStore
	Logger       *slog.Logger
}

// New creates a new SessionOrchestrator.
func New(cfg Config) *SessionOrchestrator {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &SessionOrchestrator{
		SessionID:    cfg.SessionID,
		sources:      make(map[string]*SourceState),
		channels:     make(map[string]*ChannelState),
		mixStore:     cfg.MixStore,
		channelStore: cfg.ChannelStore,
		sourceStore:  cfg.SourceStore,
		logger:       logger,
	}
}

// Initialize loads channels and creates mixers for the session.
func (so *SessionOrchestrator) Initialize(ctx context.Context) error {
	channels, err := so.channelStore.List(ctx, so.SessionID)
	if err != nil {
		return err
	}

	so.mu.Lock()
	defer so.mu.Unlock()

	for _, ch := range channels {
		cs := &ChannelState{
			Channel: ch,
			Sinks:   make(map[string]SinkWriter),
		}
		// Create mixer with output forwarding to sinks
		cs.Mixer = mixer.New(func(frame []int16) {
			cs.forwardFrame(frame)
		})
		so.channels[ch.ID] = cs
	}

	return nil
}

// SourceConnect handles a source coming online.
// It creates mixer inputs for all channels based on DB mix state.
func (so *SessionOrchestrator) SourceConnect(ctx context.Context, source crosstalk.Source) error {
	so.mu.Lock()
	defer so.mu.Unlock()

	// Check if source was previously tracked (reconnect scenario)
	state, exists := so.sources[source.ID]
	if exists {
		state.Active = true
		state.Source = source
		so.logger.Info("orchestrator: source reconnected",
			"source_id", source.ID, "name", source.Name)
	} else {
		state = &SourceState{
			Source:   source,
			Active:   true,
			InputIDs: make(map[string]string),
		}
		so.sources[source.ID] = state
		so.logger.Info("orchestrator: source connected",
			"source_id", source.ID, "name", source.Name)
	}

	// For each channel, load mix state and create/update mixer input
	for channelID, cs := range so.channels {
		entries, err := so.mixStore.GetMix(ctx, channelID)
		if err != nil {
			so.logger.Error("orchestrator: failed to load mix",
				"channel_id", channelID, "error", err)
			continue
		}

		// Find the mix entry for this source
		level := 1.0
		muted := false
		for _, entry := range entries {
			if entry.SourceID == source.ID {
				level = entry.Level
				muted = entry.Muted
				break
			}
		}

		// Create mixer input using source ID as the input ID
		inputID := source.ID
		cs.Mixer.AddInput(inputID, level, muted)
		state.InputIDs[channelID] = inputID
	}

	return nil
}

// SourceDisconnect marks a source as inactive without removing state.
// This preserves reconnect state.
func (so *SessionOrchestrator) SourceDisconnect(ctx context.Context, sourceID string) error {
	so.mu.Lock()
	defer so.mu.Unlock()

	state, exists := so.sources[sourceID]
	if !exists {
		return nil // nothing to do
	}

	state.Active = false
	so.logger.Info("orchestrator: source disconnected",
		"source_id", sourceID, "name", state.Source.Name)

	// Update source in DB
	state.Source.Connected = false
	if err := so.sourceStore.Update(ctx, &state.Source); err != nil {
		so.logger.Error("orchestrator: failed to update source state",
			"source_id", sourceID, "error", err)
	}

	return nil
}

// UpdateMix updates mixer input params for a channel in real-time.
func (so *SessionOrchestrator) UpdateMix(ctx context.Context, channelID string, entries []crosstalk.MixEntry) error {
	so.mu.RLock()
	cs, ok := so.channels[channelID]
	so.mu.RUnlock()

	if !ok {
		so.logger.Warn("orchestrator: UpdateMix for unknown channel", "channel_id", channelID)
		return nil
	}

	// Persist to DB
	if err := so.mixStore.SetMix(ctx, channelID, entries); err != nil {
		return err
	}

	// Update mixer inputs in real-time
	for _, entry := range entries {
		if err := cs.Mixer.SetLevel(entry.SourceID, entry.Level); err != nil {
			// Input may not exist yet (source not connected)
			so.logger.Debug("orchestrator: SetLevel skipped (input not found)",
				"source_id", entry.SourceID, "channel_id", channelID)
		}
		if err := cs.Mixer.SetMuted(entry.SourceID, entry.Muted); err != nil {
			so.logger.Debug("orchestrator: SetMuted skipped (input not found)",
				"source_id", entry.SourceID, "channel_id", channelID)
		}
	}

	so.logger.Info("orchestrator: mix updated",
		"channel_id", channelID, "entries", len(entries))
	return nil
}

// WriteAudio writes PCM samples from a source into all channel mixers.
func (so *SessionOrchestrator) WriteAudio(sourceID string, samples []int16) {
	so.mu.RLock()
	state, exists := so.sources[sourceID]
	if !exists || !state.Active {
		so.mu.RUnlock()
		return
	}

	for channelID, inputID := range state.InputIDs {
		cs, ok := so.channels[channelID]
		if ok {
			_ = cs.Mixer.WriteToInput(inputID, samples)
		}
	}
	so.mu.RUnlock()
}

// ForwardOutput adds a sink that receives mixed audio for a channel.
func (so *SessionOrchestrator) ForwardOutput(channelID, sinkID string, writer SinkWriter) {
	so.mu.RLock()
	cs, ok := so.channels[channelID]
	so.mu.RUnlock()

	if !ok {
		so.logger.Warn("orchestrator: ForwardOutput for unknown channel",
			"channel_id", channelID, "sink_id", sinkID)
		return
	}

	cs.AddSink(sinkID, writer)
	so.logger.Info("orchestrator: sink added",
		"channel_id", channelID, "sink_id", sinkID)
}

// RemoveSink removes a sink from a channel.
func (so *SessionOrchestrator) RemoveSink(channelID, sinkID string) {
	so.mu.RLock()
	cs, ok := so.channels[channelID]
	so.mu.RUnlock()

	if !ok {
		return
	}

	cs.RemoveSink(sinkID)
	so.logger.Info("orchestrator: sink removed",
		"channel_id", channelID, "sink_id", sinkID)
}

// StartMixers starts all channel mixers. Call after Initialize.
func (so *SessionOrchestrator) StartMixers() {
	so.mu.RLock()
	defer so.mu.RUnlock()

	for _, cs := range so.channels {
		go cs.Mixer.Run()
	}
	so.logger.Info("orchestrator: mixers started",
		"session_id", so.SessionID, "channels", len(so.channels))
}

// StopMixers stops all channel mixers.
func (so *SessionOrchestrator) StopMixers() {
	so.mu.RLock()
	defer so.mu.RUnlock()

	for _, cs := range so.channels {
		cs.Mixer.Stop()
	}
	so.logger.Info("orchestrator: mixers stopped", "session_id", so.SessionID)
}

// GetSourceState returns the state of a source, or nil if not tracked.
func (so *SessionOrchestrator) GetSourceState(sourceID string) *SourceState {
	so.mu.RLock()
	defer so.mu.RUnlock()
	return so.sources[sourceID]
}

// GetChannelState returns the state of a channel, or nil if not found.
func (so *SessionOrchestrator) GetChannelState(channelID string) *ChannelState {
	so.mu.RLock()
	defer so.mu.RUnlock()
	return so.channels[channelID]
}

// ActiveSourceCount returns the number of currently active sources.
func (so *SessionOrchestrator) ActiveSourceCount() int {
	so.mu.RLock()
	defer so.mu.RUnlock()
	count := 0
	for _, s := range so.sources {
		if s.Active {
			count++
		}
	}
	return count
}

// ChannelCount returns the number of channels.
func (so *SessionOrchestrator) ChannelCount() int {
	so.mu.RLock()
	defer so.mu.RUnlock()
	return len(so.channels)
}
