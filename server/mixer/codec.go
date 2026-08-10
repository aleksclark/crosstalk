// Package mixer implements a pure audio mixing engine for CrossTalk.
// It operates on raw PCM int16 samples at 48kHz, producing 20ms frames (960 samples).
package mixer

// FrameSize is the number of samples per 20ms frame at 48kHz.
const FrameSize = 960

// SampleRate is the mixer clock rate in Hz.
const SampleRate = 48000

// Encoder encodes PCM int16 samples into a compressed format.
// Production implementations live in the audiocodec package (Opus).
// Null/test encoders may only appear in *_test.go files.
type Encoder interface {
	Encode(pcm []int16, out []byte) (int, error)
}

// Decoder decodes compressed audio into PCM int16 samples.
// Production implementations live in the audiocodec package (Opus).
// Null/test decoders may only appear in *_test.go files.
type Decoder interface {
	Decode(data []byte, pcm []int16) (int, error)
}
