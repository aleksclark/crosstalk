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

// Mixer manages N input streams and produces 1 mixed output stream.
// It runs a mix loop at 20ms intervals (960 samples per frame at 48kHz).
type Mixer struct {
	mu      sync.RWMutex
	inputs  map[string]*Input
	output  OutputFunc
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
func (m *Mixer) AddInput(id string, level float64, muted bool) *Input {
	m.mu.Lock()
	defer m.mu.Unlock()

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
			m.mixFrame(frame, scratch)
			if m.output != nil {
				m.output(frame)
			}
		}
	}
}

// MixOnce performs a single mix iteration (useful for testing without timers).
func (m *Mixer) MixOnce(frame []int16) {
	scratch := make([]int16, FrameSize)
	m.mixFrame(frame, scratch)
}

// mixFrame reads from all inputs, scales by level, sums, and clips.
func (m *Mixer) mixFrame(frame []int16, scratch []int16) {
	// Zero the output frame.
	for i := range frame {
		frame[i] = 0
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Accumulate in int32 to avoid overflow before clipping.
	accum := make([]int32, FrameSize)

	for _, inp := range m.inputs {
		if inp.Muted {
			// Still drain the buffer to prevent stalling
			inp.Buffer.Read(scratch)
			continue
		}

		inp.Buffer.Read(scratch)

		level := inp.Level
		for i := 0; i < FrameSize; i++ {
			accum[i] += int32(math.Round(float64(scratch[i]) * level))
		}
	}

	// Clip to int16 range.
	for i := 0; i < FrameSize; i++ {
		v := accum[i]
		if v > math.MaxInt16 {
			v = math.MaxInt16
		} else if v < math.MinInt16 {
			v = math.MinInt16
		}
		frame[i] = int16(v)
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
