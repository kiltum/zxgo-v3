package sound

// Ring is a fixed-capacity stereo sample buffer that decouples emulation from
// the audio device. The emulator writes into it at whatever rate instructions
// execute; the device drains it at its own rate.
//
// On overrun the oldest frame is dropped rather than blocking emulation, and
// the drop is counted so the condition is observable instead of silent.
type Ring struct {
	buf     []int16 // interleaved L,R
	capF    int     // capacity in frames
	r, w    int     // frame cursors
	n       int     // frames currently held
	dropped int64
}

// NewRing creates a ring holding `frames` stereo frames.
func NewRing(frames int) *Ring {
	if frames < 1 {
		frames = 1
	}
	return &Ring{buf: make([]int16, frames*2), capF: frames}
}

// Write appends one stereo frame, dropping the oldest if full.
func (rb *Ring) Write(l, r int16) {
	if rb.n == rb.capF {
		rb.r = (rb.r + 1) % rb.capF
		rb.n--
		rb.dropped++
	}
	i := rb.w * 2
	rb.buf[i], rb.buf[i+1] = l, r
	rb.w = (rb.w + 1) % rb.capF
	rb.n++
}

// Read copies up to min(len(left), len(right), Depth()) frames out and returns
// how many were transferred.
func (rb *Ring) Read(left, right []int16) int {
	n := len(left)
	if len(right) < n {
		n = len(right)
	}
	if rb.n < n {
		n = rb.n
	}
	for k := 0; k < n; k++ {
		i := rb.r * 2
		left[k], right[k] = rb.buf[i], rb.buf[i+1]
		rb.r = (rb.r + 1) % rb.capF
	}
	rb.n -= n
	return n
}

// Depth reports frames currently buffered -- this is the audio latency, and the
// signal a future master-clock loop can throttle on.
func (rb *Ring) Depth() int { return rb.n }

// Capacity reports the ring size in frames.
func (rb *Ring) Capacity() int { return rb.capF }

// Dropped reports how many frames have been discarded to overrun since reset.
func (rb *Ring) Dropped() int64 { return rb.dropped }

// Reset empties the ring and clears the drop counter.
func (rb *Ring) Reset() {
	rb.r, rb.w, rb.n = 0, 0, 0
	rb.dropped = 0
}
