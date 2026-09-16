package sound

import (
	"math"
	"testing"
)

// --- helpers -----------------------------------------------------------------

type countOut struct {
	rate int
	n    int
	min  int16
	max  int16
}

func newCountOut(rate int) *countOut {
	return &countOut{rate: rate, min: math.MaxInt16, max: math.MinInt16}
}
func (c *countOut) Init() error     { return nil }
func (c *countOut) SampleRate() int { return c.rate }
func (c *countOut) Close() error    { return nil }
func (c *countOut) Queued() int     { return 0 }
func (c *countOut) HasClock() bool  { return false }
func (c *countOut) PushSamples(l, r []int16) error {
	c.n += len(l)
	for _, v := range l {
		if v < c.min {
			c.min = v
		}
		if v > c.max {
			c.max = v
		}
	}
	return nil
}

// squareSource is a test Source: a square wave of the given half-period,
// integrated exactly over each window.
type squareSource struct {
	halfPeriod int64
	amp        int16
}

func (s *squareSource) levelAt(t int64) int32 {
	if (t/s.halfPeriod)%2 == 0 {
		return int32(s.amp)
	}
	return -int32(s.amp)
}

func (s *squareSource) MeanLevel(from, to int64) int16 {
	if to <= from {
		return 0
	}
	var acc int64
	for t := from; t < to; t++ {
		acc += int64(s.levelAt(t))
	}
	return int16(acc / (to - from))
}

// checkNoDrift is THE regression gate for AUDIT S1.
//
// Total samples emitted must be determined by elapsed T-states alone, never by
// how often the source happened to change level. Tolerance is +/-1 because the
// final sample window may not have fully elapsed.
func checkNoDrift(t *testing.T, label string, emitted int, elapsedTicks int64, rate, cpu int) {
	t.Helper()
	exact := float64(elapsedTicks) * float64(rate) / float64(cpu)
	drift := float64(emitted) - exact
	pct := drift / exact * 100

	t.Logf("%-22s elapsed=%d ticks  expected=%.1f  emitted=%d  drift=%+.1f (%+.2f%%)",
		label, elapsedTicks, exact, emitted, drift, pct)

	if math.Abs(drift) > 1 {
		t.Errorf("%s: sample count drifted %+.1f samples (%+.2f%%); "+
			"must be within +/-1 of %.1f", label, drift, pct, exact)
	}
}

// togglePatterns spans benign to pathological. The 100-T-state case is the one
// that measured -20.6% on the legacy beeper (~17.5 kHz, ordinary beeper-engine
// territory). "steady" proves the count is independent of the source entirely.
var togglePatterns = []struct {
	name       string
	halfPeriod int64 // T-states between level changes; 0 = never toggle
}{
	{"steady (no transitions)", 0},
	{"100T (~17.5kHz)", 100},
	{"79T (sub-sample)", 79},
	{"1000T", 1000},
	{"37T (prime)", 37},
}

const (
	testCPU  = 3500000
	testRate = 44100
)

// --- the gate ----------------------------------------------------------------

// TestNoDrift_Mixer is the permanent gate. The sample-clock design makes the
// count a pure function of elapsed ticks, so every pattern must land within +/-1.
func TestNoDrift_Mixer(t *testing.T) {
	for _, p := range togglePatterns {
		t.Run(p.name, func(t *testing.T) {
			out := newCountOut(testRate)
			m := NewMixer(out, testCPU)

			hp := p.halfPeriod
			if hp == 0 {
				hp = math.MaxInt32 // never toggles within the run
			}
			src := &squareSource{halfPeriod: hp, amp: 10000}
			m.AddSource(src)

			// Pump once per frame, as the emulator will.
			const frameTicks = 69888
			const frames = 50
			var tick int64
			for i := 0; i < frames; i++ {
				tick += frameTicks
				m.Resolve(tick)
				m.Drain()
			}
			m.Flush() // Drain remaining buffered samples for test verification

			checkNoDrift(t, p.name, out.n, tick, testRate, testCPU)

			// This gate measures the output, so pin that nothing was lost on
			// the way there -- a lossy path could otherwise mask or mimic drift.
			if m.Dropped() != 0 {
				t.Errorf("%s: ring dropped %d frames during per-frame pumping",
					p.name, m.Dropped())
			}
			if int64(out.n) != m.Generated() {
				t.Errorf("%s: grid generated %d but output received %d",
					p.name, m.Generated(), out.n)
			}

			// Guard against passing vacuously by emitting silence.
			if p.halfPeriod != 0 && out.min == out.max {
				t.Errorf("%s: output never varied (min==max==%d); "+
					"count is right but no audio was produced", p.name, out.min)
			}
		})
	}
}

// TestNoDrift_RealBeeper runs the production Beeper -- not a test double --
// through the same gate, driven the way the emulator drives it.
//
// This replaces the legacy-baseline test, whose measurements are recorded in
// AUDIT S1: the old design drifted -20.63% at 100T, +3.17% at 1000T, and
// emitted *zero* samples at 37T, where every delta rounded away.
func TestNoDrift_RealBeeper(t *testing.T) {
	for _, p := range togglePatterns {
		t.Run(p.name, func(t *testing.T) {
			out := newCountOut(testRate)
			m := NewMixer(out, testCPU)
			b := NewBeeper()
			m.AddSource(b)

			const frameTicks = 69888
			const frames = 50
			var tick int64
			level := uint8(0x00)

			for i := 0; i < frames; i++ {
				end := tick + frameTicks
				if p.halfPeriod > 0 {
					for tick+p.halfPeriod <= end {
						tick += p.halfPeriod
						b.SetTick(tick)
						if level == 0x00 {
							level = 0x10
						} else {
							level = 0x00
						}
						b.Write(0xFE, level)
					}
				}
				tick = end
				m.Resolve(tick)
				m.Drain()
			}
			m.Flush() // Drain remaining buffered samples for test verification

			checkNoDrift(t, "beeper "+p.name, out.n, tick, testRate, testCPU)

			if m.Dropped() != 0 {
				t.Errorf("%s: ring dropped %d frames", p.name, m.Dropped())
			}
			if p.halfPeriod != 0 && out.min == out.max {
				t.Errorf("%s: output never varied (min==max==%d)", p.name, out.min)
			}
			// A couple of events legitimately remain queued in the trailing
			// partial sample window; what matters is that they drain rather
			// than accumulating a frame's worth (thousands) per pump.
			if pend := b.Pending(); pend > 8 {
				t.Errorf("%s: %d events queued after resolving; not draining",
					p.name, pend)
			}
		})
	}
}

// --- grid properties ---------------------------------------------------------

// TestMixerGridIndependentOfPumpRate pins the property that makes RunFrame and
// RunSlice interchangeable (AUDIT T7): the grid's sample count must depend only
// on elapsed T-states, never on how the caller chunks its Resolve calls.
//
// Measured via Generated (the grid count), not the output count -- a caller that
// resolves a full second in one go legitimately overruns the 100ms ring and
// drops the excess. That buffering property is asserted separately below.
func TestMixerGridIndependentOfPumpRate(t *testing.T) {
	const total int64 = 3500000 // 1 second

	run := func(chunk int64) (generated int64, emitted int, dropped int64) {
		out := newCountOut(testRate)
		m := NewMixer(out, testCPU)
		m.AddSource(&squareSource{halfPeriod: 100, amp: 10000})
		var tick int64
		for tick < total {
			tick += chunk
			if tick > total {
				tick = total
			}
			m.Resolve(tick)
			m.Drain()
		}
		m.Flush() // Drain remaining buffered samples for test verification
		return m.Generated(), out.n, m.Dropped()
	}

	oneShot, oneShotOut, oneShotDrop := run(total)
	perFrame, perFrameOut, perFrameDrop := run(69888)
	tiny, tinyOut, tinyDrop := run(97) // far smaller than one sample window

	t.Logf("one-shot:  generated=%d emitted=%d dropped=%d", oneShot, oneShotOut, oneShotDrop)
	t.Logf("per-frame: generated=%d emitted=%d dropped=%d", perFrame, perFrameOut, perFrameDrop)
	t.Logf("tiny:      generated=%d emitted=%d dropped=%d", tiny, tinyOut, tinyDrop)

	if oneShot != perFrame || oneShot != tiny {
		t.Errorf("grid count depends on pump granularity: "+
			"one-shot=%d per-frame=%d tiny=%d", oneShot, perFrame, tiny)
	}
	checkNoDrift(t, "grid/one-shot", int(oneShot), total, testRate, testCPU)

	// The real-world contract: pumping once per frame must never lose a sample.
	if perFrameDrop != 0 {
		t.Errorf("per-frame pumping dropped %d frames; ring too small "+
			"(capacity %d frames, one frame of audio ~%d)",
			perFrameDrop, testRate*ringMillis/1000, 69888*testRate/testCPU)
	}
	if int64(perFrameOut) != perFrame {
		t.Errorf("per-frame: emitted %d but grid generated %d; samples lost in Drain",
			perFrameOut, perFrame)
	}
}

// TestMeanLevelCapturesSubSampleTransitions pins the resampling contract: a
// transition landing between two sample points must still affect the output.
// Point-sampling would drop it, which is the aliasing this design avoids.
func TestMeanLevelCapturesSubSampleTransitions(t *testing.T) {
	// ~79 T-states per sample at 3.5MHz/44.1kHz. A 20-tick half-period toggles
	// ~4x within a single sample window.
	src := &squareSource{halfPeriod: 20, amp: 10000}
	mean := src.MeanLevel(0, 79)

	full := src.MeanLevel(0, 20) // a window entirely inside one half-period
	t.Logf("mean over sub-sample toggles=%d, mean over steady window=%d", mean, full)

	if full != 10000 {
		t.Errorf("steady window should read full amplitude, got %d", full)
	}
	if mean == 10000 || mean == -10000 {
		t.Errorf("window containing transitions read as a steady level (%d); "+
			"transitions between sample points are being lost", mean)
	}
}

// TestMixerResyncOnLongStall checks both halves of the contract: a long stall
// must not generate minutes of audio, AND audio must resume at the correct rate
// afterwards. Asserting only the first half passes vacuously on total silence.
func TestMixerResyncOnLongStall(t *testing.T) {
	out := newCountOut(testRate)
	m := NewMixer(out, testCPU)
	m.AddSource(&squareSource{halfPeriod: 100, amp: 10000})

	// Simulate a 60-second stall (debugger breakpoint / host suspend).
	const stallTick = int64(60) * testCPU
	m.Resolve(stallTick)
	m.Drain()

	if m.Resyncs() != 1 {
		t.Errorf("expected 1 resync after a long stall, got %d", m.Resyncs())
	}
	afterStall := m.Generated()
	if afterStall > int64(testRate) {
		t.Errorf("stall generated %d samples; resync should have capped it well "+
			"under one second's worth (%d)", afterStall, testRate)
	}
	t.Logf("60s stall generated %d samples, %d resync(s)", afterStall, m.Resyncs())

	// The half that matters: normal audio must resume at the correct rate.
	const frameTicks = 69888
	const frames = 50
	tick := stallTick
	for i := 0; i < frames; i++ {
		tick += frameTicks
		m.Resolve(tick)
		m.Drain()
	}
	m.Flush() // Drain remaining buffered samples for test verification
	resumed := m.Generated() - afterStall
	if resumed == 0 {
		t.Fatal("no audio generated after the stall; grid did not re-anchor")
	}
	checkNoDrift(t, "resumed after stall", int(resumed),
		int64(frameTicks)*frames, testRate, testCPU)
}

// --- ring --------------------------------------------------------------------

func TestRingRoundTrip(t *testing.T) {
	r := NewRing(8)
	for i := 0; i < 5; i++ {
		r.Write(int16(i), int16(-i))
	}
	if r.Depth() != 5 {
		t.Fatalf("depth = %d, want 5", r.Depth())
	}
	l := make([]int16, 8)
	rt := make([]int16, 8)
	n := r.Read(l, rt)
	if n != 5 {
		t.Fatalf("read %d frames, want 5", n)
	}
	for i := 0; i < 5; i++ {
		if l[i] != int16(i) || rt[i] != int16(-i) {
			t.Errorf("frame %d = (%d,%d), want (%d,%d)", i, l[i], rt[i], i, -i)
		}
	}
	if r.Depth() != 0 {
		t.Errorf("depth after full read = %d, want 0", r.Depth())
	}
}

func TestRingOverrunDropsOldestAndCounts(t *testing.T) {
	r := NewRing(4)
	for i := 0; i < 7; i++ {
		r.Write(int16(i), int16(i))
	}
	if r.Depth() != 4 {
		t.Errorf("depth = %d, want 4 (capacity)", r.Depth())
	}
	if r.Dropped() != 3 {
		t.Errorf("dropped = %d, want 3", r.Dropped())
	}
	l := make([]int16, 4)
	rt := make([]int16, 4)
	r.Read(l, rt)
	// Oldest dropped, so the survivors are 3,4,5,6.
	for i, want := range []int16{3, 4, 5, 6} {
		if l[i] != want {
			t.Errorf("frame %d = %d, want %d (oldest should be dropped)", i, l[i], want)
		}
	}
}

func TestRingPartialRead(t *testing.T) {
	r := NewRing(8)
	for i := 0; i < 6; i++ {
		r.Write(int16(i), int16(i))
	}
	l := make([]int16, 3)
	rt := make([]int16, 3)
	if n := r.Read(l, rt); n != 3 {
		t.Fatalf("read %d, want 3", n)
	}
	if r.Depth() != 3 {
		t.Errorf("depth = %d, want 3", r.Depth())
	}
	if n := r.Read(l, rt); n != 3 {
		t.Fatalf("second read %d, want 3", n)
	}
	if n := r.Read(l, rt); n != 0 {
		t.Errorf("read on empty ring = %d, want 0", n)
	}
}
