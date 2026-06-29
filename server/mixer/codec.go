// Package mixer implements a pure audio mixing engine for CrossTalk.
// It operates on raw PCM int16 samples at 48kHz, producing 20ms frames (960 samples).
package mixer

// FrameSize is the number of samples per 20ms frame at 48kHz.
const FrameSize = 960

// Encoder encodes PCM int16 samples into a compressed format.
type Encoder interface {
	Encode(pcm []int16, out []byte) (int, error)
}

// Decoder decodes compressed audio into PCM int16 samples.
type Decoder interface {
	Decode(data []byte, pcm []int16) (int, error)
}

// NullEncoder passes PCM through as-is (little-endian int16 bytes).
// Used for testing without libopus.
type NullEncoder struct{}

// Encode writes pcm samples as little-endian int16 bytes into out.
// Returns the number of bytes written.
func (NullEncoder) Encode(pcm []int16, out []byte) (int, error) {
	needed := len(pcm) * 2
	if len(out) < needed {
		return 0, ErrBufferTooSmall
	}
	for i, s := range pcm {
		out[i*2] = byte(s)
		out[i*2+1] = byte(s >> 8)
	}
	return needed, nil
}

// NullDecoder passes raw bytes through as PCM int16 (little-endian).
// Used for testing without libopus.
type NullDecoder struct{}

// Decode reads little-endian int16 bytes from data into pcm.
// Returns the number of samples decoded.
func (NullDecoder) Decode(data []byte, pcm []int16) (int, error) {
	samples := len(data) / 2
	if samples > len(pcm) {
		samples = len(pcm)
	}
	for i := 0; i < samples; i++ {
		pcm[i] = int16(data[i*2]) | int16(data[i*2+1])<<8
	}
	return samples, nil
}
