package mixer

import "sync"

// RingBuffer is a thread-safe PCM sample ring buffer.
// It stores int16 samples and supports concurrent read/write.
type RingBuffer struct {
	mu   sync.Mutex
	buf  []int16
	size int
	wpos int // next write position
	rpos int // next read position
	len  int // number of samples available
}

// NewRingBuffer creates a ring buffer that holds capacity samples.
func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		buf:  make([]int16, capacity),
		size: capacity,
	}
}

// Write appends samples to the ring buffer.
// If the buffer is full, oldest samples are overwritten.
func (r *RingBuffer) Write(samples []int16) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, s := range samples {
		r.buf[r.wpos] = s
		r.wpos = (r.wpos + 1) % r.size
		if r.len < r.size {
			r.len++
		} else {
			// Overwrite: advance read position
			r.rpos = (r.rpos + 1) % r.size
		}
	}
}

// Read reads up to len(out) samples from the ring buffer.
// Returns the number of samples actually read.
// If fewer samples are available than requested, the remaining positions in out are zeroed.
func (r *RingBuffer) Read(out []int16) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	n := len(out)
	if n > r.len {
		// Zero the portion we can't fill
		for i := r.len; i < n; i++ {
			out[i] = 0
		}
		n = r.len
	}

	for i := 0; i < n; i++ {
		out[i] = r.buf[r.rpos]
		r.rpos = (r.rpos + 1) % r.size
	}
	r.len -= n
	return n
}

// Available returns the number of samples available for reading.
func (r *RingBuffer) Available() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.len
}
