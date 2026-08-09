package mixer

import (
	"math"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRingBuffer_WriteRead(t *testing.T) {
	rb := NewRingBuffer(10)

	// Write 5 samples
	rb.Write([]int16{1, 2, 3, 4, 5})
	assert.Equal(t, 5, rb.Available())

	// Read 5
	out := make([]int16, 5)
	n := rb.Read(out)
	assert.Equal(t, 5, n)
	assert.Equal(t, []int16{1, 2, 3, 4, 5}, out)
	assert.Equal(t, 0, rb.Available())
}

func TestRingBuffer_Overflow(t *testing.T) {
	rb := NewRingBuffer(4)

	// Write 6 samples into a 4-sample buffer — oldest 2 get overwritten
	rb.Write([]int16{1, 2, 3, 4, 5, 6})
	assert.Equal(t, 4, rb.Available())

	out := make([]int16, 4)
	n := rb.Read(out)
	assert.Equal(t, 4, n)
	assert.Equal(t, []int16{3, 4, 5, 6}, out)
}

func TestRingBuffer_ReadMoreThanAvailable(t *testing.T) {
	rb := NewRingBuffer(10)
	rb.Write([]int16{100, 200})

	out := make([]int16, 5)
	n := rb.Read(out)
	assert.Equal(t, 2, n)
	// First 2 should have data, rest zeroed
	assert.Equal(t, int16(100), out[0])
	assert.Equal(t, int16(200), out[1])
	assert.Equal(t, int16(0), out[2])
	assert.Equal(t, int16(0), out[3])
	assert.Equal(t, int16(0), out[4])
}

func TestMixer_TwoSignalsSumCorrectly(t *testing.T) {
	frame := make([]int16, FrameSize)

	m := New(nil, WithRingSize(FrameSize*2))

	m.AddInput("src1", 1.0, false)
	m.AddInput("src2", 1.0, false)

	// Write constant-value frames
	samples1 := make([]int16, FrameSize)
	samples2 := make([]int16, FrameSize)
	for i := range samples1 {
		samples1[i] = 1000
		samples2[i] = 2000
	}

	require.NoError(t, m.WriteToInput("src1", samples1))
	require.NoError(t, m.WriteToInput("src2", samples2))

	m.MixOnce(frame)

	// Every sample should be 1000 + 2000 = 3000
	for i := 0; i < FrameSize; i++ {
		assert.Equal(t, int16(3000), frame[i], "sample %d", i)
	}
}

func TestMixer_MuteRemovesFromMix(t *testing.T) {
	frame := make([]int16, FrameSize)

	m := New(nil, WithRingSize(FrameSize*2))

	m.AddInput("src1", 1.0, false)
	m.AddInput("src2", 1.0, true) // muted

	samples1 := make([]int16, FrameSize)
	samples2 := make([]int16, FrameSize)
	for i := range samples1 {
		samples1[i] = 500
		samples2[i] = 700
	}

	require.NoError(t, m.WriteToInput("src1", samples1))
	require.NoError(t, m.WriteToInput("src2", samples2))

	m.MixOnce(frame)

	// Muted source should not contribute
	for i := 0; i < FrameSize; i++ {
		assert.Equal(t, int16(500), frame[i], "sample %d", i)
	}
}

func TestMixer_LevelScalesProportionally(t *testing.T) {
	frame := make([]int16, FrameSize)

	m := New(nil, WithRingSize(FrameSize*2))

	m.AddInput("src1", 0.5, false) // half level

	samples := make([]int16, FrameSize)
	for i := range samples {
		samples[i] = 1000
	}

	require.NoError(t, m.WriteToInput("src1", samples))

	m.MixOnce(frame)

	// 1000 * 0.5 = 500
	for i := 0; i < FrameSize; i++ {
		assert.Equal(t, int16(500), frame[i], "sample %d", i)
	}
}

func TestMixer_LevelDouble(t *testing.T) {
	frame := make([]int16, FrameSize)

	m := New(nil, WithRingSize(FrameSize*2))

	m.AddInput("src1", 2.0, false)

	samples := make([]int16, FrameSize)
	for i := range samples {
		samples[i] = 5000
	}

	require.NoError(t, m.WriteToInput("src1", samples))

	m.MixOnce(frame)

	// 5000 * 2.0 = 10000
	for i := 0; i < FrameSize; i++ {
		assert.Equal(t, int16(10000), frame[i], "sample %d", i)
	}
}

func TestMixer_Clipping(t *testing.T) {
	frame := make([]int16, FrameSize)

	m := New(nil, WithRingSize(FrameSize*2))

	m.AddInput("src1", 1.0, false)
	m.AddInput("src2", 1.0, false)

	// Both at near max -> should clip
	samples1 := make([]int16, FrameSize)
	samples2 := make([]int16, FrameSize)
	for i := range samples1 {
		samples1[i] = 20000
		samples2[i] = 20000
	}

	require.NoError(t, m.WriteToInput("src1", samples1))
	require.NoError(t, m.WriteToInput("src2", samples2))

	m.MixOnce(frame)

	// 20000 + 20000 = 40000 > MaxInt16 -> clipped to 32767
	for i := 0; i < FrameSize; i++ {
		assert.Equal(t, int16(math.MaxInt16), frame[i], "sample %d", i)
	}
}

func TestMixer_NegativeClipping(t *testing.T) {
	frame := make([]int16, FrameSize)

	m := New(nil, WithRingSize(FrameSize*2))

	m.AddInput("src1", 1.0, false)
	m.AddInput("src2", 1.0, false)

	samples1 := make([]int16, FrameSize)
	samples2 := make([]int16, FrameSize)
	for i := range samples1 {
		samples1[i] = -20000
		samples2[i] = -20000
	}

	require.NoError(t, m.WriteToInput("src1", samples1))
	require.NoError(t, m.WriteToInput("src2", samples2))

	m.MixOnce(frame)

	// -20000 + -20000 = -40000 < MinInt16 -> clipped to -32768
	for i := 0; i < FrameSize; i++ {
		assert.Equal(t, int16(math.MinInt16), frame[i], "sample %d", i)
	}
}

func TestMixer_AddRemoveInput(t *testing.T) {
	m := New(nil, WithRingSize(FrameSize*2))

	m.AddInput("src1", 1.0, false)
	m.AddInput("src2", 1.0, false)
	assert.Equal(t, 2, m.InputCount())

	err := m.RemoveInput("src1")
	require.NoError(t, err)
	assert.Equal(t, 1, m.InputCount())

	err = m.RemoveInput("nonexistent")
	assert.ErrorIs(t, err, ErrInputNotFound)
}

func TestMixer_SetLevelAndMuted(t *testing.T) {
	m := New(nil, WithRingSize(FrameSize*2))

	m.AddInput("src1", 1.0, false)

	err := m.SetLevel("src1", 0.75)
	require.NoError(t, err)
	inp := m.GetInput("src1")
	assert.Equal(t, 0.75, inp.Level)

	err = m.SetMuted("src1", true)
	require.NoError(t, err)
	inp = m.GetInput("src1")
	assert.True(t, inp.Muted)

	// Nonexistent
	err = m.SetLevel("nope", 1.0)
	assert.ErrorIs(t, err, ErrInputNotFound)
	err = m.SetMuted("nope", false)
	assert.ErrorIs(t, err, ErrInputNotFound)
}

func TestMixer_RunAndStop(t *testing.T) {
	var frameCount atomic.Int64
	m := New(func(frame []int16) {
		frameCount.Add(1)
	}, WithRingSize(FrameSize*2))

	go m.Run()

	// Give it time to produce a few frames (~50ms = 2-3 frames at 20ms each)
	time.Sleep(60 * time.Millisecond)

	m.Stop()

	assert.Greater(t, frameCount.Load(), int64(0), "should have produced at least one frame")
}

func TestNullCodec(t *testing.T) {
	enc := NullEncoder{}
	dec := NullDecoder{}

	pcm := []int16{100, -200, 32767, -32768, 0}
	buf := make([]byte, len(pcm)*2)

	n, err := enc.Encode(pcm, buf)
	require.NoError(t, err)
	assert.Equal(t, len(pcm)*2, n)

	decoded := make([]int16, len(pcm))
	ns, err := dec.Decode(buf[:n], decoded)
	require.NoError(t, err)
	assert.Equal(t, len(pcm), ns)
	assert.Equal(t, pcm, decoded)
}

func TestNullEncoder_BufferTooSmall(t *testing.T) {
	enc := NullEncoder{}
	pcm := []int16{1, 2, 3}
	buf := make([]byte, 2) // too small for 3 samples

	_, err := enc.Encode(pcm, buf)
	assert.ErrorIs(t, err, ErrBufferTooSmall)
}

func TestMixer_WriteToNonexistentInput(t *testing.T) {
	m := New(nil, WithRingSize(FrameSize*2))

	err := m.WriteToInput("nonexistent", make([]int16, FrameSize))
	assert.ErrorIs(t, err, ErrInputNotFound)
}

func TestMixer_EmptyMixProducesSilence(t *testing.T) {
	frame := make([]int16, FrameSize)

	m := New(nil, WithRingSize(FrameSize*2))

	m.MixOnce(frame)

	for i := 0; i < FrameSize; i++ {
		assert.Equal(t, int16(0), frame[i], "sample %d should be silent", i)
	}
}
