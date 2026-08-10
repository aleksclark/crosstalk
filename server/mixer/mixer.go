package mixer

import (
	"math"
	"sync"
	"time"
)

// Input represents a single audio source feeding into the mixer.
type Input struct {
	ID     string
	Muted  bool
	Level  float64 // 0.0 - 2.0, default 1.0
	Buffer *RingBuffer
}

// OutputFunc is called each time the mixer produces a mixed frame.
// The frame is FrameSize (960) samples of PCM int16 at 48kHz.
type OutputFunc func(frame []int16)

// MixStats captures per-tick mix telemetry for observers.
type MixStats struct {
	ActiveSources int
	Peak          float64 // peak absolute sample as fraction of full scale (0..1+)
	Clipped       bool
}

// StatsFunc is an optional per-tick callback (non-blocking expected).
type StatsFunc func(stats MixStats)

// Mixer manages N input streams and produces 1 mixed output stream.
// It runs a mix loop at 20ms intervals (960 samples per frame at 48kHz).
type Mixer struct {
	mu      sync.RWMutex
	inputs  map[string]*Input
	output  OutputFunc
	stats   StatsFunc
	stopCh  chan struct{}
	stopped bool

	// ringSize is the capacity (in samples) for each input's ring buffer.
	ringSize int
}

// Option configures the Mixer.
type Option func(*Mixer)

// WithRingSize sets the ring buffer size per input (in samples).
// Default is 4800 (100ms at 48kHz).
func WithRingSize(size int) Option {
	return func(m *Mixer) {
		m.ringSize = size
	}
}

// WithStats sets a per-tick stats callback (peak, active sources).
func WithStats(fn StatsFunc) Option {
	return func(m *Mixer) {
		m.stats = fn
	}
}

// New creates a new Mixer with the given output callback.
func New(output OutputFunc, opts ...Option) *Mixer {
	m := &Mixer{
		inputs:   make(map[string]*Input),
		output:   output,
		stopCh:   make(chan struct{}),
		ringSize: 4800, // 100ms at 48kHz
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// AddInput adds a new input to the mixer. Thread-safe.
// If an input with the same id already exists, its level/mute are updated and
// the existing input is returned (buffers are preserved).
func (m *Mixer) AddInput(id string, level float64, muted bool) *Input {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.inputs[id]; ok {
		existing.Level = level
		existing.Muted = muted
		return existing
	}
	inp := &Input{
		ID:     id,
		Muted:  muted,
		Level:  level,
		Buffer: NewRingBuffer(m.ringSize),
	}
	m.inputs[id] = inp
	return inp
}

// RemoveInput removes an input from the mixer. Thread-safe.
func (m *Mixer) RemoveInput(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.inputs[id]; !ok {
		return ErrInputNotFound
	}
	delete(m.inputs, id)
	return nil
}

// GetInput returns an input by ID, or nil if not found.
func (m *Mixer) GetInput(id string) *Input {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.inputs[id]
}

// SetLevel updates the level for an input. Thread-safe.
func (m *Mixer) SetLevel(id string, level float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	inp, ok := m.inputs[id]
	if !ok {
		return ErrInputNotFound
	}
	inp.Level = level
	return nil
}

// SetMuted updates the mute state for an input. Thread-safe.
func (m *Mixer) SetMuted(id string, muted bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	inp, ok := m.inputs[id]
	if !ok {
		return ErrInputNotFound
	}
	inp.Muted = muted
	return nil
}

// Snapshot returns a copy of current level/mute state keyed by input id.
func (m *Mixer) Snapshot() map[string]MixEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]MixEntry, len(m.inputs))
	for id, inp := range m.inputs {
		out[id] = MixEntry{ID: id, Level: inp.Level, Muted: inp.Muted}
	}
	return out
}

// MixEntry is a level/mute snapshot for one input.
type MixEntry struct {
	ID    string
	Level float64
	Muted bool
}

// Run starts the mix loop. It blocks until Stop is called.
func (m *Mixer) Run() {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	frame := make([]int16, FrameSize)
	scratch := make([]int16, FrameSize)

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			stats := m.mixFrame(frame, scratch)
			if m.output != nil {
				m.output(frame)
			}
			if m.stats != nil {
				m.stats(stats)
			}
		}
	}
}

// MixOnce performs a single mix iteration (useful for testing without timers).
// Returns mix stats for the tick.
func (m *Mixer) MixOnce(frame []int16) MixStats {
	scratch := make([]int16, FrameSize)
	stats := m.mixFrame(frame, scratch)
	if m.stats != nil {
		m.stats(stats)
	}
	return stats
}

// mixFrame reads from all inputs, scales by level, sums with int32 accum,
// applies a clip-guard limiter, and writes int16 output.
func (m *Mixer) mixFrame(frame []int16, scratch []int16) MixStats {
	for i := range frame {
		frame[i] = 0
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Accumulate in int32 to avoid wrap before limiting.
	accum := make([]int32, FrameSize)
	active := 0

	for _, inp := range m.inputs {
		n := inp.Buffer.Read(scratch)
		if inp.Muted {
			continue
		}
		// Count as active if it contributed any non-zero / had data available.
		if n > 0 {
			active++
		}
		level := inp.Level
		for i := 0; i < FrameSize; i++ {
			accum[i] += int32(math.Round(float64(scratch[i]) * level))
		}
	}

	var peakAbs int32
	clipped := false
	for i := 0; i < FrameSize; i++ {
		v := accum[i]
		a := v
		if a < 0 {
			a = -a
		}
		if a > peakAbs {
			peakAbs = a
		}
		// Hard clip-guard: never wrap int16.
		if v > math.MaxInt16 {
			v = math.MaxInt16
			clipped = true
		} else if v < math.MinInt16 {
			v = math.MinInt16
			clipped = true
		}
		frame[i] = int16(v)
	}

	return MixStats{
		ActiveSources: active,
		Peak:          float64(peakAbs) / float64(math.MaxInt16),
		Clipped:       clipped,
	}
}

// Stop stops the mixer loop.
func (m *Mixer) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.stopped {
		m.stopped = true
		close(m.stopCh)
	}
}

// InputCount returns the number of active inputs.
func (m *Mixer) InputCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.inputs)
}

// WriteToInput writes PCM samples to an input's ring buffer.
func (m *Mixer) WriteToInput(id string, samples []int16) error {
	m.mu.RLock()
	inp, ok := m.inputs[id]
	m.mu.RUnlock()

	if !ok {
		return ErrInputNotFound
	}
	inp.Buffer.Write(samples)
	return nil
}
