package display

import (
	"encoding/binary"
	"math"
	"sync"
)

// LevelMeter computes audio RMS level from S16LE PCM samples written to it.
// It implements io.Writer so it can be inserted into an audio pipeline.
type LevelMeter struct {
	mu    sync.Mutex
	level float64
	decay float64
}

// NewLevelMeter creates a level meter with the given decay factor (0-1).
// Higher decay = slower falloff. Typical: 0.85.
func NewLevelMeter(decay float64) *LevelMeter {
	return &LevelMeter{decay: decay}
}

// Write processes S16LE PCM samples and updates the level.
func (m *LevelMeter) Write(p []byte) (int, error) {
	samples := len(p) / 2
	if samples == 0 {
		return len(p), nil
	}

	var sumSq float64
	for i := 0; i < samples; i++ {
		s := int16(binary.LittleEndian.Uint16(p[i*2:]))
		sumSq += float64(s) * float64(s)
	}
	rms := math.Sqrt(sumSq / float64(samples))

	level := rms / 16000.0
	if level > 1.0 {
		level = 1.0
	}

	m.mu.Lock()
	if level > m.level {
		m.level = level
	} else {
		m.level = m.level*m.decay + level*(1-m.decay)
	}
	m.mu.Unlock()

	return len(p), nil
}

// Level returns the current RMS level (0.0-1.0).
func (m *LevelMeter) Level() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.level
}
