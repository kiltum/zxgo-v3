package sound

// Source is an audio-producing chip. Sources record level *events* against the
// CPU tick clock; they never generate samples themselves and know nothing about
// the sample rate. The mixer resolves their events onto a sample grid.
//
// This is what makes AY additive: a second source implements this interface and
// nothing about the timing changes.
type Source interface {
	// MeanLevel returns the source's average output over [fromTick, toTick).
	//
	// The mixer calls this with strictly increasing, non-overlapping windows,
	// so an implementation may consume its event history as it advances.
	MeanLevel(fromTick, toTick int64) int16
}

// StereoSource is implemented by sources that can produce per-ear output (for
// example the AY, whose channels are panned). The mixer uses it when present;
// a plain Source is centred across both ears.
type StereoSource interface {
	MeanLevelStereo(fromTick, toTick int64) (int16, int16)
}

const (
	// ringMillis is the target audio latency held in the ring buffer.
	ringMillis = 100

	// resyncSeconds bounds catch-up work. If emulation stalls longer than this
	// (debugger breakpoint, host suspend), the grid jumps forward rather than
	// generating minutes of samples in one call.
	resyncSeconds = 2
)

// Mixer resolves source events onto a fixed sample grid and buffers the result.
//
// The grid is the whole point: sample n's timestamp is computed from a
// monotonic counter, never from an accumulated delta, so rounding error cannot
// build up. Total samples emitted is always determined by elapsed T-states
// alone -- never by how often the sources happened to change level.
type Mixer struct {
	out        AudioOutput
	sources    []Source
	sampleRate int
	cpuFreq    int

	nextSample int64 // monotonic; the anti-drift invariant lives here
	generated  int64 // total samples the grid has produced, pre-buffering
	ring       *Ring

	scratchL, scratchR []int16 // reused, so draining never allocates

	// DC-block (AC coupling) state, one pole per ear. The AY output is unipolar,
	// so it carries a DC offset that real hardware removes with a series
	// capacitor; y = x - xPrev + R*yPrev with R = 255/256 does the same.
	dcInL, dcOutL int32
	dcInR, dcOutR int32

	resyncs int64
}

// NewMixer creates a mixer feeding the given output at the given CPU clock.
func NewMixer(out AudioOutput, cpuFreq int) *Mixer {
	rate := 44100
	if out != nil {
		if r := out.SampleRate(); r > 0 {
			rate = r
		}
	}
	frames := rate * ringMillis / 1000
	return &Mixer{
		out:        out,
		sampleRate: rate,
		cpuFreq:    cpuFreq,
		ring:       NewRing(frames),
		scratchL:   make([]int16, frames),
		scratchR:   make([]int16, frames),
	}
}

// AddSource registers an audio source. Call before emulation starts.
func (m *Mixer) AddSource(s Source) { m.sources = append(m.sources, s) }

// SetCPUClock sets the CPU frequency the tick grid is derived from.
func (m *Mixer) SetCPUClock(hz int) {
	if hz > 0 {
		m.cpuFreq = hz
	}
}

// tickOfSample is exact and depends only on the sample index, so no error
// accumulates across calls. This single line is what fixes the drift.
func (m *Mixer) tickOfSample(n int64) int64 {
	return n * int64(m.cpuFreq) / int64(m.sampleRate)
}

// Resolve generates every sample whose window has fully elapsed by uptoTick.
// Call once per frame, after the CPU has run -- never ahead of emulation, so
// the sources' event lists are always complete for the window being resolved.
//
// Must be paired with Drain at least every ~ringMillis of emulated audio.
// Resolving far more than that in one call overruns the ring and drops the
// excess (counted by Dropped) -- Generated always reflects the true grid count.
func (m *Mixer) Resolve(uptoTick int64) {
	if uptoTick-m.tickOfSample(m.nextSample) > int64(m.cpuFreq)*resyncSeconds {
		m.nextSample = uptoTick * int64(m.sampleRate) / int64(m.cpuFreq)
		m.resyncs++
	}

	for m.tickOfSample(m.nextSample+1) <= uptoTick {
		lo := m.tickOfSample(m.nextSample)
		hi := m.tickOfSample(m.nextSample + 1)

		var accL, accR int32
		for _, s := range m.sources {
			if ss, ok := s.(StereoSource); ok {
				l, r := ss.MeanLevelStereo(lo, hi)
				accL += int32(l)
				accR += int32(r)
			} else {
				v := int32(s.MeanLevel(lo, hi))
				accL += v
				accR += v
			}
		}
		// AC-couple each ear before clamping, so the high-pass sees the true
		// (unclipped) signal and unipolar sources do not sit on a DC offset.
		l := m.dcBlock(&m.dcInL, &m.dcOutL, accL)
		r := m.dcBlock(&m.dcInR, &m.dcOutR, accR)
		m.ring.Write(clamp16(l), clamp16(r))
		m.nextSample++
		m.generated++
	}
}

// dcBlock applies the one-pole high-pass y = x - xPrev + R*yPrev with
// R = 255/256, in fixed point. It removes the DC offset of unipolar sources
// (the AY), matching the series capacitor real hardware places at the output.
func (m *Mixer) dcBlock(in, out *int32, x int32) int32 {
	// y = x - xPrev + R*yPrev with R = 255/256. The product is widened to int64
	// so the multiply and shift are exact; yPrev - (yPrev>>8) would floor at
	// yPrev < 256 and leave a residual DC of ~255.
	y := int64(x) - int64(*in) + (int64(*out)*255)>>8
	*in = x
	*out = int32(y)
	return int32(y)
}

// Drain pushes every buffered frame to the output device. The ring is a
// staging buffer only; the device's own queue holds the playback backlog, and
// the emulator throttles on that queue (AudioOutput.Queued) rather than on
// wall-clock time. A single source of truth for latency removes the two
// competing control loops that used to click.
//
// scratchL/scratchR are sized to the ring capacity, so one Read drains the
// whole ring; the loop is a safety net only.
func (m *Mixer) Drain() {
	if m.out == nil {
		return
	}
	for {
		n := m.ring.Read(m.scratchL, m.scratchR)
		if n == 0 {
			return
		}
		m.out.PushSamples(m.scratchL[:n], m.scratchR[:n])
	}
}

// Flush drains all buffered samples to the output. With the device queue now
// the only latency buffer, this is the same operation as Drain; it exists for
// tests that need a deterministic, complete drain.
func (m *Mixer) Flush() {
	m.Drain()
}

// Discard drops buffered frames without pushing them to the device. Resolve
// still runs, so the sample grid stays anchored to emulated time, but nothing
// reaches the speakers. Used while the emulator runs unthrottled (fast tape
// load), where the device could never keep up and its queue would otherwise
// grow by the length of the whole tape.
func (m *Mixer) Discard() {
	m.ring.Reset()
}

// Generated reports how many samples the grid has produced since reset. This
// is the anti-drift invariant: it depends only on elapsed T-states, never on
// source activity or on how often Resolve/Drain were called. Downstream frames
// may still be dropped by ring overrun -- see Dropped.
func (m *Mixer) Generated() int64 { return m.generated }

// Depth reports buffered frames -- current audio latency.
func (m *Mixer) Depth() int { return m.ring.Depth() }

// Dropped reports frames lost to ring overrun.
func (m *Mixer) Dropped() int64 { return m.ring.Dropped() }

// Resyncs reports how many times the grid jumped forward after a long stall.
func (m *Mixer) Resyncs() int64 { return m.resyncs }

// SampleRate returns the grid's sample rate.
func (m *Mixer) SampleRate() int { return m.sampleRate }

// Reset returns the mixer to its initial state. The caller must also zero the
// emulator's tick counter -- the grid is anchored to tick 0.
func (m *Mixer) Reset() {
	m.nextSample = 0
	m.generated = 0
	m.resyncs = 0
	m.dcInL, m.dcOutL = 0, 0
	m.dcInR, m.dcOutR = 0, 0
	m.ring.Reset()
}

func clamp16(v int32) int16 {
	if v > 32767 {
		return 32767
	}
	if v < -32768 {
		return -32768
	}
	return int16(v)
}
