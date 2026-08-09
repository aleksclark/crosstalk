package mixer

// NullEncoder and NullDecoder are TEST HELPERS only.
//
// Production media path MUST use audiocodec Opus (hraban/opus). Encoded RTP
// passthrough / null codecs are forbidden as the production path.
// These remain exported so legacy integration tests can inject PCM-as-payload
// without CGO; they must not be wired into sessionrtc Manager.

// NullEncoder passes PCM through as-is (little-endian int16 bytes).
type NullEncoder struct{}

// Encode writes pcm samples as little-endian int16 bytes into out.
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
type NullDecoder struct{}

// Decode reads little-endian int16 bytes from data into pcm.
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
