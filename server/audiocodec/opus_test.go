package audiocodec

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sinePCM(freq float64, n int, amp float64) []int16 {
	out := make([]int16, n)
	for i := range out {
		out[i] = int16(amp * float64(math.MaxInt16) * math.Sin(2*math.Pi*freq*float64(i)/float64(SampleRate)))
	}
	return out
}

func rms(samples []int16) float64 {
	if len(samples) == 0 {
		return 0
	}
	var sum float64
	for _, s := range samples {
		v := float64(s)
		sum += v * v
	}
	return math.Sqrt(sum / float64(len(samples)))
}

// goertzel magnitude at freq (relative).
func goertzel(samples []int16, freq float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	k := 2 * math.Cos(2*math.Pi*freq/float64(SampleRate))
	var s1, s2 float64
	for _, x := range samples {
		s0 := float64(x) + k*s1 - s2
		s2 = s1
		s1 = s0
	}
	power := s1*s1 + s2*s2 - k*s1*s2
	return math.Sqrt(math.Abs(power)) / float64(len(samples))
}

func peakFreq(samples []int16, candidates []float64) float64 {
	bestF, bestM := 0.0, -1.0
	for _, f := range candidates {
		m := goertzel(samples, f)
		if m > bestM {
			bestM, bestF = m, f
		}
	}
	return bestF
}

func TestOpus_ValidateCodec(t *testing.T) {
	require.NoError(t, ValidateCodec(CodecInfo{MimeType: "audio/opus", ClockRate: 48000, Channels: 1}))
	require.NoError(t, ValidateCodec(CodecInfo{MimeType: "audio/opus", ClockRate: 48000, Channels: 2}))
	require.NoError(t, ValidateCodec(CodecInfo{MimeType: "opus", ClockRate: 0, Channels: 0}))
	assert.ErrorIs(t, ValidateCodec(CodecInfo{MimeType: "audio/pcmu", ClockRate: 8000}), ErrUnsupportedCodec)
	assert.ErrorIs(t, ValidateCodec(CodecInfo{MimeType: "audio/opus", ClockRate: 16000}), ErrBadSampleRate)
	assert.ErrorIs(t, ValidateCodec(CodecInfo{MimeType: "audio/opus", ClockRate: 48000, Channels: 6}), ErrBadChannels)
}

func TestOpus_DecodeValidMono20ms(t *testing.T) {
	enc, err := NewOpusEncoder()
	require.NoError(t, err)
	defer enc.Close()
	dec, err := NewOpusDecoder(1)
	require.NoError(t, err)
	defer dec.Close()

	pcm := sinePCM(440, FrameSize, 0.5)
	pkt := make([]byte, 4000)
	n, err := enc.Encode(pcm, pkt)
	require.NoError(t, err)
	require.Greater(t, n, 0)

	out := make([]int16, FrameSize)
	ns, err := dec.Decode(pkt[:n], out)
	require.NoError(t, err)
	assert.Equal(t, FrameSize, ns, "20ms @ 48kHz mono must decode to 960 samples")
}

func TestOpus_RoundtripEncodeDecode(t *testing.T) {
	enc, err := NewOpusEncoder()
	require.NoError(t, err)
	defer enc.Close()
	dec, err := NewOpusDecoder(1)
	require.NoError(t, err)
	defer dec.Close()

	const frames = 50 // 1 second
	var decoded []int16
	pkt := make([]byte, 4000)
	for i := 0; i < frames; i++ {
		// Continuous phase across frames.
		pcm := make([]int16, FrameSize)
		for j := range pcm {
			n := i*FrameSize + j
			pcm[j] = int16(0.5 * float64(math.MaxInt16) * math.Sin(2*math.Pi*440*float64(n)/float64(SampleRate)))
		}
		n, err := enc.Encode(pcm, pkt)
		require.NoError(t, err)
		out := make([]int16, FrameSize)
		ns, err := dec.Decode(pkt[:n], out)
		require.NoError(t, err)
		decoded = append(decoded, out[:ns]...)
	}

	// Drop first ~100ms (codec settle), then check frequency and RMS.
	skip := FrameSize * 5
	require.Greater(t, len(decoded), skip+FrameSize*10)
	body := decoded[skip:]

	freq := peakFreq(body, []float64{400, 420, 440, 460, 480, 500})
	assert.InDelta(t, 440, freq, 20, "recovered frequency within ±20 Hz")

	inRMS := rms(sinePCM(440, FrameSize, 0.5))
	outRMS := rms(body)
	// Broad RMS tolerance: Opus is lossy; allow 0.25x–2.5x.
	assert.Greater(t, outRMS, inRMS*0.25, "output RMS too low")
	assert.Less(t, outRMS, inRMS*2.5, "output RMS too high")
}

func TestOpus_IndependentDecodersNoSharedState(t *testing.T) {
	enc, err := NewOpusEncoder()
	require.NoError(t, err)
	defer enc.Close()

	decA, err := NewOpusDecoder(1)
	require.NoError(t, err)
	defer decA.Close()
	decB, err := NewOpusDecoder(1)
	require.NoError(t, err)
	defer decB.Close()

	pktBuf := make([]byte, 4000)
	var framesA, framesB [][]byte
	for i := 0; i < 30; i++ {
		// A = 440, B = 880
		pcmA := make([]int16, FrameSize)
		pcmB := make([]int16, FrameSize)
		for j := range pcmA {
			n := i*FrameSize + j
			pcmA[j] = int16(0.5 * float64(math.MaxInt16) * math.Sin(2*math.Pi*440*float64(n)/float64(SampleRate)))
			pcmB[j] = int16(0.5 * float64(math.MaxInt16) * math.Sin(2*math.Pi*880*float64(n)/float64(SampleRate)))
		}
		n, err := enc.Encode(pcmA, pktBuf)
		require.NoError(t, err)
		cp := make([]byte, n)
		copy(cp, pktBuf[:n])
		framesA = append(framesA, cp)

		// Fresh encoder for B to avoid state bleed on encode side.
	}
	encB, err := NewOpusEncoder()
	require.NoError(t, err)
	defer encB.Close()
	for i := 0; i < 30; i++ {
		pcmB := make([]int16, FrameSize)
		for j := range pcmB {
			n := i*FrameSize + j
			pcmB[j] = int16(0.5 * float64(math.MaxInt16) * math.Sin(2*math.Pi*880*float64(n)/float64(SampleRate)))
		}
		n, err := encB.Encode(pcmB, pktBuf)
		require.NoError(t, err)
		cp := make([]byte, n)
		copy(cp, pktBuf[:n])
		framesB = append(framesB, cp)
	}

	var outA, outB []int16
	tmp := make([]int16, FrameSize)
	for i := 0; i < 30; i++ {
		ns, err := decA.Decode(framesA[i], tmp)
		require.NoError(t, err)
		outA = append(outA, tmp[:ns]...)
		ns, err = decB.Decode(framesB[i], tmp)
		require.NoError(t, err)
		outB = append(outB, tmp[:ns]...)
	}

	skip := FrameSize * 3
	fA := peakFreq(outA[skip:], []float64{440, 880})
	fB := peakFreq(outB[skip:], []float64{440, 880})
	assert.Equal(t, 440.0, fA, "decoder A must stay on 440 — no shared state with B")
	assert.Equal(t, 880.0, fB, "decoder B must stay on 880 — no shared state with A")
}

func TestOpus_PacketLossPLCNotOtherSource(t *testing.T) {
	enc, err := NewOpusEncoder()
	require.NoError(t, err)
	defer enc.Close()
	dec, err := NewOpusDecoder(1)
	require.NoError(t, err)
	defer dec.Close()

	// Prime decoder with a few 440 Hz frames.
	pkt := make([]byte, 4000)
	var good []int16
	for i := 0; i < 10; i++ {
		pcm := make([]int16, FrameSize)
		for j := range pcm {
			n := i*FrameSize + j
			pcm[j] = int16(0.5 * float64(math.MaxInt16) * math.Sin(2*math.Pi*440*float64(n)/float64(SampleRate)))
		}
		n, err := enc.Encode(pcm, pkt)
		require.NoError(t, err)
		out := make([]int16, FrameSize)
		ns, err := dec.Decode(pkt[:n], out)
		require.NoError(t, err)
		good = append(good, out[:ns]...)
	}

	// Simulate loss: PLC frames must not inject a foreign tone (e.g. 880).
	var plc []int16
	for i := 0; i < 5; i++ {
		out := make([]int16, FrameSize)
		ns, err := dec.DecodePLC(out)
		require.NoError(t, err)
		plc = append(plc, out[:ns]...)
	}

	mag440 := goertzel(plc, 440)
	mag880 := goertzel(plc, 880)
	// PLC may be quiet or residual 440-ish; it must not prefer 880.
	assert.Less(t, mag880, mag440+1e-6+rms(plc)*0.5, "PLC must not introduce other-source 880 Hz content")
	// And PLC output length is a real frame.
	assert.Equal(t, FrameSize*5, len(plc))
	_ = good
}

func TestOpus_StereoDownmix(t *testing.T) {
	// Encode mono, decode with mono path is the production default.
	// Stereo decoder construction is validated here with PLC silence path.
	dec, err := NewOpusDecoder(2)
	require.NoError(t, err)
	defer dec.Close()

	// Without a prior packet, PLC should still fill mono samples (silence/PLC).
	out := make([]int16, FrameSize)
	ns, err := dec.DecodePLC(out)
	require.NoError(t, err)
	assert.Equal(t, FrameSize, ns)
}
