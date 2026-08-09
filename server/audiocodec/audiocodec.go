// Package audiocodec provides production Opus encode/decode adapters for the
// CrossTalk media engine. It wraps libopus via github.com/hraban/opus and is
// the only package that may import that CGO dependency.
//
// Contract:
//   - 48 kHz sample rate
//   - 20 ms frames = 960 mono samples
//   - Stereo input is explicitly downmixed to mono on decode
//   - Unsupported codecs are rejected before any bridge activation
package audiocodec

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/hraban/opus"
)

const (
	// SampleRate is the only supported PCM rate (Hz).
	SampleRate = 48000
	// FrameDurationMs is the mix/encode tick (milliseconds).
	FrameDurationMs = 20
	// FrameSize is mono samples per 20 ms frame at 48 kHz.
	FrameSize = SampleRate * FrameDurationMs / 1000 // 960
	// MaxChannels is the highest channel count accepted on decode (then downmixed).
	MaxChannels = 2
	// DefaultBitrate is the Opus encoder target bitrate (bits/s).
	DefaultBitrate = 64000
)

// Supported MIME types (case-insensitive match on the subtype).
const (
	MimeTypeOpus = "audio/opus"
)

// Errors returned by the codec boundary.
var (
	ErrUnsupportedCodec = errors.New("audiocodec: unsupported codec")
	ErrBadSampleRate    = errors.New("audiocodec: unsupported sample rate")
	ErrBadChannels      = errors.New("audiocodec: unsupported channel count")
	ErrBufferTooSmall   = errors.New("audiocodec: output buffer too small")
	ErrClosed           = errors.New("audiocodec: codec closed")
)

// Encoder encodes mono PCM int16 frames into compressed Opus packets.
type Encoder interface {
	Encode(pcm []int16, out []byte) (int, error)
	Close() error
}

// Decoder decodes Opus packets into mono PCM int16 frames.
// DecodePLC fills pcm with packet-loss concealment (or silence) for one frame.
type Decoder interface {
	Decode(data []byte, pcm []int16) (int, error)
	DecodePLC(pcm []int16) (int, error)
	Close() error
}

// CodecInfo describes a remote audio codec offer.
type CodecInfo struct {
	MimeType   string
	ClockRate  uint32
	Channels   uint16
	SDPFmtpLine string
}

// ValidateCodec rejects anything other than 48 kHz Opus before bridge activation.
// Channels may be 0 (unspecified), 1, or 2; stereo is accepted and downmixed.
func ValidateCodec(info CodecInfo) error {
	if !IsOpusMIME(info.MimeType) {
		return fmt.Errorf("%w: %q", ErrUnsupportedCodec, info.MimeType)
	}
	if info.ClockRate != 0 && info.ClockRate != SampleRate {
		return fmt.Errorf("%w: %d", ErrBadSampleRate, info.ClockRate)
	}
	ch := int(info.Channels)
	if ch == 0 {
		ch = 1
	}
	if ch < 1 || ch > MaxChannels {
		return fmt.Errorf("%w: %d", ErrBadChannels, info.Channels)
	}
	return nil
}

// IsOpusMIME reports whether mime is Opus (handles "audio/opus" and bare "opus").
func IsOpusMIME(mime string) bool {
	m := strings.ToLower(strings.TrimSpace(mime))
	return m == MimeTypeOpus || m == "opus" || strings.HasSuffix(m, "/opus")
}

// OpusEncoder is a mono 48 kHz Opus encoder.
type OpusEncoder struct {
	mu  sync.Mutex
	enc *opus.Encoder
}

// NewOpusEncoder creates a mono 48 kHz VoIP Opus encoder.
func NewOpusEncoder() (*OpusEncoder, error) {
	enc, err := opus.NewEncoder(SampleRate, 1, opus.AppVoIP)
	if err != nil {
		return nil, fmt.Errorf("audiocodec: new encoder: %w", err)
	}
	if err := enc.SetBitrate(DefaultBitrate); err != nil {
		return nil, fmt.Errorf("audiocodec: set bitrate: %w", err)
	}
	return &OpusEncoder{enc: enc}, nil
}

// Encode compresses one mono PCM frame into out. pcm should be FrameSize samples
// (or a multiple of the Opus frame size); returns bytes written.
func (e *OpusEncoder) Encode(pcm []int16, out []byte) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.enc == nil {
		return 0, ErrClosed
	}
	if len(out) == 0 {
		return 0, ErrBufferTooSmall
	}
	n, err := e.enc.Encode(pcm, out)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// Close releases encoder state.
func (e *OpusEncoder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.enc = nil
	return nil
}

// OpusDecoder decodes Opus to mono PCM. When constructed for stereo it
// downmixes L/R to mono explicitly.
type OpusDecoder struct {
	mu       sync.Mutex
	dec      *opus.Decoder
	channels int
	// scratch holds interleaved multi-channel PCM before downmix.
	scratch []int16
}

// NewOpusDecoder creates a 48 kHz Opus decoder.
// channels must be 1 or 2; stereo frames are downmixed to mono on Decode.
func NewOpusDecoder(channels int) (*OpusDecoder, error) {
	if channels < 1 || channels > MaxChannels {
		return nil, fmt.Errorf("%w: %d", ErrBadChannels, channels)
	}
	dec, err := opus.NewDecoder(SampleRate, channels)
	if err != nil {
		return nil, fmt.Errorf("audiocodec: new decoder: %w", err)
	}
	d := &OpusDecoder{dec: dec, channels: channels}
	if channels > 1 {
		d.scratch = make([]int16, FrameSize*channels*2) // headroom for PLC variance
	}
	return d, nil
}

// Decode decodes one Opus packet into mono PCM.
// Returns the number of mono samples written (typically FrameSize).
func (d *OpusDecoder) Decode(data []byte, pcm []int16) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.dec == nil {
		return 0, ErrClosed
	}
	if len(pcm) == 0 {
		return 0, ErrBufferTooSmall
	}
	if d.channels == 1 {
		n, err := d.dec.Decode(data, pcm)
		if err != nil {
			return 0, err
		}
		// hraban/opus returns samples-per-channel for mono == mono sample count.
		return n, nil
	}

	// Stereo path: decode interleaved, then downmix.
	need := len(pcm) * d.channels
	if cap(d.scratch) < need {
		d.scratch = make([]int16, need)
	}
	buf := d.scratch[:need]
	n, err := d.dec.Decode(data, buf)
	if err != nil {
		return 0, err
	}
	// n is samples per channel.
	mono := n
	if mono > len(pcm) {
		mono = len(pcm)
	}
	downmixStereo(buf, pcm, mono)
	return mono, nil
}

// DecodePLC synthesizes one lost frame into mono PCM via Opus PLC.
// Returns the number of mono samples written.
func (d *OpusDecoder) DecodePLC(pcm []int16) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.dec == nil {
		return 0, ErrClosed
	}
	if len(pcm) == 0 {
		return 0, ErrBufferTooSmall
	}
	if d.channels == 1 {
		// DecodePLC writes into the buffer; size must match missing duration.
		frame := pcm
		if len(frame) > FrameSize {
			frame = frame[:FrameSize]
		}
		if err := d.dec.DecodePLC(frame); err != nil {
			// Fall back to silence so the mix clock never stalls.
			for i := range frame {
				frame[i] = 0
			}
			return len(frame), nil
		}
		return len(frame), nil
	}

	need := FrameSize * d.channels
	if cap(d.scratch) < need {
		d.scratch = make([]int16, need)
	}
	buf := d.scratch[:need]
	if err := d.dec.DecodePLC(buf); err != nil {
		for i := range pcm {
			pcm[i] = 0
			if i+1 >= FrameSize {
				break
			}
		}
		n := FrameSize
		if n > len(pcm) {
			n = len(pcm)
		}
		return n, nil
	}
	mono := FrameSize
	if mono > len(pcm) {
		mono = len(pcm)
	}
	downmixStereo(buf, pcm, mono)
	return mono, nil
}

// Close releases decoder state.
func (d *OpusDecoder) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dec = nil
	return nil
}

// downmixStereo averages L/R into mono. src is interleaved stereo of monoLen frames.
func downmixStereo(src, dst []int16, monoLen int) {
	for i := 0; i < monoLen; i++ {
		l := int32(src[i*2])
		r := int32(src[i*2+1])
		dst[i] = int16((l + r) / 2)
	}
}

// Factory creates production Opus codecs. Safe for concurrent use.
type Factory struct{}

// NewEncoder returns a mono Opus encoder.
func (Factory) NewEncoder() (Encoder, error) { return NewOpusEncoder() }

// NewDecoder returns an Opus decoder for the given channel count (1 or 2).
func (Factory) NewDecoder(channels int) (Decoder, error) {
	if channels <= 0 {
		channels = 1
	}
	return NewOpusDecoder(channels)
}
